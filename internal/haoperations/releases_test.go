package haoperations

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type releaseRepositoryFake struct {
	value ReleaseCache
	err   error
	saves int
}

func (r *releaseRepositoryFake) ReleaseCache(context.Context) (ReleaseCache, error) {
	return r.value, r.err
}
func (r *releaseRepositoryFake) SaveReleaseCache(_ context.Context, value ReleaseCache) error {
	r.value = value
	r.err = nil
	r.saves++
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestReleaseCheckerReusesSuccessfulCacheAcrossWorkerWakes(t *testing.T) {
	start := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	now := start
	repository := &releaseRepositoryFake{value: ReleaseCache{
		Version: "v0.107.79", ReleaseURL: "https://github.com/AdguardTeam/AdGuardHome/releases/tag/v0.107.79",
		CheckedAt: start.Add(time.Second), ExpiresAt: start.Add(releaseCacheTTL + time.Second),
	}}
	requests := 0
	checker := NewReleaseChecker(repository)
	if checker.client.Timeout != 10*time.Second {
		t.Fatalf("HTTP timeout=%s", checker.client.Timeout)
	}
	checker.now = func() time.Time { return now }
	checker.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.String() != adGuardLatestReleaseURL {
			t.Fatalf("url=%s", request.URL)
		}
		if request.Header.Get("Accept") != "application/vnd.github+json" || request.Header.Get("User-Agent") != "Atlas-DNS-Controller" {
			t.Fatalf("headers=%v", request.Header)
		}
		return releaseResponse(http.StatusOK, `{"tag_name":"v0.107.80","html_url":"https://github.com/AdguardTeam/AdGuardHome/releases/tag/v0.107.80"}`), nil
	}), Timeout: 10 * time.Second}

	// Model every five-minute worker opportunity through T+6h. The cache was
	// written one second after the worker cadence was anchored, so it remains
	// valid at the old T+6h boundary.
	workerChecks := 0
	for now = start; !now.After(start.Add(releaseCacheTTL)); now = now.Add(5 * time.Minute) {
		workerChecks++
		if err := checker.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if requests != 0 || repository.saves != 0 || workerChecks != 73 {
		t.Fatalf("valid cache checks=%d upstream requests=%d saves=%d", workerChecks, requests, repository.saves)
	}

	// The first worker opportunity after expiry performs exactly one request.
	now = start.Add(releaseCacheTTL + 5*time.Minute)
	if err := checker.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || repository.saves != 1 || repository.value.Version != "v0.107.80" || !repository.value.ExpiresAt.Equal(now.Add(releaseCacheTTL)) {
		t.Fatalf("cache=%#v requests=%d saves=%d", repository.value, requests, repository.saves)
	}
}

func TestReleaseCheckerFailureCacheRetriesAtExpiry(t *testing.T) {
	start := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	now := start
	repository := &releaseRepositoryFake{value: ReleaseCache{
		Version: "v0.107.79", ReleaseURL: "https://github.com/AdguardTeam/AdGuardHome/releases/tag/v0.107.79",
		Compatibility: "supported", CheckedAt: start.Add(-releaseCacheTTL), ExpiresAt: start,
	}}
	requests := 0
	checker := NewReleaseChecker(repository)
	checker.now = func() time.Time { return now }
	checker.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return nil, errors.New("GitHub unavailable")
		}
		return releaseResponse(http.StatusOK, `{"tag_name":"v0.107.80","html_url":"https://github.com/AdguardTeam/AdGuardHome/releases/tag/v0.107.80"}`), nil
	})}

	if err := checker.Refresh(context.Background()); err == nil {
		t.Fatal("initial upstream failure was not reported")
	}
	if requests != 1 || repository.value.Version != "v0.107.79" || repository.value.ReleaseURL == "" || repository.value.Compatibility != "unknown" || repository.value.ErrorCode != "RELEASE_CHECK_UNAVAILABLE" || !repository.value.ExpiresAt.Equal(start.Add(releaseFailureCacheTTL)) {
		t.Fatalf("failure cache=%#v requests=%d", repository.value, requests)
	}
	for _, offset := range []time.Duration{5 * time.Minute, 10 * time.Minute} {
		now = start.Add(offset)
		if err := checker.Refresh(context.Background()); err != nil {
			t.Fatalf("cached failure at %s returned %v", offset, err)
		}
	}
	if requests != 1 {
		t.Fatalf("failure evidence caused an early retry: requests=%d", requests)
	}

	now = start.Add(releaseFailureCacheTTL)
	if err := checker.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || repository.value.Version != "v0.107.80" || repository.value.ErrorCode != "" {
		t.Fatalf("recovered cache=%#v requests=%d", repository.value, requests)
	}
}

func TestReleaseCheckerStartupUsesPersistedCacheContract(t *testing.T) {
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		cache     ReleaseCache
		wantCalls int
	}{
		{name: "valid success", cache: ReleaseCache{Version: "v0.107.79", ExpiresAt: now.Add(time.Minute)}},
		{name: "expired success", cache: ReleaseCache{Version: "v0.107.79", ExpiresAt: now.Add(-time.Nanosecond)}, wantCalls: 1},
		{name: "expired failure", cache: ReleaseCache{Version: "v0.107.79", ErrorCode: "RELEASE_CHECK_UNAVAILABLE", ExpiresAt: now.Add(-time.Nanosecond)}, wantCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &releaseRepositoryFake{value: test.cache}
			calls := 0
			checker := NewReleaseChecker(repository)
			checker.now = func() time.Time { return now }
			checker.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return releaseResponse(http.StatusOK, `{"tag_name":"v0.107.80","html_url":"https://github.com/AdguardTeam/AdGuardHome/releases/tag/v0.107.80"}`), nil
			})}
			if err := checker.Refresh(context.Background()); err != nil {
				t.Fatal(err)
			}
			if calls != test.wantCalls {
				t.Fatalf("startup upstream calls=%d want %d", calls, test.wantCalls)
			}
		})
	}
}

func TestReleaseCheckerRejectsInvalidOrOversizedResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		code   string
	}{
		{name: "status", status: http.StatusTooManyRequests, body: `{}`, code: "RELEASE_CHECK_UNAVAILABLE"},
		{name: "oversized", status: http.StatusOK, body: strings.Repeat("x", 64*1024+1), code: "RELEASE_CHECK_INVALID_RESPONSE"},
		{name: "invalid payload", status: http.StatusOK, body: `{"tag_name":""}`, code: "RELEASE_CHECK_INVALID_RESPONSE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
			repository := &releaseRepositoryFake{value: ReleaseCache{Version: "v0.107.79", ReleaseURL: "https://example.test/previous"}}
			checker := NewReleaseChecker(repository)
			checker.now = func() time.Time { return now }
			checker.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return releaseResponse(test.status, test.body), nil
			})}
			if err := checker.Refresh(context.Background()); err == nil {
				t.Fatal("invalid response was not reported")
			}
			if repository.value.ErrorCode != test.code || repository.value.Version != "v0.107.79" || repository.value.ReleaseURL == "" {
				t.Fatalf("failure cache=%#v", repository.value)
			}
		})
	}
}

func releaseResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

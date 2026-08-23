package adguard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/querylog"
)

func TestNormalizeQueryLogItemSupportsOptionalAndLegacyFields(t *testing.T) {
	var item queryLogItem
	if err := json.Unmarshal([]byte(`{
		"answer":[{"ttl":10,"type":"AAAA","value":"::"}],
		"client":"192.0.2.0","client_info":{"name":"Living room"},
		"elapsedMs":"0.098403","question":{"host":"Example.COM.","type":"aaaa"},
		"reason":"FilteredBlackList","rules":[{"text":"||example.com^","filter_list_id":7}],
		"status":"NOERROR","time":"2026-08-09T01:02:03.123456789Z"
	}`), &item); err != nil {
		t.Fatal(err)
	}
	event, ok := normalizeQueryLogItem(item)
	if !ok {
		t.Fatal("valid query-log item was rejected")
	}
	if event.QueryName != "example.com" || event.QueryType != "AAAA" || event.ResponseStatus != querylog.StatusBlocked || event.ClientDisplayName != "Living room" {
		t.Fatalf("unexpected normalized event: %#v", event)
	}
}

func TestReadLegacyQueryLogConfigAcceptsFractionalInterval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/control/querylog_info" {
			http.NotFound(response, request)
			return
		}
		_, _ = response.Write([]byte(`{"enabled":true,"interval":0.25,"anonymize_client_ip":true}`))
	}))
	defer server.Close()
	config, err := NewConfigurationReader(NewProbe(time.Second)).ReadQueryLogConfig(context.Background(), probeRequest(server.URL), "v0.107.52")
	if err != nil {
		t.Fatal(err)
	}
	if !config.Enabled || !config.AnonymizeClientIP {
		t.Fatalf("config = %+v", config)
	}
}

func TestReadQueryLogUsesStableSourceCursorWithoutOffset(t *testing.T) {
	older := "2026-08-09T01:02:03.123456789Z"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/control/querylog" || request.URL.Query().Get("older_than") != older || request.URL.Query().Get("limit") != "50" || request.URL.Query().Has("offset") {
			t.Errorf("unexpected request: %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		_, _ = response.Write([]byte(`{"oldest":"2026-08-09T01:01:00Z","data":[{"client":"192.0.2.0","elapsedMs":1,"question":{"name":"example.org","type":"A"},"status":"NOERROR","time":"2026-08-09T01:02:00Z"}]}`))
	}))
	defer server.Close()
	page, err := NewConfigurationReader(NewProbe(time.Second)).ReadQueryLog(context.Background(), probeRequest(server.URL), older, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Oldest != "2026-08-09T01:01:00Z" || page.Events[0].ClientIdentifier != "192.0.2.0" {
		t.Fatalf("page = %+v", page)
	}
}

func TestReadQueryLogCompatibilityFixtures(t *testing.T) {
	for _, version := range []string{"v0.107.78", "v0.107.79"} {
		version := version
		t.Run(version, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("testdata", version, "querylog.json"))
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/control/querylog" {
					http.NotFound(response, request)
					return
				}
				response.Header().Set("Content-Type", "application/json")
				_, _ = response.Write(body)
			}))
			defer server.Close()
			page, err := NewConfigurationReader(NewProbe(time.Second)).ReadQueryLog(context.Background(), probeRequest(server.URL), "", 500)
			if err != nil || len(page.Events) != 1 || page.InvalidRecords != 0 {
				t.Fatalf("ReadQueryLog(%s) page=%+v error=%v", version, page, err)
			}
		})
	}
}

func TestReadQueryLogMissingEndpointIsCapabilityError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	reader := NewConfigurationReader(NewProbe(time.Second))
	_, err := reader.ReadQueryLogConfig(context.Background(), probeRequest(server.URL), "v0.107.79")
	assertDomainErrorKind(t, err, domain.ErrorCapability)
	_, err = reader.ReadQueryLog(context.Background(), probeRequest(server.URL), "", 50)
	assertDomainErrorKind(t, err, domain.ErrorCapability)
}

func TestNormalizeQueryLogItemPreservesClientIDAsIdentifier(t *testing.T) {
	item := queryLogItem{ClientID: "dot-client", Time: "2026-08-09T01:02:03Z", ElapsedMS: json.RawMessage(`1`)}
	item.Question.Name, item.Question.Type = "example.org", "A"
	event, ok := normalizeQueryLogItem(item)
	if !ok || event.ClientIdentifier != "dot-client" || event.ClientDisplayName != "" {
		t.Fatalf("event = %+v ok=%v", event, ok)
	}
}

func TestNormalizeQueryLogItemAcceptsDNSRootQuestion(t *testing.T) {
	item := queryLogItem{Time: "2026-08-09T01:02:03Z", ElapsedMS: json.RawMessage(`1`)}
	item.Question.Name, item.Question.Type = ".", "NS"
	event, ok := normalizeQueryLogItem(item)
	if !ok {
		t.Fatal("DNS root query was rejected")
	}
	if event.QueryName != "." {
		t.Fatalf("expected DNS root name to be preserved, got %q", event.QueryName)
	}
}

func TestNormalizeQueryLogItemRejectsMalformedRecord(t *testing.T) {
	item := queryLogItem{Time: "not-a-time", ElapsedMS: json.RawMessage(`"NaN"`)}
	if _, ok := normalizeQueryLogItem(item); ok {
		t.Fatal("malformed item was accepted")
	}
}

func TestFilteringStatusMapping(t *testing.T) {
	for reason, want := range map[string]string{
		"NotFilteredNotFound": querylog.StatusAllowed,
		"FilteredBlackList":   querylog.StatusBlocked,
		"FilteredSafeSearch":  querylog.StatusSafeSearch,
		"RewriteRule":         querylog.StatusRewritten,
		"unexpected":          querylog.StatusOther,
	} {
		if got := mapFilteringStatus(reason); got != want {
			t.Errorf("mapFilteringStatus(%q) = %q, want %q", reason, got, want)
		}
	}
}

func TestSupportsQueryLogBoundaries(t *testing.T) {
	for version, want := range map[string]bool{"v0.107.51": false, "v0.107.52": true, "v0.107.78": true, "v0.107.79": true, "v0.107.80": true, "v0.108.0": false} {
		if got := SupportsQueryLog(version); got != want {
			t.Errorf("SupportsQueryLog(%q) = %v, want %v", version, got, want)
		}
	}
}

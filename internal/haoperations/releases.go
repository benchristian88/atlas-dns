package haoperations

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	adGuardLatestReleaseURL = "https://api.github.com/repos/AdguardTeam/AdGuardHome/releases/latest"
	releaseCacheTTL         = 6 * time.Hour
	releaseFailureCacheTTL  = 15 * time.Minute
)

type ReleaseRepository interface {
	ReleaseCache(context.Context) (ReleaseCache, error)
	SaveReleaseCache(context.Context, ReleaseCache) error
}

type ReleaseChecker struct {
	repository ReleaseRepository
	client     *http.Client
	now        func() time.Time
	cacheTTL   time.Duration
}

func NewReleaseChecker(repository ReleaseRepository) *ReleaseChecker {
	return &ReleaseChecker{repository: repository, client: &http.Client{Timeout: 10 * time.Second}, now: time.Now, cacheTTL: releaseCacheTTL}
}

func (c *ReleaseChecker) Refresh(ctx context.Context) error {
	if cached, err := c.repository.ReleaseCache(ctx); err == nil && c.now().UTC().Before(cached.ExpiresAt) {
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, adGuardLatestReleaseURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "Atlas-DNS-Controller")
	response, err := c.client.Do(request)
	if err != nil {
		return c.recordFailure(ctx, "RELEASE_CHECK_UNAVAILABLE")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return c.recordFailure(ctx, "RELEASE_CHECK_UNAVAILABLE")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
	if err != nil || len(body) > 64*1024 {
		return c.recordFailure(ctx, "RELEASE_CHECK_INVALID_RESPONSE")
	}
	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || strings.TrimSpace(payload.TagName) == "" {
		return c.recordFailure(ctx, "RELEASE_CHECK_INVALID_RESPONSE")
	}
	now := c.now().UTC()
	return c.repository.SaveReleaseCache(ctx, ReleaseCache{Version: strings.TrimSpace(payload.TagName), ReleaseURL: strings.TrimSpace(payload.HTMLURL), Compatibility: "unknown", CheckedAt: now, ExpiresAt: now.Add(c.cacheTTL)})
}

func (c *ReleaseChecker) recordFailure(ctx context.Context, code string) error {
	now := c.now().UTC()
	value := ReleaseCache{Compatibility: "unknown", CheckedAt: now, ExpiresAt: now.Add(releaseFailureCacheTTL), ErrorCode: code}
	if current, err := c.repository.ReleaseCache(ctx); err == nil {
		value.Version, value.ReleaseURL = current.Version, current.ReleaseURL
	}
	if err := c.repository.SaveReleaseCache(ctx, value); err != nil {
		return err
	}
	return errors.New("upstream release check failed")
}

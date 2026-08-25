package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "https://controller.example.test")
	t.Setenv("DATABASE_URL", "postgres://example.invalid/test")
	t.Setenv("SESSION_SECRET", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 48))))
	t.Setenv("CREDENTIAL_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")))
	t.Setenv("NODE_HEALTH_INTERVAL", "45s")
	configuration, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !configuration.SecureCookies() {
		t.Fatal("SecureCookies() = false for HTTPS public URL")
	}
	if configuration.NodeHealthInterval.String() != "45s" {
		t.Fatalf("NodeHealthInterval = %v", configuration.NodeHealthInterval)
	}
	if configuration.StatisticsPollInterval.String() != "1h0m0s" {
		t.Fatalf("StatisticsPollInterval = %v", configuration.StatisticsPollInterval)
	}
	if !configuration.QueryLogCollection || configuration.QueryLogPollInterval.String() != "30s" || configuration.QueryLogRetention != 7*24*time.Hour {
		t.Fatalf("unexpected query-log defaults: enabled=%v interval=%v retention=%v", configuration.QueryLogCollection, configuration.QueryLogPollInterval, configuration.QueryLogRetention)
	}
}

func TestLoadRejectsUnsafeQueryLogBounds(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "http://localhost:8080")
	t.Setenv("DATABASE_URL", "postgres://example.invalid/test")
	t.Setenv("SESSION_SECRET", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 48))))
	t.Setenv("CREDENTIAL_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")))
	t.Setenv("QUERY_LOG_RETENTION", "2400h")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted excessive query-log retention")
	}
}

func TestLoadRejectsUnsafePersistedMonitoringFallbackBounds(t *testing.T) {
	for name, value := range map[string]string{
		"NODE_HEALTH_INTERVAL":     "2s",
		"STATISTICS_POLL_INTERVAL": "25h",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("PUBLIC_BASE_URL", "http://localhost:8080")
			t.Setenv("DATABASE_URL", "postgres://example.invalid/test")
			t.Setenv("SESSION_SECRET", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 48))))
			t.Setenv("CREDENTIAL_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")))
			t.Setenv(name, value)
			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted unsafe %s=%s", name, value)
			}
		})
	}
}

func TestLoadRejectsPlaceholderCredentialKey(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "http://localhost:8080")
	t.Setenv("DATABASE_URL", "postgres://example.invalid/test")
	t.Setenv("SESSION_SECRET", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 48))))
	t.Setenv("CREDENTIAL_ENCRYPTION_KEY", "replace-me")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a non-base64 credential key")
	}
}

func TestLoadRejectsUnsafePublicURL(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "https://admin:secret@controller.example.test/path")
	t.Setenv("DATABASE_URL", "postgres://example.invalid/test")
	t.Setenv("SESSION_SECRET", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 48))))
	t.Setenv("CREDENTIAL_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")))
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a public URL containing credentials and a path")
	}
}

func TestLoadRejectsPlaceholderSessionSecret(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "http://localhost:8080")
	t.Setenv("DATABASE_URL", "postgres://example.invalid/test")
	t.Setenv("SESSION_SECRET", "replace-me")
	t.Setenv("CREDENTIAL_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")))
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a non-base64 session secret")
	}
}

func TestLoadRejectsShortMetricsToken(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "http://localhost:8080")
	t.Setenv("DATABASE_URL", "postgres://example.invalid/test")
	t.Setenv("SESSION_SECRET", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 48))))
	t.Setenv("CREDENTIAL_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")))
	t.Setenv("METRICS_BEARER_TOKEN", "too-short")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a short metrics bearer token")
	}
}

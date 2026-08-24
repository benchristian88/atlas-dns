package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment             string
	HTTPAddress             string
	DatabaseURL             string
	PublicBaseURL           *url.URL
	SessionSecret           []byte
	CredentialEncryptionKey []byte
	SessionDuration         time.Duration
	NodeHealthInterval      time.Duration
	NodeRequestTimeout      time.Duration
	StatisticsPollInterval  time.Duration
	QueryLogCollection      bool
	QueryLogPollInterval    time.Duration
	QueryLogRetention       time.Duration
	WebDistDirectory        string
	LogLevel                string
	AutoMigrate             bool
	MetricsToken            string
	PGDumpPath              string
	PGRestorePath           string
	InstallationType        string
}

func Load() (Config, error) {
	publicURL, err := url.Parse(required("PUBLIC_BASE_URL"))
	if err != nil || publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" ||
		(publicURL.Path != "" && publicURL.Path != "/") || (publicURL.Scheme != "http" && publicURL.Scheme != "https") {
		return Config{}, fmt.Errorf("PUBLIC_BASE_URL must be an origin URL without credentials, path, query, or fragment")
	}
	publicURL.Path = ""
	credentialKey, err := decodeKey(required("CREDENTIAL_ENCRYPTION_KEY"))
	if err != nil {
		return Config{}, err
	}
	sessionSecret, err := decodeSecret(required("SESSION_SECRET"))
	if err != nil {
		return Config{}, err
	}
	sessionDuration, err := duration("SESSION_DURATION", 12*time.Hour)
	if err != nil {
		return Config{}, err
	}
	healthInterval, err := duration("NODE_HEALTH_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	if healthInterval < 5*time.Second || healthInterval > time.Hour {
		return Config{}, fmt.Errorf("NODE_HEALTH_INTERVAL must be between 5s and 1h")
	}
	requestTimeout, err := duration("NODE_REQUEST_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	statisticsInterval, err := duration("STATISTICS_POLL_INTERVAL", time.Hour)
	if err != nil {
		return Config{}, err
	}
	if statisticsInterval < time.Minute || statisticsInterval > 24*time.Hour {
		return Config{}, fmt.Errorf("STATISTICS_POLL_INTERVAL must be between 1m and 24h")
	}
	queryLogCollection, err := boolean("QUERY_LOG_COLLECTION_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	queryLogInterval, err := duration("QUERY_LOG_POLL_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	if queryLogInterval < 5*time.Second || queryLogInterval > time.Hour {
		return Config{}, fmt.Errorf("QUERY_LOG_POLL_INTERVAL must be between 5s and 1h")
	}
	queryLogRetention, err := duration("QUERY_LOG_RETENTION", 7*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	if queryLogRetention < time.Hour || queryLogRetention > 90*24*time.Hour {
		return Config{}, fmt.Errorf("QUERY_LOG_RETENTION must be between 1h and 2160h")
	}
	autoMigrate, err := boolean("AUTO_MIGRATE", true)
	if err != nil {
		return Config{}, err
	}
	if required("DATABASE_URL") == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	metricsToken := required("METRICS_BEARER_TOKEN")
	if metricsToken != "" && len(metricsToken) < 32 {
		return Config{}, fmt.Errorf("METRICS_BEARER_TOKEN must be at least 32 characters when configured")
	}
	installationType := env("INSTALLATION_TYPE", "unknown")
	if installationType != "docker" && installationType != "native_systemd" && installationType != "custom" && installationType != "unknown" {
		return Config{}, fmt.Errorf("INSTALLATION_TYPE must be docker, native_systemd, custom, or unknown")
	}
	return Config{
		Environment:             env("APP_ENV", "development"),
		HTTPAddress:             env("HTTP_ADDR", ":8080"),
		DatabaseURL:             required("DATABASE_URL"),
		PublicBaseURL:           publicURL,
		SessionSecret:           sessionSecret,
		CredentialEncryptionKey: credentialKey,
		SessionDuration:         sessionDuration,
		NodeHealthInterval:      healthInterval,
		NodeRequestTimeout:      requestTimeout,
		StatisticsPollInterval:  statisticsInterval,
		QueryLogCollection:      queryLogCollection,
		QueryLogPollInterval:    queryLogInterval,
		QueryLogRetention:       queryLogRetention,
		WebDistDirectory:        env("WEB_DIST_DIR", "web/dist"),
		LogLevel:                env("LOG_LEVEL", "info"),
		AutoMigrate:             autoMigrate,
		MetricsToken:            metricsToken,
		PGDumpPath:              env("PG_DUMP_PATH", "pg_dump"),
		PGRestorePath:           env("PG_RESTORE_PATH", "pg_restore"),
		InstallationType:        installationType,
	}, nil
}

func (c Config) SecureCookies() bool { return c.PublicBaseURL.Scheme == "https" }

func required(key string) string { return strings.TrimSpace(os.Getenv(key)) }

func env(key, fallback string) string {
	if value := required(key); value != "" {
		return value
	}
	return fallback
}

func decodeKey(value string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("CREDENTIAL_ENCRYPTION_KEY must be base64 encoding of exactly 32 bytes")
	}
	return decoded, nil
}

func decodeSecret(value string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) < 32 {
		return nil, fmt.Errorf("SESSION_SECRET must be base64 encoding of at least 32 bytes")
	}
	return decoded, nil
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	value := required(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}

func boolean(key string, fallback bool) (bool, error) {
	value := required(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return parsed, nil
}

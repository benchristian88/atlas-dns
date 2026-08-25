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
	// v1.1 reads these variables only as a one-time legacy seed. Invalid stale
	// values therefore fall back safely and cannot prevent a database-backed
	// installation from starting.
	sessionDuration := legacyDuration("SESSION_DURATION", 12*time.Hour, 15*time.Minute, 30*24*time.Hour)
	healthInterval := legacyDuration("NODE_HEALTH_INTERVAL", 30*time.Second, 5*time.Second, time.Hour)
	requestTimeout := legacyDuration("NODE_REQUEST_TIMEOUT", 10*time.Second, time.Second, 2*time.Minute)
	statisticsInterval := legacyDuration("STATISTICS_POLL_INTERVAL", time.Hour, time.Minute, 24*time.Hour)
	queryLogCollection := legacyBoolean("QUERY_LOG_COLLECTION_ENABLED", true)
	queryLogInterval := legacyDuration("QUERY_LOG_POLL_INTERVAL", 30*time.Second, 5*time.Second, time.Hour)
	queryLogRetention := legacyDuration("QUERY_LOG_RETENTION", 7*24*time.Hour, time.Hour, 90*24*time.Hour)
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
	logLevel := env("LOG_LEVEL", "info")
	if logLevel != "debug" && logLevel != "info" && logLevel != "warn" && logLevel != "error" {
		logLevel = "info"
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
		LogLevel:                logLevel,
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

func legacyDuration(key string, fallback, minimum, maximum time.Duration) time.Duration {
	value, err := duration(key, fallback)
	if err != nil || value < minimum || value > maximum {
		return fallback
	}
	return value
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

func legacyBoolean(key string, fallback bool) bool {
	value, err := boolean(key, fallback)
	if err != nil {
		return fallback
	}
	return value
}

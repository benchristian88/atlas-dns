package querylog

import (
	"crypto/sha256"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	StatusAllowed      = "allowed"
	StatusBlocked      = "blocked"
	StatusRewritten    = "rewritten"
	StatusSafeSearch   = "safe_search"
	StatusSafeBrowsing = "safe_browsing"
	StatusParental     = "parental"
	StatusError        = "error"
	StatusOther        = "other"
)

type Answer struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   int64  `json:"ttl,omitempty"`
}

type Rule struct {
	Text         string `json:"text"`
	FilterListID int64  `json:"filterListId,omitempty"`
}

// SourceEvent is the normalized, bounded representation produced by a node
// adapter. It excludes raw response bodies, credentials, node URLs, and WHOIS.
type SourceEvent struct {
	Timestamp         time.Time `json:"-"`
	QueryName         string    `json:"query"`
	QueryType         string    `json:"queryType"`
	ClientIdentifier  string    `json:"clientIdentifier"`
	ClientDisplayName string    `json:"clientDisplayName,omitempty"`
	ClientProtocol    string    `json:"clientProtocol,omitempty"`
	ResponseStatus    string    `json:"status"`
	ResponseCode      string    `json:"responseCode,omitempty"`
	ElapsedMS         float64   `json:"processingTimeMs"`
	Upstream          string    `json:"upstream,omitempty"`
	FilteringReason   string    `json:"filteringReason,omitempty"`
	ServiceName       string    `json:"serviceName,omitempty"`
	Rules             []Rule    `json:"rules"`
	Answers           []Answer  `json:"answers"`
	Cached            bool      `json:"cached"`
	AnswerDNSSEC      bool      `json:"answerDnssec"`
}

type SourcePage struct {
	Events         []SourceEvent
	Oldest         string
	InvalidRecords int
}

type SourceConfig struct {
	Enabled           bool
	AnonymizeClientIP bool
}

func (e *SourceEvent) Normalize() bool {
	e.Timestamp = e.Timestamp.UTC()
	e.QueryName = normalizeDomain(e.QueryName)
	e.QueryType = upperBounded(e.QueryType, 32)
	e.ClientIdentifier = bounded(e.ClientIdentifier, 512)
	e.ClientDisplayName = bounded(e.ClientDisplayName, 512)
	e.ClientProtocol = strings.ToLower(bounded(e.ClientProtocol, 32))
	e.ResponseCode = upperBounded(e.ResponseCode, 64)
	e.Upstream = bounded(e.Upstream, 2048)
	e.FilteringReason = bounded(e.FilteringReason, 128)
	e.ServiceName = bounded(e.ServiceName, 256)
	if !ValidStatus(e.ResponseStatus) {
		e.ResponseStatus = StatusOther
	}
	if math.IsNaN(e.ElapsedMS) || math.IsInf(e.ElapsedMS, 0) || e.ElapsedMS < 0 {
		return false
	}
	if len(e.Rules) > 10 {
		e.Rules = e.Rules[:10]
	}
	for index := range e.Rules {
		e.Rules[index].Text = bounded(e.Rules[index].Text, 2048)
	}
	if len(e.Answers) > 20 {
		e.Answers = e.Answers[:20]
	}
	for index := range e.Answers {
		e.Answers[index].Type = upperBounded(e.Answers[index].Type, 32)
		e.Answers[index].Value = bounded(e.Answers[index].Value, 2048)
	}
	return !e.Timestamp.IsZero() && e.QueryName != "" && e.QueryType != ""
}

func (e SourceEvent) Fingerprint() [sha256.Size]byte {
	body, _ := json.Marshal(struct {
		Timestamp        string
		QueryName        string
		QueryType        string
		ClientIdentifier string
		ClientProtocol   string
		ResponseStatus   string
		ResponseCode     string
		ElapsedMS        float64
		Upstream         string
		FilteringReason  string
		ServiceName      string
		Rules            []Rule
		Answers          []Answer
		Cached           bool
		AnswerDNSSEC     bool
	}{e.Timestamp.Format(time.RFC3339Nano), e.QueryName, e.QueryType, e.ClientIdentifier,
		e.ClientProtocol, e.ResponseStatus, e.ResponseCode,
		e.ElapsedMS, e.Upstream, e.FilteringReason, e.ServiceName, e.Rules, e.Answers,
		e.Cached, e.AnswerDNSSEC})
	return sha256.Sum256(body)
}

type Event struct {
	ID                string    `json:"id"`
	ClusterID         string    `json:"-"`
	NodeID            string    `json:"nodeId"`
	NodeName          string    `json:"nodeName"`
	SourceTimestamp   time.Time `json:"timestamp"`
	IngestedAt        time.Time `json:"ingestedAt"`
	SourceFingerprint []byte    `json:"-"`
	SourceOccurrence  int       `json:"-"`
	SourceEvent
}

type Checkpoint struct {
	ClusterID           string
	NodeID              string
	HighWatermarkAt     *time.Time
	SourceNewestAt      *time.Time
	SourceOldestAt      *time.Time
	LastAttemptAt       time.Time
	LastSuccessAt       *time.Time
	LastStatus          string
	ErrorCode           string
	GapDetected         bool
	GapReason           string
	LoggingEnabled      *bool
	NodeVersion         string
	UpdatedAt           time.Time
	ConsecutiveFailures int
}

type Attempt struct {
	ID              string
	ClusterID       string
	NodeID          string
	StartedAt       time.Time
	CompletedAt     time.Time
	Status          string
	ErrorCode       string
	FetchedRecords  int
	InsertedRecords int
	PageCount       int
	GapDetected     bool
	GapReason       string
}

func ValidStatus(value string) bool {
	switch value {
	case StatusAllowed, StatusBlocked, StatusRewritten, StatusSafeSearch,
		StatusSafeBrowsing, StatusParental, StatusError, StatusOther:
		return true
	default:
		return false
	}
}

func SupportsVersion(version string) bool {
	value := strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(value, ".")
	if len(parts) < 3 {
		return false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	patch, patchErr := strconv.Atoi(parts[2])
	return majorErr == nil && minorErr == nil && patchErr == nil && major == 0 && minor == 107 && patch >= 52
}

// VersionBelowMinimum distinguishes evidence of an unsupported old contract
// from a malformed or different API generation whose capability is unknown.
func VersionBelowMinimum(version string) bool {
	value := strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(value, ".")
	if len(parts) < 3 {
		return false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	patch, patchErr := strconv.Atoi(parts[2])
	if majorErr != nil || minorErr != nil || patchErr != nil || major != 0 {
		return false
	}
	return minor < 107 || (minor == 107 && patch < 52)
}

func normalizeDomain(value string) string {
	value = bounded(value, 1025)
	if value == "." {
		return value
	}
	return strings.ToLower(strings.TrimSuffix(value, "."))
}

func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func upperBounded(value string, limit int) string { return strings.ToUpper(bounded(value, limit)) }

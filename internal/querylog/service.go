package querylog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

var queryTypePattern = regexp.MustCompile(`^[A-Z][A-Z0-9-]{0,31}$`)

type Repository interface {
	ClusterByID(context.Context, string) (domain.Cluster, error)
	ListNodes(context.Context, string) ([]domain.Node, error)
	ListQueryEvents(context.Context, EventQuery) ([]Event, error)
	QueryEventByID(context.Context, string, string) (Event, error)
	QueryLogCheckpoints(context.Context, string, string) ([]Checkpoint, error)
	QueryLogTypes(context.Context, string, string) ([]string, error)
}

type Service struct {
	repository        Repository
	pollInterval      time.Duration
	collectionEnabled bool
	retention         time.Duration
	now               func() time.Time
}

type Options struct {
	CollectionEnabled bool
	Retention         time.Duration
}

func NewService(repository Repository, pollInterval time.Duration, options ...Options) *Service {
	settings := Options{CollectionEnabled: true, Retention: 7 * 24 * time.Hour}
	if len(options) > 0 {
		settings = options[0]
	}
	return &Service{repository: repository, pollInterval: pollInterval, collectionEnabled: settings.CollectionEnabled, retention: settings.Retention, now: time.Now}
}

type EventQuery struct {
	ClusterID string
	NodeID    string
	Search    string
	Status    string
	QueryType string
	Client    string
	Limit     int
	BeforeAt  *time.Time
	BeforeID  string
}

type ListRequest struct {
	ClusterID string
	NodeID    string
	Cursor    string
	Search    string
	Status    string
	QueryType string
	Client    string
	Limit     int
}

type Page struct {
	Items       []Event    `json:"items"`
	NextCursor  string     `json:"nextCursor,omitempty"`
	GeneratedAt time.Time  `json:"generatedAt"`
	Coverage    Coverage   `json:"coverage"`
	Filters     FilterData `json:"filters"`
}

type FilterData struct {
	Statuses   []string `json:"statuses"`
	QueryTypes []string `json:"queryTypes"`
}

type Coverage struct {
	Status            string         `json:"status"`
	CollectionEnabled bool           `json:"collectionEnabled"`
	RetentionSeconds  int64          `json:"retentionSeconds"`
	ExpectedNodes     int            `json:"expectedNodes"`
	IncludedNodes     int            `json:"includedNodes"`
	StaleNodes        int            `json:"staleNodes"`
	UnsupportedNodes  int            `json:"unsupportedNodes"`
	DisabledNodes     int            `json:"disabledNodes"`
	MaintenanceNodes  int            `json:"maintenanceNodes"`
	ErrorNodes        int            `json:"errorNodes"`
	GapNodes          int            `json:"gapNodes"`
	CurrentThrough    *time.Time     `json:"currentThrough,omitempty"`
	StaleAfterSeconds int64          `json:"staleAfterSeconds"`
	Nodes             []NodeCoverage `json:"nodes"`
}

type NodeCoverage struct {
	NodeID         string     `json:"nodeId"`
	NodeName       string     `json:"nodeName"`
	Status         string     `json:"status"`
	ReasonCode     string     `json:"reasonCode,omitempty"`
	LastAttemptAt  *time.Time `json:"lastAttemptAt,omitempty"`
	LastSuccessAt  *time.Time `json:"lastSuccessAt,omitempty"`
	CurrentThrough *time.Time `json:"currentThrough,omitempty"`
	GapDetected    bool       `json:"gapDetected"`
}

type cursorPayload struct {
	Version int    `json:"v"`
	At      string `json:"at"`
	ID      string `json:"id"`
}

func (s *Service) List(ctx context.Context, request ListRequest) (Page, error) {
	if !domain.ValidID(request.ClusterID) {
		return Page{}, domain.Validation("clusterId", "must be a valid UUID")
	}
	if request.NodeID != "" && !domain.ValidID(request.NodeID) {
		return Page{}, domain.Validation("nodeId", "must be a valid UUID")
	}
	request.Search = strings.TrimSpace(request.Search)
	request.Client = strings.TrimSpace(request.Client)
	request.Status = strings.TrimSpace(request.Status)
	request.QueryType = strings.ToUpper(strings.TrimSpace(request.QueryType))
	if len(request.Search) > 256 {
		return Page{}, domain.Validation("search", "must be at most 256 characters")
	}
	if len(request.Client) > 512 {
		return Page{}, domain.Validation("client", "must be at most 512 characters")
	}
	if request.Status != "" && !ValidStatus(request.Status) {
		return Page{}, domain.Validation("status", "is not a supported query status")
	}
	if request.QueryType != "" && !queryTypePattern.MatchString(request.QueryType) {
		return Page{}, domain.Validation("queryType", "must be a valid DNS query type")
	}
	if request.Limit < 1 || request.Limit > 100 {
		return Page{}, domain.Validation("limit", "must be between 1 and 100")
	}
	if _, err := s.repository.ClusterByID(ctx, request.ClusterID); err != nil {
		return Page{}, err
	}
	nodes, err := s.repository.ListNodes(ctx, request.ClusterID)
	if err != nil {
		return Page{}, err
	}
	if request.NodeID != "" && !containsNode(nodes, request.NodeID) {
		return Page{}, domain.NewError(domain.ErrorNotFound, "node was not found in the cluster")
	}
	query := EventQuery{ClusterID: request.ClusterID, NodeID: request.NodeID, Search: request.Search,
		Status: request.Status, QueryType: request.QueryType, Client: request.Client, Limit: request.Limit + 1}
	if request.Cursor != "" {
		at, id, decodeErr := decodeCursor(request.Cursor)
		if decodeErr != nil {
			return Page{}, domain.Validation("cursor", "is invalid or unsupported")
		}
		query.BeforeAt, query.BeforeID = &at, id
	}
	events, err := s.repository.ListQueryEvents(ctx, query)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: events, GeneratedAt: s.now().UTC(), Filters: FilterData{Statuses: []string{
		StatusAllowed, StatusBlocked, StatusRewritten, StatusSafeSearch, StatusSafeBrowsing, StatusParental, StatusError, StatusOther,
	}}}
	if len(page.Items) > request.Limit {
		page.Items = page.Items[:request.Limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeCursor(last.SourceTimestamp, last.ID)
	}
	page.Filters.QueryTypes, err = s.repository.QueryLogTypes(ctx, request.ClusterID, request.NodeID)
	if err != nil {
		return Page{}, err
	}
	checkpoints, err := s.repository.QueryLogCheckpoints(ctx, request.ClusterID, request.NodeID)
	if err != nil {
		return Page{}, err
	}
	page.Coverage = s.coverage(nodes, checkpoints, request.NodeID)
	return page, nil
}

func (s *Service) Detail(ctx context.Context, clusterID, eventID string) (Event, error) {
	if !domain.ValidID(clusterID) {
		return Event{}, domain.Validation("clusterId", "must be a valid UUID")
	}
	if !domain.ValidID(eventID) {
		return Event{}, domain.Validation("eventId", "must be a valid UUID")
	}
	if _, err := s.repository.ClusterByID(ctx, clusterID); err != nil {
		return Event{}, err
	}
	return s.repository.QueryEventByID(ctx, clusterID, eventID)
}

func (s *Service) coverage(nodes []domain.Node, checkpoints []Checkpoint, nodeID string) Coverage {
	staleAfter := 3 * s.pollInterval
	if staleAfter < 2*time.Minute {
		staleAfter = 2 * time.Minute
	}
	result := Coverage{Status: "complete", CollectionEnabled: s.collectionEnabled, RetentionSeconds: int64(s.retention.Seconds()), StaleAfterSeconds: int64(staleAfter.Seconds()), Nodes: []NodeCoverage{}}
	byNode := make(map[string]Checkpoint, len(checkpoints))
	for _, checkpoint := range checkpoints {
		byNode[checkpoint.NodeID] = checkpoint
	}
	now := s.now().UTC()
	for _, node := range nodes {
		if nodeID != "" && node.ID != nodeID {
			continue
		}
		coverage := NodeCoverage{NodeID: node.ID, NodeName: node.Name}
		if !node.Enabled {
			coverage.Status, coverage.ReasonCode = "excluded", "NODE_DISABLED"
			result.Nodes = append(result.Nodes, coverage)
			continue
		}
		result.ExpectedNodes++
		if !s.collectionEnabled {
			coverage.Status, coverage.ReasonCode = "collection_disabled", "QUERY_LOG_COLLECTION_DISABLED"
			result.DisabledNodes++
			result.Nodes = append(result.Nodes, coverage)
			continue
		}
		if node.MaintenanceMode {
			coverage.Status, coverage.ReasonCode = "maintenance", "NODE_MAINTENANCE"
			result.MaintenanceNodes++
			result.Nodes = append(result.Nodes, coverage)
			continue
		}
		if VersionBelowMinimum(node.Version) {
			coverage.Status, coverage.ReasonCode = "unsupported", "QUERY_LOG_UNSUPPORTED"
			result.UnsupportedNodes++
			result.Nodes = append(result.Nodes, coverage)
			continue
		}
		if !SupportsVersion(node.Version) {
			coverage.Status, coverage.ReasonCode = "error", "QUERY_LOG_CAPABILITY_UNKNOWN"
			result.ErrorNodes++
			result.Nodes = append(result.Nodes, coverage)
			continue
		}
		checkpoint, ok := byNode[node.ID]
		if !ok {
			coverage.Status, coverage.ReasonCode = "missing", "QUERY_LOG_NOT_INGESTED"
			result.ErrorNodes++
			result.Nodes = append(result.Nodes, coverage)
			continue
		}
		attemptAt := checkpoint.LastAttemptAt
		coverage.LastAttemptAt, coverage.LastSuccessAt, coverage.CurrentThrough = &attemptAt, checkpoint.LastSuccessAt, checkpoint.SourceNewestAt
		coverage.GapDetected = checkpoint.GapDetected
		if checkpoint.LoggingEnabled != nil && !*checkpoint.LoggingEnabled {
			coverage.Status, coverage.ReasonCode = "logging_disabled", "QUERY_LOG_DISABLED"
			result.DisabledNodes++
			result.Nodes = append(result.Nodes, coverage)
			continue
		}
		if checkpoint.LastSuccessAt != nil {
			result.IncludedNodes++
			if checkpoint.SourceNewestAt != nil && (result.CurrentThrough == nil || checkpoint.SourceNewestAt.Before(*result.CurrentThrough)) {
				at := *checkpoint.SourceNewestAt
				result.CurrentThrough = &at
			}
		}
		if checkpoint.LastStatus == "unsupported" {
			coverage.Status, coverage.ReasonCode = "unsupported", checkpoint.ErrorCode
			result.UnsupportedNodes++
		} else if checkpoint.LastStatus == "failed" {
			coverage.Status, coverage.ReasonCode = "error", checkpoint.ErrorCode
			result.ErrorNodes++
		} else if checkpoint.GapDetected {
			coverage.Status, coverage.ReasonCode = "gap", checkpoint.GapReason
			result.GapNodes++
		} else if checkpoint.LastSuccessAt == nil || now.Sub(*checkpoint.LastSuccessAt) > staleAfter {
			coverage.Status, coverage.ReasonCode = "stale", "QUERY_LOG_STALE"
			result.StaleNodes++
		} else {
			coverage.Status = "current"
		}
		result.Nodes = append(result.Nodes, coverage)
	}
	if result.ExpectedNodes == 0 || result.IncludedNodes == 0 {
		result.Status = "unavailable"
	} else if result.IncludedNodes < result.ExpectedNodes || result.StaleNodes+result.ErrorNodes+result.GapNodes+result.DisabledNodes > 0 {
		result.Status = "partial"
	}
	return result
}

func containsNode(nodes []domain.Node, id string) bool {
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

func encodeCursor(at time.Time, id string) string {
	body, _ := json.Marshal(cursorPayload{Version: 1, At: at.UTC().Format(time.RFC3339Nano), ID: id})
	return base64.RawURLEncoding.EncodeToString(body)
}

func decodeCursor(value string) (time.Time, string, error) {
	body, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(body) > 512 {
		return time.Time{}, "", fmt.Errorf("decode cursor")
	}
	var payload cursorPayload
	if json.Unmarshal(body, &payload) != nil || payload.Version != 1 || !domain.ValidID(payload.ID) {
		return time.Time{}, "", fmt.Errorf("decode cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, payload.At)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decode cursor")
	}
	return at.UTC(), payload.ID, nil
}

func SortTypes(values []string) {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
}

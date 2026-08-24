package telemetry

import (
	"context"
	"math"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

type Repository interface {
	ClusterByID(context.Context, string) (domain.Cluster, error)
	ListNodes(context.Context, string) ([]domain.Node, error)
	LatestStatisticsSnapshots(context.Context, string, Range, string) ([]Snapshot, error)
	LatestStatisticsAttempts(context.Context, string, string) ([]NodeAttempt, error)
}

type Service struct {
	repository   Repository
	pollInterval time.Duration
	timeout      time.Duration
	now          func() time.Time
	settings     interface {
		RuntimeSettings() systemsettings.RuntimeSettings
	}
}

func (s *Service) SetRuntimeSettings(provider interface {
	RuntimeSettings() systemsettings.RuntimeSettings
}) {
	s.settings = provider
}

func NewService(repository Repository, pollInterval, timeout time.Duration) *Service {
	return &Service{repository: repository, pollInterval: pollInterval, timeout: timeout, now: time.Now}
}

type Report struct {
	Range       Range          `json:"range"`
	Scope       Scope          `json:"scope"`
	State       string         `json:"state"`
	GeneratedAt time.Time      `json:"generatedAt"`
	Freshness   Freshness      `json:"freshness"`
	Coverage    Coverage       `json:"coverage"`
	Totals      Totals         `json:"totals"`
	Series      []SeriesPoint  `json:"series"`
	Rankings    Rankings       `json:"rankings"`
	Nodes       []NodeCoverage `json:"nodes"`
}

type Scope struct {
	Type   string `json:"type"`
	NodeID string `json:"nodeId,omitempty"`
}

type Freshness struct {
	NewestAt          *time.Time `json:"newestAt,omitempty"`
	OldestAt          *time.Time `json:"oldestAt,omitempty"`
	StaleAfterSeconds int64      `json:"staleAfterSeconds"`
}

type Coverage struct {
	Status           string `json:"status"`
	ExpectedNodes    int    `json:"expectedNodes"`
	IncludedNodes    int    `json:"includedNodes"`
	MissingNodes     int    `json:"missingNodes"`
	StaleNodes       int    `json:"staleNodes"`
	UnsupportedNodes int    `json:"unsupportedNodes"`
	MaintenanceNodes int    `json:"maintenanceNodes"`
}

type Totals struct {
	DNSQueries                   int64   `json:"dnsQueries"`
	BlockedFiltering             int64   `json:"blockedFiltering"`
	BlockedPercentage            float64 `json:"blockedPercentage"`
	ReplacedSafeBrowsing         int64   `json:"replacedSafeBrowsing"`
	ReplacedSafeSearch           int64   `json:"replacedSafeSearch"`
	ReplacedParental             int64   `json:"replacedParental"`
	SafetyInterventions          int64   `json:"safetyInterventions"`
	SafetyInterventionPercentage float64 `json:"safetyInterventionPercentage"`
	AverageProcessingMS          float64 `json:"averageProcessingMs"`
}

type SeriesPoint struct {
	At                   time.Time `json:"at"`
	DNSQueries           int64     `json:"dnsQueries"`
	BlockedFiltering     int64     `json:"blockedFiltering"`
	ReplacedSafeBrowsing int64     `json:"replacedSafeBrowsing"`
	ReplacedParental     int64     `json:"replacedParental"`
	IncludedNodes        int       `json:"includedNodes"`
}

type Ranking struct {
	Key        string  `json:"key"`
	Value      float64 `json:"value"`
	Percentage float64 `json:"percentage,omitempty"`
}

type Rankings struct {
	QueriedDomains           []Ranking `json:"queriedDomains"`
	BlockedDomains           []Ranking `json:"blockedDomains"`
	Clients                  []Ranking `json:"clients"`
	UpstreamResponses        []Ranking `json:"upstreamResponses"`
	UpstreamAverageLatencyMS []Ranking `json:"upstreamAverageLatencyMs"`
}

type NodeCoverage struct {
	NodeID      string     `json:"nodeId"`
	NodeName    string     `json:"nodeName"`
	Status      string     `json:"status"`
	ReasonCode  string     `json:"reasonCode,omitempty"`
	CollectedAt *time.Time `json:"collectedAt,omitempty"`
	DNSQueries  int64      `json:"dnsQueries,omitempty"`
}

func (s *Service) Statistics(ctx context.Context, clusterID string, window Range, nodeID string, limit int) (Report, error) {
	if !domain.ValidID(clusterID) {
		return Report{}, domain.Validation("clusterId", "must be a valid UUID")
	}
	if _, ok := ParseRange(string(window)); !ok {
		return Report{}, domain.Validation("range", "must be 24h, 7d, or 30d")
	}
	if nodeID != "" && !domain.ValidID(nodeID) {
		return Report{}, domain.Validation("nodeId", "must be a valid UUID")
	}
	if limit < 1 || limit > 25 {
		return Report{}, domain.Validation("limit", "must be between 1 and 25")
	}
	if _, err := s.repository.ClusterByID(ctx, clusterID); err != nil {
		return Report{}, err
	}
	nodes, err := s.repository.ListNodes(ctx, clusterID)
	if err != nil {
		return Report{}, err
	}
	if nodeID != "" {
		found := false
		for _, node := range nodes {
			found = found || node.ID == nodeID
		}
		if !found {
			return Report{}, domain.NewError(domain.ErrorNotFound, "node was not found in the cluster")
		}
	}
	snapshots, err := s.repository.LatestStatisticsSnapshots(ctx, clusterID, window, nodeID)
	if err != nil {
		return Report{}, err
	}
	attempts, err := s.repository.LatestStatisticsAttempts(ctx, clusterID, nodeID)
	if err != nil {
		return Report{}, err
	}
	return s.aggregate(window, nodeID, limit, nodes, snapshots, attempts), nil
}

func (s *Service) aggregate(window Range, nodeID string, limit int, nodes []domain.Node, snapshots []Snapshot, attempts []NodeAttempt) Report {
	now := s.now().UTC()
	pollInterval := s.pollInterval
	if s.settings != nil {
		pollInterval = s.settings.RuntimeSettings().StatisticsPollInterval
	}
	staleAfter := 2*pollInterval + s.timeout
	if staleAfter < 3*time.Hour {
		staleAfter = 3 * time.Hour
	}
	report := Report{Range: window, Scope: Scope{Type: "cluster", NodeID: nodeID}, State: "unavailable", GeneratedAt: now,
		Freshness: Freshness{StaleAfterSeconds: int64(staleAfter.Seconds())}, Series: []SeriesPoint{}, Nodes: []NodeCoverage{}}
	if nodeID != "" {
		report.Scope.Type = "node"
	}
	snapshotByNode := make(map[string]Snapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotByNode[snapshot.NodeID] = snapshot
	}
	attemptByNode := make(map[string]NodeAttempt, len(attempts))
	for _, attempt := range attempts {
		attemptByNode[attempt.NodeID] = attempt
	}
	included := make([]Snapshot, 0, len(snapshots))
	for _, node := range nodes {
		if nodeID != "" && node.ID != nodeID {
			continue
		}
		coverage := NodeCoverage{NodeID: node.ID, NodeName: node.Name}
		if !node.Enabled {
			coverage.Status, coverage.ReasonCode = "excluded", "NODE_DISABLED"
			report.Nodes = append(report.Nodes, coverage)
			continue
		}
		report.Coverage.ExpectedNodes++
		if node.MaintenanceMode {
			coverage.Status, coverage.ReasonCode = "maintenance", "NODE_MAINTENANCE"
			report.Coverage.MaintenanceNodes++
			report.Nodes = append(report.Nodes, coverage)
			continue
		}
		attempt := attemptByNode[node.ID]
		snapshot, hasSnapshot := snapshotByNode[node.ID]
		if attempt.Status == "unsupported" && (!hasSnapshot || !attempt.CompletedAt.Before(snapshot.CollectedAt)) {
			coverage.Status, coverage.ReasonCode = "unsupported", attempt.RangeErrors[window]
			if coverage.ReasonCode == "" {
				coverage.ReasonCode = attempt.ErrorCode
			}
			if coverage.ReasonCode == "" {
				coverage.ReasonCode = "STATISTICS_EXACT_RANGE_UNSUPPORTED"
			}
			report.Coverage.UnsupportedNodes++
			report.Nodes = append(report.Nodes, coverage)
			continue
		}
		if hasSnapshot {
			at := snapshot.CollectedAt
			coverage.CollectedAt, coverage.DNSQueries = &at, snapshot.DNSQueries
			coverage.Status = "included"
			if now.Sub(snapshot.CollectedAt) > staleAfter {
				coverage.Status, coverage.ReasonCode = "stale", "STATISTICS_STALE"
				report.Coverage.StaleNodes++
			}
			report.Coverage.IncludedNodes++
			included = append(included, snapshot)
			report.Nodes = append(report.Nodes, coverage)
			continue
		}
		coverage.Status, coverage.ReasonCode = "missing", attempt.RangeErrors[window]
		if coverage.ReasonCode == "" {
			coverage.ReasonCode = attempt.ErrorCode
		}
		if attempt.Status == "unsupported" || coverage.ReasonCode == "STATISTICS_EXACT_RANGE_UNSUPPORTED" {
			coverage.Status = "unsupported"
			report.Coverage.UnsupportedNodes++
		} else {
			report.Coverage.MissingNodes++
		}
		report.Nodes = append(report.Nodes, coverage)
	}
	s.aggregateSnapshots(&report, included, limit)
	report.Coverage.Status = "complete"
	if report.Coverage.ExpectedNodes == 0 || report.Coverage.IncludedNodes == 0 {
		report.Coverage.Status = "unavailable"
		report.State = "unavailable"
	} else if report.Coverage.IncludedNodes < report.Coverage.ExpectedNodes || report.Coverage.StaleNodes > 0 {
		report.Coverage.Status = "partial"
		report.State = "partial"
	} else {
		report.State = "ready"
	}
	return report
}

func (s *Service) aggregateSnapshots(report *Report, snapshots []Snapshot, limit int) {
	queried, blocked, clients, upstreamCounts := map[string]float64{}, map[string]float64{}, map[string]float64{}, map[string]float64{}
	upstreamWeighted := map[string]float64{}
	type pointValue struct {
		dns, blocked, safe, parental int64
		nodes                        int
	}
	points := map[time.Time]pointValue{}
	var processingWeighted float64
	for _, snapshot := range snapshots {
		report.Totals.DNSQueries += snapshot.DNSQueries
		report.Totals.BlockedFiltering += snapshot.BlockedFiltering
		report.Totals.ReplacedSafeBrowsing += snapshot.ReplacedSafeBrowsing
		report.Totals.ReplacedSafeSearch += snapshot.ReplacedSafeSearch
		report.Totals.ReplacedParental += snapshot.ReplacedParental
		processingWeighted += snapshot.AverageProcessingSeconds * float64(snapshot.DNSQueries)
		if report.Freshness.NewestAt == nil || snapshot.CollectedAt.After(*report.Freshness.NewestAt) {
			at := snapshot.CollectedAt
			report.Freshness.NewestAt = &at
		}
		if report.Freshness.OldestAt == nil || snapshot.CollectedAt.Before(*report.Freshness.OldestAt) {
			at := snapshot.CollectedAt
			report.Freshness.OldestAt = &at
		}
		mergeRanked(queried, snapshot.TopQueriedDomains, normalizeDomain)
		mergeRanked(blocked, snapshot.TopBlockedDomains, normalizeDomain)
		mergeRanked(clients, snapshot.TopClients, normalizeClient)
		mergeRanked(upstreamCounts, snapshot.TopUpstreamResponses, normalizeUpstream)
		averages := rankedMap(snapshot.TopUpstreamAverageSeconds, normalizeUpstream)
		for key, count := range rankedMap(snapshot.TopUpstreamResponses, normalizeUpstream) {
			if average, ok := averages[key]; ok {
				upstreamWeighted[key] += average * count
			}
		}
		step := time.Hour
		if snapshot.TimeUnit == "days" {
			step = 24 * time.Hour
		}
		for index := range snapshot.DNSQueriesSeries {
			at := snapshot.SourceEndedAt.Add(-time.Duration(len(snapshot.DNSQueriesSeries)-index) * step)
			point := points[at]
			point.dns += snapshot.DNSQueriesSeries[index]
			point.blocked += snapshot.BlockedFilteringSeries[index]
			point.safe += snapshot.ReplacedSafeBrowsingSeries[index]
			point.parental += snapshot.ReplacedParentalSeries[index]
			point.nodes++
			points[at] = point
		}
	}
	report.Totals.SafetyInterventions = report.Totals.ReplacedSafeBrowsing + report.Totals.ReplacedSafeSearch + report.Totals.ReplacedParental
	if report.Totals.DNSQueries > 0 {
		report.Totals.BlockedPercentage = percentage(float64(report.Totals.BlockedFiltering), float64(report.Totals.DNSQueries))
		report.Totals.SafetyInterventionPercentage = percentage(float64(report.Totals.SafetyInterventions), float64(report.Totals.DNSQueries))
		report.Totals.AverageProcessingMS = round(processingWeighted/float64(report.Totals.DNSQueries)*1000, 3)
	}
	for at, value := range points {
		report.Series = append(report.Series, SeriesPoint{At: at, DNSQueries: value.dns, BlockedFiltering: value.blocked,
			ReplacedSafeBrowsing: value.safe, ReplacedParental: value.parental, IncludedNodes: value.nodes})
	}
	sort.Slice(report.Series, func(i, j int) bool { return report.Series[i].At.Before(report.Series[j].At) })
	report.Rankings.QueriedDomains = rankings(queried, float64(report.Totals.DNSQueries), limit, 1)
	report.Rankings.BlockedDomains = rankings(blocked, float64(report.Totals.BlockedFiltering), limit, 1)
	report.Rankings.Clients = rankings(clients, float64(report.Totals.DNSQueries), limit, 1)
	report.Rankings.UpstreamResponses = rankings(upstreamCounts, sumMap(upstreamCounts), limit, 1)
	upstreamAverages := map[string]float64{}
	for key, weighted := range upstreamWeighted {
		if upstreamCounts[key] > 0 {
			upstreamAverages[key] = weighted / upstreamCounts[key] * 1000
		}
	}
	report.Rankings.UpstreamAverageLatencyMS = rankings(upstreamAverages, 0, limit, 3)
}

func mergeRanked(target map[string]float64, values []RankedValue, normalize func(string) string) {
	for _, value := range values {
		if key := normalize(value.Key); key != "" {
			target[key] += value.Value
		}
	}
}
func rankedMap(values []RankedValue, normalize func(string) string) map[string]float64 {
	result := map[string]float64{}
	mergeRanked(result, values, normalize)
	return result
}
func normalizeDomain(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}
func normalizeClient(value string) string {
	value = strings.TrimSpace(value)
	if address, err := netip.ParseAddr(value); err == nil {
		return address.String()
	}
	return value
}
func normalizeUpstream(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func sumMap(values map[string]float64) float64 {
	var result float64
	for _, value := range values {
		result += value
	}
	return result
}
func percentage(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return round(numerator/denominator*100, 2)
}
func round(value float64, places int) float64 {
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}
func rankings(values map[string]float64, denominator float64, limit, places int) []Ranking {
	result := make([]Ranking, 0, len(values))
	for key, value := range values {
		result = append(result, Ranking{Key: key, Value: round(value, places), Percentage: percentage(value, denominator)})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Value == result[j].Value {
			return result[i].Key < result[j].Key
		}
		return result[i].Value > result[j].Value
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

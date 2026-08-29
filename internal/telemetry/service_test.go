package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

const (
	testClusterID = "11111111-1111-4111-8111-111111111111"
	testNodeOne   = "22222222-2222-4222-8222-222222222222"
	testNodeTwo   = "33333333-3333-4333-8333-333333333333"
	testNodeThree = "44444444-4444-4444-8444-444444444444"
)

type repositoryFake struct {
	nodes     []domain.Node
	snapshots []Snapshot
	attempts  []NodeAttempt
}

func (r repositoryFake) ClusterByID(context.Context, string) (domain.Cluster, error) {
	return domain.Cluster{ID: testClusterID}, nil
}
func (r repositoryFake) ListNodes(context.Context, string) ([]domain.Node, error) {
	return r.nodes, nil
}
func (r repositoryFake) LatestStatisticsSnapshots(context.Context, string, Range, string) ([]Snapshot, error) {
	return r.snapshots, nil
}
func (r repositoryFake) LatestStatisticsAttempts(context.Context, string, string) ([]NodeAttempt, error) {
	return r.attempts, nil
}

func TestStatisticsAggregatesWeightedValuesAndCoverage(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 30, 0, 0, time.UTC)
	end := now.Truncate(time.Hour).Add(time.Hour)
	repository := repositoryFake{
		nodes: []domain.Node{
			{ID: testNodeOne, ClusterID: testClusterID, Name: "one", Enabled: true},
			{ID: testNodeTwo, ClusterID: testClusterID, Name: "two", Enabled: true},
			{ID: testNodeThree, ClusterID: testClusterID, Name: "old", Enabled: true},
		},
		snapshots: []Snapshot{
			statisticsFixture(testNodeOne, "one", now, end, 100, 10, .010, 80, .020, "Example.COM."),
			statisticsFixture(testNodeTwo, "two", now.Add(-time.Minute), end, 300, 30, .030, 20, .040, "example.com"),
		},
		attempts: []NodeAttempt{{NodeID: testNodeThree, Status: "unsupported", ErrorCode: "STATISTICS_EXACT_RANGE_UNSUPPORTED", CompletedAt: now}},
	}
	service := NewService(repository, time.Hour, 10*time.Second)
	service.now = func() time.Time { return now }
	report, err := service.Statistics(context.Background(), testClusterID, Range24Hours, "", 10)
	if err != nil {
		t.Fatalf("Statistics() error = %v", err)
	}
	if report.Totals.DNSQueries != 400 || report.Totals.BlockedFiltering != 40 || report.Totals.BlockedPercentage != 10 {
		t.Fatalf("totals = %+v", report.Totals)
	}
	if report.Totals.AverageProcessingMS != 25 {
		t.Fatalf("AverageProcessingMS = %v, want 25", report.Totals.AverageProcessingMS)
	}
	if report.Coverage.ExpectedNodes != 3 || report.Coverage.IncludedNodes != 2 || report.Coverage.UnsupportedNodes != 1 || report.State != "partial" {
		t.Fatalf("coverage/state = %+v %q", report.Coverage, report.State)
	}
	if len(report.Rankings.QueriedDomains) != 1 || report.Rankings.QueriedDomains[0].Key != "example.com" || report.Rankings.QueriedDomains[0].Value != 400 {
		t.Fatalf("queried domains = %+v", report.Rankings.QueriedDomains)
	}
	if len(report.Rankings.Clients) != 1 || report.Rankings.Clients[0].Value != 400 {
		t.Fatalf("clients = %+v", report.Rankings.Clients)
	}
	if len(report.Rankings.UpstreamAverageLatencyMS) != 1 || report.Rankings.UpstreamAverageLatencyMS[0].Value != 24 {
		t.Fatalf("upstream averages = %+v", report.Rankings.UpstreamAverageLatencyMS)
	}
	if len(report.Series) != 2 || report.Series[0].DNSQueries != 40 || report.Series[1].DNSQueries != 360 || report.Series[0].IncludedNodes != 2 {
		t.Fatalf("series = %+v", report.Series)
	}
}

func TestStatisticsNodeScopeRejectsNodeOutsideCluster(t *testing.T) {
	service := NewService(repositoryFake{}, time.Hour, time.Second)
	_, err := service.Statistics(context.Background(), testClusterID, Range24Hours, testNodeOne, 10)
	if err == nil {
		t.Fatal("Statistics() accepted a node outside the cluster")
	}
}

func TestStatisticsFreshnessUsesLivePersistedPollInterval(t *testing.T) {
	service := NewService(repositoryFake{}, time.Hour, 10*time.Second)
	runtime := systemsettings.NewRuntimeStore(systemsettings.RuntimeSettings{
		NodeHealthInterval: 30 * time.Second, StatisticsPollInterval: 24 * time.Hour,
		QueryLogCollection: true, QueryLogPollInterval: 30 * time.Second, QueryLogRetention: 7 * 24 * time.Hour,
	})
	service.SetRuntimeSettings(runtime)
	report := service.aggregate(Range24Hours, "", 10, nil, nil, nil)
	if report.Freshness.StaleAfterSeconds != int64((48*time.Hour+10*time.Second)/time.Second) {
		t.Fatalf("dynamic stale threshold = %d", report.Freshness.StaleAfterSeconds)
	}
}

func TestStatisticsReportsRangeBeyondNodeRetentionAsUnavailable(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 30, 0, 0, time.UTC)
	repository := repositoryFake{
		nodes: []domain.Node{{ID: testNodeOne, ClusterID: testClusterID, Name: "one", Enabled: true}},
		attempts: []NodeAttempt{{NodeID: testNodeOne, Status: "succeeded", CompletedAt: now,
			CollectedRanges: 1, RangeErrors: map[Range]string{Range7Days: ErrorRangeExceedsNodeRetention}}},
	}
	service := NewService(repository, time.Hour, time.Second)
	service.now = func() time.Time { return now }
	report, err := service.Statistics(context.Background(), testClusterID, Range7Days, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "unavailable" || len(report.Nodes) != 1 || report.Nodes[0].Status != "missing" || report.Nodes[0].ReasonCode != ErrorRangeExceedsNodeRetention {
		t.Fatalf("report = %#v", report)
	}
}

func TestStatisticsPreservesUnsupportedSourceReason(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 30, 0, 0, time.UTC)
	repository := repositoryFake{
		nodes: []domain.Node{{ID: testNodeOne, ClusterID: testClusterID, Name: "one", Enabled: true}},
		attempts: []NodeAttempt{{NodeID: testNodeOne, Status: "unsupported", ErrorCode: ErrorStatisticsDisabled,
			CompletedAt: now, RangeErrors: map[Range]string{Range24Hours: ErrorStatisticsDisabled}}},
	}
	service := NewService(repository, time.Hour, time.Second)
	service.now = func() time.Time { return now }
	report, err := service.Statistics(context.Background(), testClusterID, Range24Hours, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Nodes) != 1 || report.Nodes[0].Status != "unsupported" || report.Nodes[0].ReasonCode != ErrorStatisticsDisabled {
		t.Fatalf("report = %#v", report)
	}
}

func statisticsFixture(nodeID, name string, collectedAt, end time.Time, queries, blocked int64, average float64, upstreamCount, upstreamAverage float64, domainName string) Snapshot {
	return Snapshot{NodeID: nodeID, NodeName: name, ClusterID: testClusterID, Range: Range24Hours, CollectedAt: collectedAt,
		SourceStartedAt: end.Add(-24 * time.Hour), SourceEndedAt: end, SourceSnapshot: SourceSnapshot{
			TimeUnit: "hours", DNSQueries: queries, BlockedFiltering: blocked, AverageProcessingSeconds: average,
			TopQueriedDomains:         []RankedValue{{Key: domainName, Value: float64(queries)}},
			TopClients:                []RankedValue{{Key: "192.0.2.10", Value: float64(queries)}},
			TopUpstreamResponses:      []RankedValue{{Key: "1.1.1.1:53", Value: upstreamCount}},
			TopUpstreamAverageSeconds: []RankedValue{{Key: "1.1.1.1:53", Value: upstreamAverage}},
			DNSQueriesSeries:          []int64{queries / 10, queries - queries/10}, BlockedFilteringSeries: []int64{blocked / 10, blocked - blocked/10},
			ReplacedSafeBrowsingSeries: []int64{0, 0}, ReplacedParentalSeries: []int64{0, 0},
		}}
}

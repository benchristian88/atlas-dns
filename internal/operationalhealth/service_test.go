package operationalhealth

import (
	"context"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
	"github.com/benchristian88/atlas-dns/internal/inventory"
	"github.com/benchristian88/atlas-dns/internal/querylog"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
	"github.com/benchristian88/atlas-dns/internal/telemetry"
)

type repositoryFake struct {
	nodes       []domain.Node
	snapshots   []inventory.Snapshot
	attempts    []telemetry.NodeAttempt
	checkpoints []querylog.Checkpoint
	dnsProbes   []haoperations.DNSProbeResult
	database    Database
}

func (repositoryFake) ClusterByID(context.Context, string) (domain.Cluster, error) {
	return domain.Cluster{}, nil
}
func (r repositoryFake) ListNodes(context.Context, string) ([]domain.Node, error) {
	return r.nodes, nil
}
func (r repositoryFake) LatestSnapshots(context.Context, string) ([]inventory.Snapshot, error) {
	return r.snapshots, nil
}
func (r repositoryFake) LatestSuccessfulSnapshots(context.Context, string) ([]inventory.Snapshot, error) {
	return r.snapshots, nil
}
func (repositoryFake) CapabilityProfiles(context.Context, string) ([]inventory.CapabilityProfile, error) {
	return nil, nil
}
func (r repositoryFake) LatestStatisticsAttempts(context.Context, string, string) ([]telemetry.NodeAttempt, error) {
	return r.attempts, nil
}
func (r repositoryFake) QueryLogCheckpoints(context.Context, string, string) ([]querylog.Checkpoint, error) {
	return r.checkpoints, nil
}
func (r repositoryFake) LatestDNSProbes(context.Context, string) ([]haoperations.DNSProbeResult, error) {
	return r.dnsProbes, nil
}
func (r repositoryFake) OperationalDatabase(context.Context, time.Duration, time.Duration) (Database, error) {
	return r.database, nil
}

func TestStatusAggregatesStaleGapAndUnreachableWithoutFailingController(t *testing.T) {
	now := time.Date(2026, 8, 9, 1, 0, 0, 0, time.UTC)
	old := now.Add(-4 * time.Hour)
	nodeID := "22222222-2222-4222-8222-222222222222"
	repository := repositoryFake{
		nodes:       []domain.Node{{ID: nodeID, Name: "dns-secondary", Enabled: true, HealthStatus: domain.NodeUnreachable, LastPolledAt: &now, LastSeenAt: &old, LastErrorCode: "NODE_UNREACHABLE"}},
		snapshots:   []inventory.Snapshot{{NodeID: nodeID, ObservedAt: old, CollectionStatus: "succeeded"}},
		attempts:    []telemetry.NodeAttempt{{NodeID: nodeID, Status: "succeeded", CompletedAt: old, CollectedRanges: 3}},
		checkpoints: []querylog.Checkpoint{{NodeID: nodeID, LastAttemptAt: old, LastSuccessAt: &old, LastStatus: "succeeded", SourceNewestAt: &old, GapDetected: true, GapReason: "QUERY_LOG_NODE_RETENTION_GAP"}},
		database:    Database{State: Healthy},
	}
	service := NewService(repository, NewTracker(), Options{NodeInterval: 30 * time.Second, RequestTimeout: 10 * time.Second, StatisticsInterval: time.Hour, QueryLogInterval: 30 * time.Second, QueryLogEnabled: true})
	service.now = func() time.Time { return now }
	status, err := service.Status(context.Background(), "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if status.Summary.State != Degraded || status.Nodes[0].State != Failed || status.Observation.Nodes[0].State != Stale || status.Statistics.Nodes[0].State != Stale || status.QueryLog.Nodes[0].State != Stale {
		t.Fatalf("unexpected status aggregation: %#v", status)
	}
	if !status.QueryLog.Nodes[0].GapDetected || status.QueryLog.Nodes[0].GapReason == "" {
		t.Fatal("known gap was not retained")
	}
}

func TestDatabaseFailureFailsOverallHealth(t *testing.T) {
	repository := repositoryFake{database: Database{State: Failed, ErrorCode: "DATABASE_UNAVAILABLE"}}
	service := NewService(repository, NewTracker(), Options{})
	status, err := service.Status(context.Background(), "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if status.Summary.State != Failed || !status.Summary.ActionRequired {
		t.Fatalf("summary = %#v", status.Summary)
	}
}

func TestDNSHealthUsesLiveRuntimeFreshnessWindow(t *testing.T) {
	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	probedAt := now.Add(-5 * time.Minute)
	nodeID := "22222222-2222-4222-8222-222222222222"
	repository := repositoryFake{
		nodes:     []domain.Node{{ID: nodeID, Name: "dns-primary", Enabled: true, HealthStatus: domain.NodeHealthy}},
		dnsProbes: []haoperations.DNSProbeResult{{NodeID: nodeID, Status: "healthy", ProbedAt: probedAt}},
		database:  Database{State: Healthy},
	}
	runtime := systemsettings.NewRuntimeStore(systemsettings.RuntimeSettings{
		NodeHealthInterval: 6 * time.Minute, StatisticsPollInterval: time.Hour,
		QueryLogCollection: true, QueryLogPollInterval: 30 * time.Second, QueryLogRetention: 24 * time.Hour,
	})
	service := NewService(repository, NewTracker(), Options{NodeInterval: 30 * time.Second, RequestTimeout: 10 * time.Second, StatisticsInterval: time.Hour, QueryLogInterval: 30 * time.Second, QueryLogEnabled: true})
	service.SetRuntimeSettings(runtime)
	service.now = func() time.Time { return now }

	status, err := service.Status(context.Background(), "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if status.DNSService.State != Healthy || status.DNSService.Nodes[0].State != Healthy {
		t.Fatalf("long-interval DNS health=%#v", status.DNSService)
	}

	runtime.Update(systemsettings.RuntimeSettings{
		NodeHealthInterval: 30 * time.Second, StatisticsPollInterval: time.Hour,
		QueryLogCollection: true, QueryLogPollInterval: 30 * time.Second, QueryLogRetention: 24 * time.Hour,
	})
	status, err = service.Status(context.Background(), "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if status.DNSService.State != Degraded || status.DNSService.Nodes[0].State != Stale {
		t.Fatalf("live short-interval DNS health=%#v", status.DNSService)
	}
}

func TestDNSHealthKeepsExplicitFailureAfterFreshnessWindow(t *testing.T) {
	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	nodeID := "22222222-2222-4222-8222-222222222222"
	result := dnsServiceHealth(
		[]domain.Node{{ID: nodeID, Name: "dns-primary", Enabled: true}},
		[]haoperations.DNSProbeResult{{NodeID: nodeID, Status: "failed", ErrorCode: "DNS_PROBE_UNREACHABLE", ProbedAt: now.Add(-20 * time.Minute)}},
		now, 2*time.Minute, 30*time.Second,
	)
	if result.State != Failed || result.Nodes[0].State != Failed || result.Nodes[0].ErrorCode != "DNS_PROBE_UNREACHABLE" {
		t.Fatalf("aged explicit failure health=%#v", result)
	}
}

func TestStatisticsEligibleRangeSuccessIsHealthy(t *testing.T) {
	now := time.Date(2026, 8, 9, 1, 0, 0, 0, time.UTC)
	nodeID := "22222222-2222-4222-8222-222222222222"
	result := statisticsHealth(
		[]domain.Node{{ID: nodeID, Name: "dns-primary", Enabled: true}},
		[]telemetry.NodeAttempt{{NodeID: nodeID, Status: "succeeded", CompletedAt: now, CollectedRanges: 1,
			RangeErrors: map[telemetry.Range]string{telemetry.Range7Days: telemetry.ErrorRangeExceedsNodeRetention}}},
		now, 3*time.Hour, time.Hour,
	)
	if result.State != Healthy || len(result.Nodes) != 1 || result.Nodes[0].State != Healthy || result.Nodes[0].RecordsReceived != 1 {
		t.Fatalf("statistics health = %#v", result)
	}
}

func TestStatisticsFreshSuccessSurvivesPostMaintenanceSkip(t *testing.T) {
	now := time.Date(2026, 8, 16, 1, 0, 0, 0, time.UTC)
	lastSuccess := now.Add(-30 * time.Minute)
	nodes := []domain.Node{
		{ID: "22222222-2222-4222-8222-222222222222", Name: "dns-primary", Enabled: true},
		{ID: "33333333-3333-4333-8333-333333333333", Name: "dns-secondary", Enabled: true},
	}
	result := statisticsHealth(nodes, []telemetry.NodeAttempt{
		{NodeID: nodes[0].ID, Status: "succeeded", CompletedAt: now, LastSuccessAt: &now, CollectedRanges: 3},
		{NodeID: nodes[1].ID, Status: "maintenance", ErrorCode: "NODE_MAINTENANCE", CompletedAt: now.Add(-10 * time.Minute), LastSuccessAt: &lastSuccess},
	}, now, 3*time.Hour, time.Hour)

	if result.State != Healthy || result.CurrentNodes != 2 || result.CoveragePercent != 100 {
		t.Fatalf("statistics health = %#v", result)
	}
	if result.Nodes[1].State != Healthy || result.Nodes[1].ErrorCode != "" || result.Nodes[1].LastSuccessAt == nil || !result.Nodes[1].LastSuccessAt.Equal(lastSuccess) {
		t.Fatalf("post-maintenance node = %#v", result.Nodes[1])
	}
	if summary := aggregate(Status{Database: Database{State: Healthy}, Statistics: result}); summary.State != Healthy || summary.ActionRequired {
		t.Fatalf("controller summary = %#v", summary)
	}
}

func TestStatisticsPostMaintenanceWithoutFreshSuccessDegrades(t *testing.T) {
	now := time.Date(2026, 8, 16, 1, 0, 0, 0, time.UTC)
	staleSuccess := now.Add(-4 * time.Hour)
	nodes := []domain.Node{
		{ID: "22222222-2222-4222-8222-222222222222", Name: "dns-primary", Enabled: true},
		{ID: "33333333-3333-4333-8333-333333333333", Name: "dns-secondary", Enabled: true},
	}
	result := statisticsHealth(nodes, []telemetry.NodeAttempt{
		{NodeID: nodes[0].ID, Status: "succeeded", CompletedAt: now, LastSuccessAt: &now, CollectedRanges: 3},
		{NodeID: nodes[1].ID, Status: "maintenance", ErrorCode: "NODE_MAINTENANCE", CompletedAt: now.Add(-10 * time.Minute), LastSuccessAt: &staleSuccess},
	}, now, 3*time.Hour, time.Hour)

	if result.State != Degraded || result.CurrentNodes != 1 || result.StaleNodes != 1 || result.Nodes[1].State != Stale {
		t.Fatalf("statistics health = %#v", result)
	}
}

func TestStatisticsPartialNodeDegradesCompleteCoverage(t *testing.T) {
	now := time.Date(2026, 8, 16, 1, 0, 0, 0, time.UTC)
	nodes := []domain.Node{
		{ID: "22222222-2222-4222-8222-222222222222", Name: "dns-primary", Enabled: true},
		{ID: "33333333-3333-4333-8333-333333333333", Name: "dns-secondary", Enabled: true},
	}
	result := statisticsHealth(nodes, []telemetry.NodeAttempt{
		{NodeID: nodes[0].ID, Status: "succeeded", CompletedAt: now, CollectedRanges: 3},
		{NodeID: nodes[1].ID, Status: "partial", ErrorCode: "STATISTICS_TIMEOUT", CompletedAt: now, CollectedRanges: 2},
	}, now, 3*time.Hour, time.Hour)

	if result.State != Degraded || result.CurrentNodes != 2 || result.CoveragePercent != 100 || result.Nodes[1].State != Degraded {
		t.Fatalf("statistics health = %#v", result)
	}
	if summary := aggregate(Status{Database: Database{State: Healthy}, Statistics: result}); summary.State != Degraded || !summary.ActionRequired {
		t.Fatalf("controller summary = %#v", summary)
	}
}

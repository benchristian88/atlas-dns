package querylog

import (
	"context"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

const (
	queryTestCluster = "11111111-1111-4111-8111-111111111111"
	queryTestNode    = "22222222-2222-4222-8222-222222222222"
	queryTestEvent   = "33333333-3333-4333-8333-333333333333"
)

type queryRepositoryFake struct {
	nodes       []domain.Node
	events      []Event
	checkpoints []Checkpoint
	query       EventQuery
}

func (r *queryRepositoryFake) ClusterByID(context.Context, string) (domain.Cluster, error) {
	return domain.Cluster{ID: queryTestCluster}, nil
}
func (r *queryRepositoryFake) ListNodes(context.Context, string) ([]domain.Node, error) {
	return r.nodes, nil
}
func (r *queryRepositoryFake) ListQueryEvents(_ context.Context, query EventQuery) ([]Event, error) {
	r.query = query
	return r.events, nil
}
func (r *queryRepositoryFake) QueryEventByID(context.Context, string, string) (Event, error) {
	return Event{ID: queryTestEvent}, nil
}
func (r *queryRepositoryFake) QueryLogCheckpoints(context.Context, string, string) ([]Checkpoint, error) {
	return r.checkpoints, nil
}
func (r *queryRepositoryFake) QueryLogTypes(context.Context, string, string) ([]string, error) {
	return []string{"A", "AAAA"}, nil
}

func TestListBuildsStableCursorAndForwardsFilters(t *testing.T) {
	now := time.Date(2026, 8, 9, 2, 0, 0, 0, time.UTC)
	repository := &queryRepositoryFake{
		nodes: []domain.Node{{ID: queryTestNode, Name: "dns-a", Enabled: true, Version: "v0.107.78"}},
		events: []Event{
			{ID: queryTestEvent, SourceTimestamp: now},
			{ID: "44444444-4444-4444-8444-444444444444", SourceTimestamp: now.Add(-time.Second)},
		},
		checkpoints: []Checkpoint{{NodeID: queryTestNode, LastAttemptAt: now, LastSuccessAt: &now, SourceNewestAt: &now, LastStatus: "succeeded"}},
	}
	service := NewService(repository, time.Minute)
	service.now = func() time.Time { return now }
	page, err := service.List(context.Background(), ListRequest{ClusterID: queryTestCluster, NodeID: queryTestNode, Search: "Example", Status: StatusBlocked, QueryType: "aaaa", Client: "client", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextCursor == "" || !page.Coverage.CollectionEnabled {
		t.Fatalf("page = %+v", page)
	}
	if repository.query.Limit != 2 || repository.query.QueryType != "AAAA" || repository.query.Status != StatusBlocked || repository.query.NodeID != queryTestNode {
		t.Fatalf("query = %+v", repository.query)
	}
	at, id, err := decodeCursor(page.NextCursor)
	if err != nil || !at.Equal(now) || id != queryTestEvent {
		t.Fatalf("cursor = %v %q %v", at, id, err)
	}
}

func TestCoverageReportsPartialAndCollectionDisabledHonestly(t *testing.T) {
	now := time.Date(2026, 8, 9, 2, 0, 0, 0, time.UTC)
	nodes := []domain.Node{
		{ID: queryTestNode, Name: "current", Enabled: true, Version: "v0.107.78"},
		{ID: "44444444-4444-4444-8444-444444444444", Name: "unsupported", Enabled: true, Version: "v0.107.51"},
	}
	service := NewService(&queryRepositoryFake{}, time.Minute)
	service.now = func() time.Time { return now }
	coverage := service.coverage(nodes, []Checkpoint{{NodeID: queryTestNode, LastAttemptAt: now, LastSuccessAt: &now, SourceNewestAt: &now, LastStatus: "succeeded"}}, "")
	if coverage.Status != "partial" || coverage.IncludedNodes != 1 || coverage.UnsupportedNodes != 1 {
		t.Fatalf("coverage = %+v", coverage)
	}
	disabled := NewService(&queryRepositoryFake{}, time.Minute, Options{CollectionEnabled: false, Retention: 24 * time.Hour})
	disabled.now = func() time.Time { return now }
	coverage = disabled.coverage(nodes[:1], nil, "")
	if coverage.CollectionEnabled || coverage.Status != "unavailable" || coverage.Nodes[0].ReasonCode != "QUERY_LOG_COLLECTION_DISABLED" {
		t.Fatalf("disabled coverage = %+v", coverage)
	}
}

func TestCoverageClassifiesStaleDisabledGapErrorAndMaintenance(t *testing.T) {
	now := time.Date(2026, 8, 9, 2, 0, 0, 0, time.UTC)
	ids := []string{
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
		"55555555-5555-4555-8555-555555555555",
		"66666666-6666-4666-8666-666666666666",
	}
	nodes := make([]domain.Node, len(ids))
	for index, id := range ids {
		nodes[index] = domain.Node{ID: id, Name: id[:4], Enabled: true, Version: "v0.107.78"}
	}
	nodes[4].MaintenanceMode = true
	stale := now.Add(-10 * time.Minute)
	disabled := false
	checkpoints := []Checkpoint{
		{NodeID: ids[0], LastAttemptAt: stale, LastSuccessAt: &stale, SourceNewestAt: &stale, LastStatus: "succeeded"},
		{NodeID: ids[1], LastAttemptAt: now, LoggingEnabled: &disabled, LastStatus: "logging_disabled"},
		{NodeID: ids[2], LastAttemptAt: now, LastSuccessAt: &now, SourceNewestAt: &now, LastStatus: "succeeded", GapDetected: true, GapReason: "QUERY_LOG_NODE_RETENTION_GAP"},
		{NodeID: ids[3], LastAttemptAt: now, LastSuccessAt: &stale, SourceNewestAt: &stale, LastStatus: "failed", ErrorCode: "NODE_UNREACHABLE"},
	}
	service := NewService(&queryRepositoryFake{}, time.Minute)
	service.now = func() time.Time { return now }
	coverage := service.coverage(nodes, checkpoints, "")
	if coverage.StaleNodes != 1 || coverage.DisabledNodes != 1 || coverage.GapNodes != 1 || coverage.ErrorNodes != 1 || coverage.MaintenanceNodes != 1 || coverage.Status != "partial" {
		t.Fatalf("coverage = %+v", coverage)
	}
}

func TestListRejectsInvalidCursorAndNodeOutsideCluster(t *testing.T) {
	service := NewService(&queryRepositoryFake{}, time.Minute)
	if _, err := service.List(context.Background(), ListRequest{ClusterID: queryTestCluster, Cursor: "bad", Limit: 50}); err == nil {
		t.Fatal("invalid cursor was accepted")
	}
	if _, err := service.List(context.Background(), ListRequest{ClusterID: queryTestCluster, NodeID: queryTestNode, Limit: 50}); err == nil {
		t.Fatal("node outside cluster was accepted")
	}
}

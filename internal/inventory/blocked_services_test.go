package inventory

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

const catalogueClusterID = "11111111-1111-4111-8111-111111111111"

type catalogueReaderFake struct {
	unusedReader
	byURL map[string]NodeBlockedServicesCatalogue
	err   error
	calls atomic.Int64
}

func (f *catalogueReaderFake) ReadBlockedServicesCatalogue(_ context.Context, request domain.NodeProbeRequest, _ string) (NodeBlockedServicesCatalogue, error) {
	f.calls.Add(1)
	if f.err != nil {
		return NodeBlockedServicesCatalogue{}, f.err
	}
	return f.byURL[request.BaseURL], nil
}

func catalogueNode(id, name, version, baseURL string) (domain.Node, domain.NodeRecord, CapabilityProfile) {
	node := domain.Node{ID: id, ClusterID: catalogueClusterID, Name: name, Version: version, Enabled: true}
	record := domain.NodeRecord{Node: node}
	record.Node.BaseURL = baseURL
	profile := CapabilityProfile{NodeID: id, ProductVersion: version, Compatibility: string(domain.CompatibilitySupported), SchemaVersion: 2, Features: map[string]bool{"blocked_services": true}}
	return node, record, profile
}

func TestBlockedServicesCatalogueMergesNodeSupportAndPreservesPartialState(t *testing.T) {
	nodeA, recordA, profileA := catalogueNode("22222222-2222-4222-8222-222222222222", "Primary", "v0.107.78", "http://primary.test")
	nodeB, recordB, profileB := catalogueNode("33333333-3333-4333-8333-333333333333", "Secondary", "v0.107.78", "http://secondary.test")
	repository := &fakeRepository{
		nodes: []domain.Node{nodeA, nodeB}, profiles: []CapabilityProfile{profileA, profileB},
		nodeRecords: map[string]domain.NodeRecord{nodeA.ID: recordA, nodeB.ID: recordB},
	}
	reader := &catalogueReaderFake{byURL: map[string]NodeBlockedServicesCatalogue{
		recordA.Node.BaseURL: {Services: []BlockedServiceMetadata{{ID: "chatgpt", Name: "ChatGPT", GroupID: "ai"}, {ID: "youtube", Name: "YouTube", GroupID: "streaming"}}, Groups: []BlockedServiceGroup{{ID: "ai"}, {ID: "streaming"}}},
		recordB.Node.BaseURL: {Services: []BlockedServiceMetadata{{ID: "youtube", Name: "YouTube"}}},
	}}
	service := NewService(repository, unusedCredentials{}, reader)
	catalogue, err := service.BlockedServicesCatalogue(context.Background(), catalogueClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls.Load() != 2 || catalogue.Partial || catalogue.Stale || len(catalogue.Services) != 2 || len(catalogue.Nodes) != 2 {
		t.Fatalf("unexpected merged catalogue: %#v calls=%d", catalogue, reader.calls.Load())
	}
	if got := catalogue.Services[0]; got.ID != "chatgpt" || len(got.SupportedNodeIDs) != 1 || len(got.UnsupportedNodeIDs) != 1 || got.UnsupportedNodeIDs[0] != nodeB.ID {
		t.Fatalf("unexpected ChatGPT compatibility: %#v", got)
	}
	issues, err := service.ValidateBlockedServiceIDs(context.Background(), catalogueClusterID, []string{"chatgpt", "legacy-id"})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 3 {
		t.Fatalf("issues = %#v, want ChatGPT on one node and legacy-id on both", issues)
	}
}

func TestBlockedServicesCatalogueCacheRefreshesForVersionAndFallsBackAsStale(t *testing.T) {
	node, record, profile := catalogueNode("22222222-2222-4222-8222-222222222222", "Primary", "v0.107.78", "http://primary.test")
	repository := &fakeRepository{nodes: []domain.Node{node}, profiles: []CapabilityProfile{profile}, nodeRecords: map[string]domain.NodeRecord{node.ID: record}}
	reader := &catalogueReaderFake{byURL: map[string]NodeBlockedServicesCatalogue{record.Node.BaseURL: {Services: []BlockedServiceMetadata{{ID: "youtube", Name: "YouTube"}}}}}
	service := NewService(repository, unusedCredentials{}, reader)
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.catalogueTTL = time.Minute

	if _, err := service.BlockedServicesCatalogue(context.Background(), catalogueClusterID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.BlockedServicesCatalogue(context.Background(), catalogueClusterID); err != nil {
		t.Fatal(err)
	}
	if reader.calls.Load() != 1 {
		t.Fatalf("cache miss: calls=%d", reader.calls.Load())
	}

	now = now.Add(2 * time.Minute)
	reader.err = errors.New("node unavailable")
	stale, err := service.BlockedServicesCatalogue(context.Background(), catalogueClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Stale || !stale.Partial || stale.Nodes[0].Status != "stale" || reader.calls.Load() != 2 {
		t.Fatalf("unexpected stale fallback: %#v calls=%d", stale, reader.calls.Load())
	}

	reader.err = nil
	repository.nodes[0].Version = "v0.107.78"
	repository.profiles[0].ProductVersion = "v0.107.78"
	refreshed, err := service.BlockedServicesCatalogue(context.Background(), catalogueClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Stale || reader.calls.Load() != 3 {
		t.Fatalf("version change did not refresh cache: %#v calls=%d", refreshed, reader.calls.Load())
	}
	repository.profiles[0].Features["blocked_services"] = false
	if _, err := service.BlockedServicesCatalogue(context.Background(), catalogueClusterID); err != nil {
		t.Fatal(err)
	}
	if reader.calls.Load() != 4 {
		t.Fatalf("capability change did not refresh cache: calls=%d", reader.calls.Load())
	}
}

func TestBlockedServicesCatalogueReportsUnsupportedVersionPerNode(t *testing.T) {
	node, record, profile := catalogueNode("22222222-2222-4222-8222-222222222222", "Future", "v0.108.0", "http://future.test")
	repository := &fakeRepository{nodes: []domain.Node{node}, profiles: []CapabilityProfile{profile}, nodeRecords: map[string]domain.NodeRecord{node.ID: record}}
	reader := &catalogueReaderFake{err: domain.NewError(domain.ErrorCapability, "unsupported version")}
	service := NewService(repository, unusedCredentials{}, reader)
	catalogue, err := service.BlockedServicesCatalogue(context.Background(), catalogueClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if !catalogue.Partial || catalogue.Nodes[0].Status != "unsupported" || catalogue.Nodes[0].ErrorCode != string(domain.ErrorCapability) {
		t.Fatalf("unexpected unsupported result: %#v", catalogue)
	}
}

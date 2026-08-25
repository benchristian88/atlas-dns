package inventory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
)

type fakeRepository struct {
	snapshot    Snapshot
	node        domain.Node
	nodes       []domain.Node
	profiles    []CapabilityProfile
	draft       *Draft
	imported    bool
	nodeRecord  domain.NodeRecord
	nodeRecords map[string]domain.NodeRecord
	audits      []domain.AuditEvent
}

func (f *fakeRepository) SnapshotByID(context.Context, string) (Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeRepository) NodeByID(context.Context, string) (domain.Node, error) { return f.node, nil }
func (f *fakeRepository) DraftByCluster(context.Context, string) (Draft, error) {
	if f.draft == nil {
		return Draft{}, domain.NewError(domain.ErrorNotFound, "configuration draft was not found")
	}
	return *f.draft, nil
}
func (f *fakeRepository) ImportDraft(_ context.Context, d Draft, _ int, _ domain.AuditEvent) error {
	f.imported = true
	f.draft = &d
	return nil
}
func (*fakeRepository) CreateCluster(context.Context, domain.Cluster, domain.AuditEvent) error {
	return nil
}
func (*fakeRepository) ListClusters(context.Context) ([]domain.Cluster, error) { return nil, nil }
func (*fakeRepository) ClusterByID(context.Context, string) (domain.Cluster, error) {
	return domain.Cluster{}, nil
}
func (*fakeRepository) UpdateCluster(context.Context, domain.Cluster, int, domain.AuditEvent) error {
	return nil
}
func (*fakeRepository) CreateNode(context.Context, domain.NodeRecord, domain.AuditEvent) error {
	return nil
}
func (f *fakeRepository) ListNodes(context.Context, string) ([]domain.Node, error) {
	return f.nodes, nil
}
func (f *fakeRepository) NodeRecordByID(_ context.Context, id string) (domain.NodeRecord, error) {
	if record, ok := f.nodeRecords[id]; ok {
		return record, nil
	}
	return f.nodeRecord, nil
}
func (*fakeRepository) UpdateNode(context.Context, domain.NodeRecord, int, domain.AuditEvent) error {
	return nil
}
func (*fakeRepository) SoftDeleteNode(context.Context, string, int, time.Time, domain.AuditEvent) error {
	return nil
}
func (*fakeRepository) UpdateNodeHealth(context.Context, string, domain.NodeHealth, domain.Compatibility, string, *int, string, time.Time, bool) error {
	return nil
}
func (*fakeRepository) RecordNodeTestResult(context.Context, string, domain.NodeHealth, domain.Compatibility, string, *int, string, time.Time, bool, domain.AuditEvent) error {
	return nil
}
func (*fakeRepository) SetNodeMaintenance(context.Context, string, bool, int, time.Time, domain.AuditEvent) error {
	return nil
}
func (*fakeRepository) SaveObservation(context.Context, Snapshot, CapabilityProfile) error {
	return nil
}
func (*fakeRepository) LatestSnapshots(context.Context, string) ([]Snapshot, error) { return nil, nil }
func (f *fakeRepository) CapabilityProfiles(context.Context, string) ([]CapabilityProfile, error) {
	return f.profiles, nil
}
func (f *fakeRepository) RecordAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.audits = append(f.audits, event)
	return nil
}

type unusedCredentials struct{}

func (unusedCredentials) Decrypt(string, domain.EncryptedCredentials) (domain.NodeCredentials, error) {
	return domain.NodeCredentials{}, nil
}

type unusedReader struct{}

func (unusedReader) ReadConfiguration(context.Context, domain.NodeProbeRequest, string) (configuration.Document, CapabilityProfile, error) {
	return configuration.Document{}, CapabilityProfile{}, nil
}

type refreshReader struct {
	unusedReader
	whitelist bool
	cancel    context.CancelFunc
}

func (r *refreshReader) RefreshFilters(_ context.Context, _ domain.NodeProbeRequest, whitelist bool) error {
	r.whitelist = whitelist
	if r.cancel != nil {
		r.cancel()
		return context.Canceled
	}
	return nil
}

func TestImportRequiresConfirmationAndCreatesOnlyDraft(t *testing.T) {
	clusterID := "11111111-1111-4111-8111-111111111111"
	nodeID := "22222222-2222-4222-8222-222222222222"
	snapshotID := "33333333-3333-4333-8333-333333333333"
	document := configuration.Document{SchemaVersion: 1, NodeSpecific: configuration.NodeSpecific{BindHosts: []string{"192.0.2.10"}, DNSPort: 53}}
	repo := &fakeRepository{snapshot: Snapshot{ID: snapshotID, NodeID: nodeID, Document: &document, CanonicalHash: "hash", CollectionStatus: "succeeded"}, node: domain.Node{ID: nodeID, ClusterID: clusterID}}
	service := NewService(repo, unusedCredentials{}, unusedReader{})
	actor := domain.Actor{UserID: "44444444-4444-4444-8444-444444444444", RequestID: "55555555-5555-4555-8555-555555555555"}
	if _, err := service.Import(context.Background(), actor, clusterID, snapshotID, 0, false); err == nil {
		t.Fatal("unconfirmed import succeeded")
	}
	draft, err := service.Import(context.Background(), actor, clusterID, snapshotID, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.imported || draft.Version != 1 || draft.SchemaVersion != configuration.SchemaVersion || draft.SourceSnapshotID != snapshotID || draft.Document.NodeOverrides[nodeID].DNSPort != 53 {
		t.Fatalf("unexpected draft: %#v", draft)
	}
}

func TestImportConvertsLegacyObservationIntoSchemaV2Draft(t *testing.T) {
	clusterID := "11111111-1111-4111-8111-111111111111"
	nodeID := "22222222-2222-4222-8222-222222222222"
	snapshotID := "33333333-3333-4333-8333-333333333333"
	legacy := configuration.Document{SchemaVersion: configuration.LegacySchemaVersion, NodeSpecific: configuration.NodeSpecific{BindHosts: []string{"192.0.2.10"}, DNSPort: 53}}
	current := &Draft{Version: 3, Document: configuration.DesiredDocument{SchemaVersion: configuration.SchemaVersion, NodeOverrides: map[string]configuration.NodeSpecific{}}}
	repo := &fakeRepository{snapshot: Snapshot{ID: snapshotID, NodeID: nodeID, Document: &legacy, CollectionStatus: "succeeded"}, node: domain.Node{ID: nodeID, ClusterID: clusterID}, draft: current}
	service := NewService(repo, unusedCredentials{}, unusedReader{})
	actor := domain.Actor{UserID: "44444444-4444-4444-8444-444444444444", RequestID: "55555555-5555-4555-8555-555555555555"}
	draft, err := service.Import(context.Background(), actor, clusterID, snapshotID, 3, true)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.imported || draft.SchemaVersion != configuration.SchemaVersion {
		t.Fatalf("legacy observation was not converted: %#v", draft)
	}
}

func TestImportRejectsSnapshotWithoutListenerIdentity(t *testing.T) {
	clusterID := "11111111-1111-4111-8111-111111111111"
	nodeID := "22222222-2222-4222-8222-222222222222"
	snapshotID := "33333333-3333-4333-8333-333333333333"
	document := configuration.Document{SchemaVersion: 1}
	repo := &fakeRepository{snapshot: Snapshot{ID: snapshotID, NodeID: nodeID, Document: &document, CanonicalHash: "hash", CollectionStatus: "succeeded"}, node: domain.Node{ID: nodeID, ClusterID: clusterID}}
	service := NewService(repo, unusedCredentials{}, unusedReader{})
	actor := domain.Actor{UserID: "44444444-4444-4444-8444-444444444444", RequestID: "55555555-5555-4555-8555-555555555555"}

	if _, err := service.Import(context.Background(), actor, clusterID, snapshotID, 0, true); err == nil {
		t.Fatal("snapshot without listener identity was imported")
	}
	if repo.imported {
		t.Fatal("invalid snapshot changed the draft")
	}
}

func TestRefreshFiltersAuditsRequestedAndSucceeded(t *testing.T) {
	const nodeID = "22222222-2222-4222-8222-222222222222"
	repo := &fakeRepository{nodeRecord: domain.NodeRecord{Node: domain.Node{ID: nodeID, Enabled: true, BaseURL: "http://node.test", CertificatePolicy: domain.CertificateInsecureHTTP}}}
	reader := &refreshReader{}
	service := NewService(repo, unusedCredentials{}, reader)
	actor := domain.Actor{UserID: "44444444-4444-4444-8444-444444444444", RequestID: "55555555-5555-4555-8555-555555555555"}
	if err := service.RefreshFilters(context.Background(), actor, nodeID, true); err != nil {
		t.Fatal(err)
	}
	if !reader.whitelist || len(repo.audits) != 2 || repo.audits[0].Action != "filters.refresh_requested" || repo.audits[1].Action != "filters.refresh_succeeded" {
		t.Fatalf("refresh result reader=%#v audits=%#v", reader, repo.audits)
	}
}

func TestRefreshFiltersAuditsFailureAfterRequestCancellation(t *testing.T) {
	const nodeID = "22222222-2222-4222-8222-222222222222"
	repo := &fakeRepository{nodeRecord: domain.NodeRecord{Node: domain.Node{ID: nodeID, Enabled: true, BaseURL: "http://node.test", CertificatePolicy: domain.CertificateInsecureHTTP}}}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &refreshReader{cancel: cancel}
	service := NewService(repo, unusedCredentials{}, reader)
	actor := domain.Actor{UserID: "44444444-4444-4444-8444-444444444444", RequestID: "55555555-5555-4555-8555-555555555555"}
	if err := service.RefreshFilters(ctx, actor, nodeID, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("RefreshFilters() error = %v, want context cancellation", err)
	}
	if len(repo.audits) != 2 || repo.audits[1].Action != "filters.refresh_failed" {
		t.Fatalf("terminal failure audit missing: %#v", repo.audits)
	}
}

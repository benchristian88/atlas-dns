package controlplane

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

const (
	reconcileClusterID  = "00000000-0000-4000-8000-000000000010"
	reconcileNodeID     = "00000000-0000-4000-8000-000000000011"
	reconcileRevisionID = "00000000-0000-4000-8000-000000000012"
)

type reconciliationRepositoryFake struct {
	Repository
	policy     domain.ReconciliationPolicy
	revision   Revision
	snapshot   inventory.Snapshot
	drift      *DriftEvent
	deployment *Deployment
}

func (f *reconciliationRepositoryFake) ListClusters(context.Context) ([]domain.Cluster, error) {
	active := reconcileRevisionID
	return []domain.Cluster{{ID: reconcileClusterID, ReconciliationPolicy: f.policy, ActiveRevisionID: &active}}, nil
}
func (*reconciliationRepositoryFake) ClusterHasActiveDeployment(context.Context, string) (bool, error) {
	return false, nil
}
func (f *reconciliationRepositoryFake) RevisionByID(context.Context, string) (Revision, error) {
	return f.revision, nil
}
func (*reconciliationRepositoryFake) ListNodes(context.Context, string) ([]domain.Node, error) {
	return []domain.Node{{ID: reconcileNodeID, ClusterID: reconcileClusterID, Enabled: true, CompatibilityStatus: domain.CompatibilitySupported}}, nil
}
func (*reconciliationRepositoryFake) UpdateNodeConvergence(context.Context, string, string, time.Time) error {
	return nil
}
func (f *reconciliationRepositoryFake) UpsertDriftEvent(_ context.Context, item DriftEvent, _ domain.AuditEvent) (DriftEvent, bool, error) {
	f.drift = &item
	return item, true, nil
}
func (*reconciliationRepositoryFake) ResolveNodeDrift(context.Context, string, string, time.Time, domain.AuditEvent) (bool, error) {
	return false, nil
}
func (f *reconciliationRepositoryFake) LatestSnapshots(context.Context, string) ([]inventory.Snapshot, error) {
	return []inventory.Snapshot{f.snapshot}, nil
}
func (*reconciliationRepositoryFake) CapabilityProfiles(context.Context, string) ([]inventory.CapabilityProfile, error) {
	return []inventory.CapabilityProfile{{NodeID: reconcileNodeID, Compatibility: string(domain.CompatibilitySupported), SchemaVersion: configuration.SchemaVersion, Features: map[string]bool{"dns": true, "filtering": true, "clients": true, "rewrites": true, "blocked_services": true, "safety": true, "query_log": true, "statistics": true, "tls": true, "safe_search_ecosia": true, "filter_interval_arbitrary": true, "cache_toggle": true, "upstream_timeout": true, "rewrite_toggle": true, "ignored_lists_toggle": true}}}, nil
}
func (f *reconciliationRepositoryFake) CreateDeployment(_ context.Context, deployment Deployment, _ domain.AuditEvent) error {
	f.deployment = &deployment
	return nil
}
func (*reconciliationRepositoryFake) UpdateDriftReconciliation(context.Context, string, string, *string) error {
	return nil
}

type reconciliationObserverFake struct{ snapshot inventory.Snapshot }

func (f reconciliationObserverFake) Observe(context.Context, string) (inventory.Snapshot, error) {
	return f.snapshot, nil
}

func TestReconcilerEnforceCreatesDurableTargetedDeployment(t *testing.T) {
	desired := configuration.DesiredDocument{SchemaVersion: configuration.SchemaVersion, Shared: configuration.Shared{DNS: configuration.DNS{UpstreamDNS: []string{"1.1.1.1"}}, Filtering: configuration.Filtering{UpdateInterval: 24}}, NodeOverrides: map[string]configuration.NodeSpecific{reconcileNodeID: {BindHosts: []string{"0.0.0.0"}, DNSPort: 53}}}
	observedDocument, err := configuration.Effective(desired, reconcileNodeID)
	if err != nil {
		t.Fatal(err)
	}
	observedDocument.Shared.DNS.UpstreamDNS = []string{"8.8.8.8"}
	_, observedHash, err := configuration.Marshal(observedDocument)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := inventory.Snapshot{ID: "00000000-0000-4000-8000-000000000013", NodeID: reconcileNodeID, Document: &observedDocument, CanonicalHash: observedHash, CollectionStatus: "succeeded"}
	repository := &reconciliationRepositoryFake{policy: domain.ReconciliationEnforce, revision: Revision{ID: reconcileRevisionID, ClusterID: reconcileClusterID, Document: desired}, snapshot: snapshot}
	service := NewService(repository)
	reconciler := NewReconciler(repository, service, reconciliationObserverFake{snapshot: snapshot}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repository.drift == nil || repository.drift.Policy != "enforce" {
		t.Fatalf("drift = %#v", repository.drift)
	}
	if repository.deployment == nil || repository.deployment.Origin != "reconciliation" || len(repository.deployment.Nodes) != 1 || repository.deployment.Nodes[0].NodeID != reconcileNodeID {
		t.Fatalf("deployment = %#v", repository.deployment)
	}
}

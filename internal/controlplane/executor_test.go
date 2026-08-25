package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

type executionRepositoryFake struct {
	deployment Deployment
	revision   Revision
	records    map[string]domain.NodeRecord
	activated  bool
}

func (f *executionRepositoryFake) ClaimDeployment(context.Context, time.Time) (string, error) {
	return f.deployment.ID, nil
}
func (f *executionRepositoryFake) DeploymentByID(context.Context, string) (Deployment, error) {
	return f.deployment, nil
}
func (f *executionRepositoryFake) RevisionByID(context.Context, string) (Revision, error) {
	return f.revision, nil
}
func (f *executionRepositoryFake) NodeRecordByID(_ context.Context, id string) (domain.NodeRecord, error) {
	return f.records[id], nil
}
func (f *executionRepositoryFake) SetDeploymentRunning(context.Context, string) error {
	f.deployment.Status = "running"
	return nil
}
func (f *executionRepositoryFake) UpdateDeploymentNode(_ context.Context, node DeploymentNode) error {
	for index := range f.deployment.Nodes {
		if f.deployment.Nodes[index].ID == node.ID {
			f.deployment.Nodes[index] = node
		}
	}
	return nil
}
func (*executionRepositoryFake) MarkNodeApplied(context.Context, string, string, string, string, time.Time) error {
	return nil
}
func (*executionRepositoryFake) UpdateNodeConvergence(context.Context, string, string, time.Time) error {
	return nil
}
func (f *executionRepositoryFake) FinishDeployment(_ context.Context, deployment Deployment, activate bool, _ domain.AuditEvent) error {
	f.deployment.Status, f.deployment.ErrorCode, f.deployment.CompletedAt = deployment.Status, deployment.ErrorCode, deployment.CompletedAt
	f.activated = activate
	return nil
}
func (*executionRepositoryFake) InterruptDeployments(context.Context, time.Time) error { return nil }

type decrypterFake struct{}

func (decrypterFake) Decrypt(string, domain.EncryptedCredentials) (domain.NodeCredentials, error) {
	return domain.NodeCredentials{Username: "admin", Password: "secret"}, nil
}

type writerFake struct{ nodes []string }

func (f *writerFake) ApplyConfiguration(_ context.Context, request domain.NodeProbeRequest, _ configuration.Document) error {
	f.nodes = append(f.nodes, request.BaseURL)
	return nil
}

type observerFake struct {
	document configuration.Document
	failNode string
	calls    []string
}

func (f *observerFake) Observe(_ context.Context, nodeID string) (inventory.Snapshot, error) {
	f.calls = append(f.calls, nodeID)
	if nodeID == f.failNode {
		return inventory.Snapshot{}, errors.New("unavailable")
	}
	document := f.document
	return inventory.Snapshot{ID: "snapshot-" + nodeID, NodeID: nodeID, Document: &document, CollectionStatus: "succeeded"}, nil
}

func TestExecutorValidatesEveryTargetBeforeMutation(t *testing.T) {
	repository, document := executionFixture()
	writer := &writerFake{}
	observer := &observerFake{document: document, failNode: "node-b"}
	executor := NewExecutor(repository, decrypterFake{}, writer, observer)
	worked, err := executor.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !worked || len(writer.nodes) != 0 {
		t.Fatalf("worked=%v writes=%v, want no mutation", worked, writer.nodes)
	}
	if repository.deployment.Status != "failed" || repository.activated {
		t.Fatalf("deployment status=%s activated=%v", repository.deployment.Status, repository.activated)
	}
}

func TestExecutorDeploysSequentiallyAndActivatesAfterReadBack(t *testing.T) {
	repository, document := executionFixture()
	writer := &writerFake{}
	observer := &observerFake{document: document}
	executor := NewExecutor(repository, decrypterFake{}, writer, observer)
	if _, err := executor.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(writer.nodes) != 2 || writer.nodes[0] != "node-a" || writer.nodes[1] != "node-b" {
		t.Fatalf("writes = %v, want node-a then node-b", writer.nodes)
	}
	if repository.deployment.Status != "succeeded" || !repository.activated {
		t.Fatalf("deployment status=%s activated=%v", repository.deployment.Status, repository.activated)
	}
	for _, node := range repository.deployment.Nodes {
		if node.Status != "succeeded" || node.VerificationSnapshotID == nil {
			t.Fatalf("node task = %#v", node)
		}
	}
}

func executionFixture() (*executionRepositoryFake, configuration.Document) {
	document := configuration.Document{SchemaVersion: configuration.SchemaVersion, Shared: configuration.Shared{DNS: configuration.DNS{UpstreamDNS: []string{"1.1.1.1"}}, Filtering: configuration.Filtering{UpdateInterval: 24}}, NodeSpecific: configuration.NodeSpecific{BindHosts: []string{"0.0.0.0"}, DNSPort: 53}}
	desired := configuration.DesiredDocument{SchemaVersion: configuration.SchemaVersion, Shared: document.Shared, NodeOverrides: map[string]configuration.NodeSpecific{"node-a": document.NodeSpecific, "node-b": document.NodeSpecific}}
	deployment := Deployment{ID: "deployment", ClusterID: "cluster", RevisionID: "revision", Status: "queued", Origin: "manual", RequestID: "00000000-0000-4000-8000-000000000001", Nodes: []DeploymentNode{{ID: "task-a", NodeID: "node-a", Position: 1, Status: "pending"}, {ID: "task-b", NodeID: "node-b", Position: 2, Status: "pending"}}}
	records := map[string]domain.NodeRecord{
		"node-a": {Node: domain.Node{ID: "node-a", Enabled: true, BaseURL: "node-a", CompatibilityStatus: domain.CompatibilitySupported}},
		"node-b": {Node: domain.Node{ID: "node-b", Enabled: true, BaseURL: "node-b", CompatibilityStatus: domain.CompatibilitySupported}},
	}
	return &executionRepositoryFake{deployment: deployment, revision: Revision{ID: "revision", Document: desired}, records: records}, document
}

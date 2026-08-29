package onboarding

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/controlplane"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
	"github.com/benchristian88/atlas-dns/internal/inventory"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

const (
	testClusterID = "11111111-1111-4111-8111-111111111111"
	testNodeID    = "22222222-2222-4222-8222-222222222222"
	testUserID    = "33333333-3333-4333-8333-333333333333"
)

type onboardingRepositoryFake struct {
	clusters  []domain.Cluster
	nodes     []domain.Node
	snapshots []inventory.Snapshot
	profiles  []inventory.CapabilityProfile
	revisions []controlplane.Revision
	channels  []haoperations.NotificationChannel
	state     *State
	lastAudit domain.AuditEvent
}

func (r *onboardingRepositoryFake) ListClusters(context.Context) ([]domain.Cluster, error) {
	return r.clusters, nil
}
func (r *onboardingRepositoryFake) ClusterByID(context.Context, string) (domain.Cluster, error) {
	if len(r.clusters) == 0 {
		return domain.Cluster{}, domain.NewError(domain.ErrorNotFound, "cluster not found")
	}
	return r.clusters[0], nil
}
func (r *onboardingRepositoryFake) ListNodes(context.Context, string) ([]domain.Node, error) {
	return r.nodes, nil
}
func (r *onboardingRepositoryFake) LatestSnapshots(context.Context, string) ([]inventory.Snapshot, error) {
	return r.snapshots, nil
}
func (r *onboardingRepositoryFake) CapabilityProfiles(context.Context, string) ([]inventory.CapabilityProfile, error) {
	return r.profiles, nil
}
func (r *onboardingRepositoryFake) ListRevisions(context.Context, string, bool) ([]controlplane.Revision, error) {
	return r.revisions, nil
}
func (r *onboardingRepositoryFake) ListNotificationChannels(context.Context, string) ([]haoperations.NotificationChannel, error) {
	return r.channels, nil
}
func (r *onboardingRepositoryFake) OnboardingState(context.Context, string) (State, error) {
	if r.state == nil {
		return State{}, domain.NewError(domain.ErrorNotFound, "onboarding state not found")
	}
	return *r.state, nil
}
func (r *onboardingRepositoryFake) SaveOnboardingState(_ context.Context, value State, _ int, event domain.AuditEvent) error {
	r.state = &value
	r.lastAudit = event
	return nil
}

type settingsReaderFake struct{}

func (settingsReaderFake) Get(context.Context) (systemsettings.Settings, error) {
	return systemsettings.Settings{
		UpdateChecksEnabled: true, RecordVersion: 1,
		NodeHealthIntervalSeconds: 30, StatisticsPollIntervalSeconds: 3600,
		QueryLogCollectionEnabled: true, QueryLogPollIntervalSeconds: 30, QueryLogRetentionSeconds: 604800,
	}, nil
}

type initialCollectorFake struct {
	clusterID string
	err       error
}

func (f *initialCollectorFake) CollectInitial(_ context.Context, clusterID string) error {
	f.clusterID = clusterID
	return f.err
}

func newOnboardingService(repository *onboardingRepositoryFake) *Service {
	service := NewService(repository, settingsReaderFake{}, "https://atlas.example.test")
	service.now = func() time.Time { return time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC) }
	return service
}

func readyNode(version string) (domain.Node, inventory.Snapshot, inventory.CapabilityProfile) {
	node := domain.Node{ID: testNodeID, ClusterID: testClusterID, Name: "Primary", Enabled: true, HealthStatus: domain.NodeHealthy, CompatibilityStatus: domain.CompatibilitySupported, Version: version}
	document := &configuration.Document{}
	snapshot := inventory.Snapshot{ID: "44444444-4444-4444-8444-444444444444", NodeID: node.ID, SchemaVersion: configuration.SchemaVersion, Document: document, CollectionStatus: "succeeded"}
	profile := inventory.CapabilityProfile{NodeID: node.ID, Compatibility: string(domain.CompatibilitySupported), SchemaVersion: configuration.SchemaVersion}
	return node, snapshot, profile
}

func TestFreshInstallAndNoNodeResumeFromCanonicalFacts(t *testing.T) {
	repository := &onboardingRepositoryFake{}
	status, err := newOnboardingService(repository).Status(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !status.SetupRequired || status.ResumeStep != StepIdentity || status.Cluster != nil {
		t.Fatalf("fresh status = %#v", status)
	}

	repository.clusters = []domain.Cluster{{ID: testClusterID, Name: "Home"}}
	status, err = newOnboardingService(repository).Status(context.Background(), testClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if status.ResumeStep != StepPrimaryNode || status.EligibleNodeCount != 0 {
		t.Fatalf("no-node status = %#v", status)
	}
}

func TestUnsupportedOldNodeIsBlockedAndSupportedBaselineResumes(t *testing.T) {
	for _, version := range []string{"v0.107.78", "v0.107.79", "v0.107.80"} {
		t.Run(version, func(t *testing.T) {
			repository := &onboardingRepositoryFake{clusters: []domain.Cluster{{ID: testClusterID}}}
			node, snapshot, profile := readyNode(version)
			repository.nodes, repository.snapshots, repository.profiles = []domain.Node{node}, []inventory.Snapshot{snapshot}, []inventory.CapabilityProfile{profile}
			status, err := newOnboardingService(repository).Status(context.Background(), testClusterID)
			if err != nil {
				t.Fatal(err)
			}
			if status.EligibleNodeCount != 1 || status.ResumeStep != StepSecondaryNode {
				t.Fatalf("supported status = %#v", status)
			}
		})
	}

	repository := &onboardingRepositoryFake{clusters: []domain.Cluster{{ID: testClusterID}}}
	node, snapshot, profile := readyNode("v0.107.77")
	repository.nodes, repository.snapshots, repository.profiles = []domain.Node{node}, []inventory.Snapshot{snapshot}, []inventory.CapabilityProfile{profile}
	status, err := newOnboardingService(repository).Status(context.Background(), testClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if status.EligibleNodeCount != 0 || status.ResumeStep != StepPrimaryNode {
		t.Fatalf("old node was not blocked: %#v", status)
	}
}

func TestDeliberateSingleNodeSkipResumesAtBaseline(t *testing.T) {
	repository := &onboardingRepositoryFake{clusters: []domain.Cluster{{ID: testClusterID}}}
	node, snapshot, profile := readyNode("v0.107.79")
	repository.nodes, repository.snapshots, repository.profiles = []domain.Node{node}, []inventory.Snapshot{snapshot}, []inventory.CapabilityProfile{profile}
	service := newOnboardingService(repository)
	status, err := service.Progress(context.Background(), domain.Actor{UserID: testUserID, RequestID: "request"}, testClusterID, 0, ProgressInput{RedundancySkipped: boolPointer(true)})
	if err != nil {
		t.Fatal(err)
	}
	if status.ResumeStep != StepBaseline || repository.state == nil || repository.state.RedundancySkippedAt == nil || repository.lastAudit.Action != "onboarding.progress_updated" {
		t.Fatalf("resume/audit status=%#v state=%#v audit=%#v", status, repository.state, repository.lastAudit)
	}
}

func TestFinishIsAuditedAndNotRevokedByTransientHealth(t *testing.T) {
	now := time.Date(2026, 8, 24, 7, 0, 0, 0, time.UTC)
	repository := &onboardingRepositoryFake{
		clusters:  []domain.Cluster{{ID: testClusterID}},
		state:     &State{ClusterID: testClusterID, RedundancySkippedAt: &now, MonitoringReviewedAt: &now, NotificationsSkippedAt: &now, RecordVersion: 3},
		revisions: []controlplane.Revision{{ID: "55555555-5555-4555-8555-555555555555", ClusterID: testClusterID, RevisionNumber: 1, SchemaVersion: configuration.SchemaVersion}},
	}
	node, snapshot, profile := readyNode("v0.107.79")
	repository.nodes, repository.snapshots, repository.profiles = []domain.Node{node}, []inventory.Snapshot{snapshot}, []inventory.CapabilityProfile{profile}
	service := newOnboardingService(repository)
	collector := &initialCollectorFake{err: errors.New("one collector unavailable")}
	service.SetInitialCollector(collector, slog.New(slog.NewTextHandler(io.Discard, nil)))
	status, err := service.Finish(context.Background(), domain.Actor{UserID: testUserID, RequestID: "request"}, testClusterID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Completed || status.SetupRequired || status.ResumeStep != StepComplete || repository.lastAudit.Action != "onboarding.completed" {
		t.Fatalf("completed status=%#v audit=%#v", status, repository.lastAudit)
	}
	if collector.clusterID != testClusterID {
		t.Fatalf("initial collection cluster = %q", collector.clusterID)
	}
	repository.nodes[0].HealthStatus = domain.NodeUnreachable
	status, err = service.Status(context.Background(), testClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Completed || status.SetupRequired || status.ResumeStep != StepComplete || status.TopologyReady {
		t.Fatalf("transient health revoked completion: %#v", status)
	}
}

func boolPointer(value bool) *bool { return &value }

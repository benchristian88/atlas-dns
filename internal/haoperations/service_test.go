package haoperations

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

const (
	serviceClusterID = "11111111-1111-4111-8111-111111111111"
	serviceNodeA     = "22222222-2222-4222-8222-222222222222"
	serviceNodeB     = "33333333-3333-4333-8333-333333333333"
	serviceNodeC     = "44444444-4444-4444-8444-444444444444"
)

type serviceRepositoryFake struct {
	Repository
	nodes            []domain.Node
	probes           []DNSProbeResult
	snapshots        []inventory.Snapshot
	activeDeployment bool
	openDrift        bool
	settings         map[string]NodeSettings
	release          ReleaseCache
	cluster          domain.Cluster
	events           []Event
	history          []HistoryItem
	lastHistoryQuery HistoryQuery
	audits           []domain.AuditEvent
	collectorChecks  []Check
}

func (r *serviceRepositoryFake) ListNodes(context.Context, string) ([]domain.Node, error) {
	return r.nodes, nil
}
func (r *serviceRepositoryFake) NodeByID(_ context.Context, id string) (domain.Node, error) {
	for _, node := range r.nodes {
		if node.ID == id {
			return node, nil
		}
	}
	return domain.Node{}, domain.NewError(domain.ErrorNotFound, "node was not found")
}
func (r *serviceRepositoryFake) NodeRecordByID(ctx context.Context, id string) (domain.NodeRecord, error) {
	node, err := r.NodeByID(ctx, id)
	return domain.NodeRecord{Node: node}, err
}
func (r *serviceRepositoryFake) ClusterByID(context.Context, string) (domain.Cluster, error) {
	if r.cluster.ID == "" {
		return domain.Cluster{ID: serviceClusterID}, nil
	}
	return r.cluster, nil
}
func (r *serviceRepositoryFake) LatestDNSProbes(context.Context, string) ([]DNSProbeResult, error) {
	return r.probes, nil
}
func (r *serviceRepositoryFake) LatestDNSProbe(_ context.Context, nodeID string) (DNSProbeResult, error) {
	for _, probe := range r.probes {
		if probe.NodeID == nodeID {
			return probe, nil
		}
	}
	return DNSProbeResult{}, domain.NewError(domain.ErrorNotFound, "probe was not found")
}
func (r *serviceRepositoryFake) LatestSuccessfulSnapshots(context.Context, string) ([]inventory.Snapshot, error) {
	return r.snapshots, nil
}
func (r *serviceRepositoryFake) ActiveDeploymentExists(context.Context, string) (bool, error) {
	return r.activeDeployment, nil
}
func (r *serviceRepositoryFake) OpenDriftExists(context.Context, string) (bool, error) {
	return r.openDrift, nil
}
func (r *serviceRepositoryFake) NodeLifecycleSettings(_ context.Context, nodeID string) (NodeSettings, error) {
	if value, ok := r.settings[nodeID]; ok {
		return value, nil
	}
	return NodeSettings{}, domain.NewError(domain.ErrorNotFound, "settings were not found")
}
func (r *serviceRepositoryFake) ReleaseCache(context.Context) (ReleaseCache, error) {
	if r.release.Version == "" {
		return ReleaseCache{}, errors.New("release cache unavailable")
	}
	return r.release, nil
}
func (r *serviceRepositoryFake) SaveDNSProbe(_ context.Context, value DNSProbeResult, event *Event) error {
	r.probes = append(r.probes, value)
	if event != nil {
		r.events = append(r.events, *event)
	}
	return nil
}
func (r *serviceRepositoryFake) RecordHAEvent(_ context.Context, event Event) error {
	r.events = append(r.events, event)
	return nil
}
func (r *serviceRepositoryFake) RecordHAEventAndAudit(_ context.Context, event Event, audit domain.AuditEvent) error {
	r.events = append(r.events, event)
	r.audits = append(r.audits, audit)
	return nil
}
func (r *serviceRepositoryFake) ListHAHistory(_ context.Context, query HistoryQuery) ([]HistoryItem, error) {
	r.lastHistoryQuery = query
	result := make([]HistoryItem, 0, len(r.history))
	for _, item := range r.history {
		if query.BeforeAt != nil && (item.OccurredAt.After(*query.BeforeAt) || item.OccurredAt.Equal(*query.BeforeAt) && item.ID >= query.BeforeID) {
			continue
		}
		result = append(result, item)
		if len(result) == query.Limit {
			break
		}
	}
	return result, nil
}
func (r *serviceRepositoryFake) CollectorChecks(context.Context, string) ([]Check, error) {
	return r.collectorChecks, nil
}

type serviceMaintenanceFake struct {
	repository *serviceRepositoryFake
	calls      []bool
}

func (m *serviceMaintenanceFake) SetNodeMaintenance(_ context.Context, _ domain.Actor, nodeID string, enabled bool, expectedVersion int) (domain.Node, error) {
	for index := range m.repository.nodes {
		node := &m.repository.nodes[index]
		if node.ID != nodeID {
			continue
		}
		if node.RecordVersion != expectedVersion {
			return domain.Node{}, domain.NewError(domain.ErrorConflict, "node was changed by another request")
		}
		m.calls = append(m.calls, enabled)
		node.MaintenanceMode = enabled
		node.RecordVersion++
		if enabled {
			node.ConvergenceStatus = "maintenance"
		} else {
			node.ConvergenceStatus = "pending"
		}
		return *node, nil
	}
	return domain.Node{}, domain.NewError(domain.ErrorNotFound, "node was not found")
}

type serviceObserverFake struct {
	snapshot inventory.Snapshot
	err      error
}

func (f serviceObserverFake) Observe(context.Context, string) (inventory.Snapshot, error) {
	return f.snapshot, f.err
}

type serviceStatusProbeFake struct {
	err     error
	request *domain.NodeProbeRequest
}

func (f serviceStatusProbeFake) Status(_ context.Context, request domain.NodeProbeRequest) (domain.NodeProbeResult, error) {
	if f.request != nil {
		*f.request = request
	}
	return domain.NodeProbeResult{}, f.err
}

type serviceCredentialFake struct{ err error }

func (f serviceCredentialFake) Decrypt(string, domain.EncryptedCredentials) (domain.NodeCredentials, error) {
	return domain.NodeCredentials{Username: "admin", Password: "secret"}, f.err
}

type serviceDNSProbeFake struct{ err error }

func (f serviceDNSProbeFake) Probe(context.Context, DNSProbeRequest) (DNSProbeResult, error) {
	return DNSProbeResult{Status: "healthy", UDPStatus: "healthy", TCPStatus: "healthy", ProbedAt: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)}, f.err
}

func healthyNode(id string) domain.Node {
	return domain.Node{ID: id, ClusterID: serviceClusterID, Name: id, Enabled: true, HealthStatus: domain.NodeHealthy, CompatibilityStatus: domain.CompatibilitySupported, ConvergenceStatus: "converged", Version: "v0.107.78"}
}

func TestSummarySeparatesNNodeDNSAPIAndMaintenance(t *testing.T) {
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	nodeA, nodeB, nodeC := healthyNode(serviceNodeA), healthyNode(serviceNodeB), healthyNode(serviceNodeC)
	nodeB.HealthStatus = domain.NodeUnreachable
	nodeC.MaintenanceMode = true
	repository := &serviceRepositoryFake{
		nodes: []domain.Node{nodeA, nodeB, nodeC},
		probes: []DNSProbeResult{
			{NodeID: serviceNodeA, Status: "healthy", UDPStatus: "healthy", TCPStatus: "healthy", ProbedAt: now},
			{NodeID: serviceNodeB, Status: "healthy", UDPStatus: "healthy", TCPStatus: "healthy", ProbedAt: now},
			{NodeID: serviceNodeC, Status: "failed", UDPStatus: "failed", TCPStatus: "failed", ProbedAt: now},
		},
		settings: map[string]NodeSettings{
			serviceNodeA: {NodeID: serviceNodeA, InstallationType: InstallationDocker},
			serviceNodeB: {NodeID: serviceNodeB, InstallationType: InstallationDocker},
			serviceNodeC: {NodeID: serviceNodeC, InstallationType: InstallationDocker},
		},
	}
	service := NewService(repository, nil, nil, nil, nil, nil)
	service.now = func() time.Time { return now }
	summary, err := service.Summary(context.Background(), serviceClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.State != "degraded" || summary.ServingDNSNodes != 2 || summary.APIReachableNodes != 2 || summary.MaintenanceNodes != 1 || len(summary.Nodes) != 3 {
		t.Fatalf("summary=%#v", summary)
	}
	if summary.Nodes[2].DNSStatus != "maintenance" {
		t.Fatalf("maintenance DNS dimension=%#v", summary.Nodes[2])
	}
}

func TestOperationalHistoryUsesStableBoundedCursorPagination(t *testing.T) {
	now := time.Date(2026, 8, 20, 2, 0, 0, 0, time.UTC)
	repository := &serviceRepositoryFake{
		nodes: []domain.Node{healthyNode(serviceNodeA)},
		history: []HistoryItem{
			{ID: "90000000-0000-4000-8000-000000000003", Kind: "event", ClusterID: serviceClusterID, NodeID: stringPointer(serviceNodeA), OccurredAt: now},
			{ID: "90000000-0000-4000-8000-000000000002", Kind: "notification", ClusterID: serviceClusterID, NodeID: stringPointer(serviceNodeA), OccurredAt: now},
			{ID: "90000000-0000-4000-8000-000000000001", Kind: "event", ClusterID: serviceClusterID, NodeID: stringPointer(serviceNodeA), OccurredAt: now.Add(-time.Minute)},
		},
	}
	service := NewService(repository, nil, nil, nil, nil, nil)
	first, err := service.History(context.Background(), HistoryRequest{ClusterID: serviceClusterID, NodeID: serviceNodeA, Limit: 2})
	if err != nil || len(first.Items) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	if repository.lastHistoryQuery.Limit != 3 || repository.lastHistoryQuery.BeforeAt != nil {
		t.Fatalf("first query=%#v", repository.lastHistoryQuery)
	}
	second, err := service.History(context.Background(), HistoryRequest{ClusterID: serviceClusterID, NodeID: serviceNodeA, Limit: 2, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.HasMore || second.NextCursor != "" || second.Items[0].ID != repository.history[2].ID {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	if repository.lastHistoryQuery.BeforeAt == nil || repository.lastHistoryQuery.BeforeID != repository.history[1].ID {
		t.Fatalf("second query=%#v", repository.lastHistoryQuery)
	}
}

func TestOperationalHistoryRejectsInvalidCursorLimitAndCrossClusterNode(t *testing.T) {
	repository := &serviceRepositoryFake{nodes: []domain.Node{healthyNode(serviceNodeA)}}
	service := NewService(repository, nil, nil, nil, nil, nil)
	for _, request := range []HistoryRequest{
		{ClusterID: serviceClusterID, Limit: 101},
		{ClusterID: serviceClusterID, Limit: 50, Cursor: "not-a-cursor"},
	} {
		if _, err := service.History(context.Background(), request); err == nil {
			t.Fatalf("invalid request accepted: %#v", request)
		}
	}
	repository.nodes[0].ClusterID = serviceNodeB
	if _, err := service.History(context.Background(), HistoryRequest{ClusterID: serviceClusterID, NodeID: serviceNodeA, Limit: 50}); err == nil {
		t.Fatal("cross-cluster node filter accepted")
	}
}

func stringPointer(value string) *string { return &value }

func TestSummaryReportsAllDNSFailedAtRisk(t *testing.T) {
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	repository := &serviceRepositoryFake{nodes: []domain.Node{healthyNode(serviceNodeA), healthyNode(serviceNodeB)}, probes: []DNSProbeResult{{NodeID: serviceNodeA, Status: "failed", ProbedAt: now}, {NodeID: serviceNodeB, Status: "failed", ProbedAt: now}}, settings: map[string]NodeSettings{}}
	service := NewService(repository, nil, nil, nil, nil, nil)
	service.now = func() time.Time { return now }
	summary, err := service.Summary(context.Background(), serviceClusterID)
	if err != nil || summary.State != "at_risk" || summary.ServingDNSNodes != 0 {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}
}

func TestSummaryReturnsEmptyNodeCollectionForNewCluster(t *testing.T) {
	repository := &serviceRepositoryFake{settings: map[string]NodeSettings{}}
	service := NewService(repository, nil, nil, nil, nil, nil)
	summary, err := service.Summary(context.Background(), serviceClusterID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Nodes == nil || len(summary.Nodes) != 0 {
		t.Fatalf("nodes=%#v, want a non-nil empty collection", summary.Nodes)
	}
	if summary.TotalNodes != 0 || summary.State != "at_risk" {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestVersionsSupportsRollingV010778ToV010779(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	nodeA, nodeB := healthyNode(serviceNodeA), healthyNode(serviceNodeB)
	nodeB.Version = "v0.107.79"
	repository := &serviceRepositoryFake{
		nodes:   []domain.Node{nodeA, nodeB},
		release: ReleaseCache{Version: "v0.107.79", CheckedAt: now, ExpiresAt: now.Add(time.Hour)},
		settings: map[string]NodeSettings{
			serviceNodeA: {NodeID: serviceNodeA, InstallationType: InstallationDocker},
			serviceNodeB: {NodeID: serviceNodeB, InstallationType: InstallationDocker},
		},
	}
	service := NewService(repository, nil, nil, nil, nil, nil)
	service.now = func() time.Time { return now }
	versions, err := service.Versions(context.Background(), serviceClusterID)
	if err != nil || !versions[0].UpdateAvailable || versions[1].UpdateAvailable {
		t.Fatalf("mixed rolling versions=%+v err=%v", versions, err)
	}
	repository.nodes[0].Version = "v0.107.79"
	versions, err = service.Versions(context.Background(), serviceClusterID)
	if err != nil || versions[0].UpdateAvailable || versions[1].UpdateAvailable {
		t.Fatalf("completed rolling versions=%+v err=%v", versions, err)
	}
}

func TestMaintenancePreflightBlocksDeploymentAndActiveDHCP(t *testing.T) {
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	document := configuration.Document{NodeSpecific: configuration.NodeSpecific{DHCP: &configuration.DHCPConfig{Enabled: true}}}
	repository := &serviceRepositoryFake{
		nodes:            []domain.Node{healthyNode(serviceNodeA), healthyNode(serviceNodeB)},
		probes:           []DNSProbeResult{{NodeID: serviceNodeA, Status: "healthy", ProbedAt: now}, {NodeID: serviceNodeB, Status: "healthy", ProbedAt: now}},
		snapshots:        []inventory.Snapshot{{NodeID: serviceNodeA, Document: &document}},
		activeDeployment: true,
		openDrift:        true,
		settings:         map[string]NodeSettings{},
	}
	service := NewService(repository, nil, nil, nil, nil, nil)
	service.now = func() time.Time { return now }
	preflight, err := service.MaintenancePreflight(context.Background(), serviceNodeA)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.Allowed || !preflight.ActiveDHCP || !preflight.ActiveDeployment || !preflight.OpenDrift || preflight.HealthyDNSNodesRemaining != 1 {
		t.Fatalf("preflight=%#v", preflight)
	}
}

func TestMaintenanceLifecycleEntersReturnsAndHandlesDuplicates(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	node := healthyNode(serviceNodeA)
	node.RecordVersion, node.BaseURL = 4, "http://192.0.2.10:3000"
	document := configuration.Document{ObservedOnly: configuration.ObservedOnly{TLS: configuration.TLSStatus{Enabled: false, NotAfter: "2026-08-10T00:00:00Z"}}}
	repository := &serviceRepositoryFake{
		nodes:     []domain.Node{node, healthyNode(serviceNodeB)},
		probes:    []DNSProbeResult{{NodeID: serviceNodeA, Status: "healthy", ProbedAt: now}, {NodeID: serviceNodeB, Status: "healthy", ProbedAt: now}},
		snapshots: []inventory.Snapshot{{NodeID: serviceNodeA, ObservedAt: now, CollectionStatus: "succeeded", Document: &document}},
		openDrift: true,
		settings: map[string]NodeSettings{
			serviceNodeA: {NodeID: serviceNodeA, DNSProbeHost: "192.0.2.10", DNSProbePort: 53, DNSProbeName: ".", DNSProbeType: "NS", ProbeUDP: true, ProbeTCP: true},
		},
	}
	maintenance := &serviceMaintenanceFake{repository: repository}
	var apiRequest domain.NodeProbeRequest
	service := NewService(repository, maintenance, serviceObserverFake{snapshot: inventory.Snapshot{NodeID: serviceNodeA, CollectionStatus: "succeeded", Document: &document}}, serviceStatusProbeFake{request: &apiRequest}, serviceCredentialFake{}, serviceDNSProbeFake{})
	service.now = func() time.Time { return now }

	entered, err := service.EnterMaintenance(context.Background(), domain.Actor{UserID: serviceNodeB, RequestID: "request-enter"}, serviceNodeA, 4, false, "")
	if err != nil || !entered.MaintenanceMode || len(maintenance.calls) != 1 || !maintenance.calls[0] {
		t.Fatalf("enter node=%#v calls=%v err=%v", entered, maintenance.calls, err)
	}
	if len(repository.events) != 1 || repository.events[0].EventType != "maintenance.started" {
		t.Fatalf("enter events=%#v", repository.events)
	}
	if _, err := service.EnterMaintenance(context.Background(), domain.Actor{}, serviceNodeA, 4, false, ""); err != nil || len(maintenance.calls) != 1 || len(repository.events) != 1 {
		t.Fatalf("duplicate enter calls=%v events=%#v err=%v", maintenance.calls, repository.events, err)
	}

	validation, err := service.ReturnToService(context.Background(), domain.Actor{UserID: serviceNodeB, RequestID: "request-return"}, serviceNodeA, 5)
	if err != nil || !validation.Succeeded || len(maintenance.calls) != 2 || maintenance.calls[1] {
		t.Fatalf("return validation=%#v calls=%v err=%v", validation, maintenance.calls, err)
	}
	foundPendingReconciliation, foundTLSNotApplicable := false, false
	for _, check := range validation.Checks {
		if check.Name == "convergence_drift" && check.Status == "warning" && !check.Required && check.ErrorCode == "CONFIGURATION_RECONCILIATION_PENDING" {
			foundPendingReconciliation = true
		}
		if check.Name == "tls" && check.Status == "not_applicable" && !check.Required {
			foundTLSNotApplicable = true
		}
	}
	if !foundPendingReconciliation || !foundTLSNotApplicable {
		t.Fatalf("return checks did not preserve drift warning and TLS applicability: %#v", validation.Checks)
	}
	if apiRequest.BaseURL != node.BaseURL || apiRequest.CertificatePolicy != node.CertificatePolicy {
		t.Fatalf("HTTP-only administration probe request=%#v", apiRequest)
	}
	if repository.nodes[0].MaintenanceMode || repository.nodes[0].RecordVersion != 6 {
		t.Fatalf("persisted node=%#v", repository.nodes[0])
	}
	if repository.events[len(repository.events)-1].EventType != "maintenance.ended" {
		t.Fatalf("return events=%#v", repository.events)
	}
	summary, summaryErr := service.Summary(context.Background(), serviceClusterID)
	if summaryErr != nil || summary.MaintenanceNodes != 0 || summary.Nodes[0].DNSStatus != "healthy" {
		t.Fatalf("post-return summary=%#v err=%v", summary, summaryErr)
	}
	if duplicate, duplicateErr := service.ReturnToService(context.Background(), domain.Actor{}, serviceNodeA, 5); duplicateErr != nil || !duplicate.Succeeded || len(maintenance.calls) != 2 {
		t.Fatalf("duplicate return=%#v calls=%v err=%v", duplicate, maintenance.calls, duplicateErr)
	}
}

func TestReturnToServiceFailureLeavesMaintenanceAndAuditsFailure(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	node := healthyNode(serviceNodeA)
	node.MaintenanceMode, node.RecordVersion, node.ConvergenceStatus = true, 2, "maintenance"
	repository := &serviceRepositoryFake{
		nodes:     []domain.Node{node},
		snapshots: []inventory.Snapshot{{NodeID: serviceNodeA, ObservedAt: now, Document: &configuration.Document{}}},
		settings: map[string]NodeSettings{
			serviceNodeA: {NodeID: serviceNodeA, DNSProbeHost: "192.0.2.10", DNSProbePort: 53, DNSProbeName: ".", DNSProbeType: "NS", ProbeUDP: true, ProbeTCP: true},
		},
	}
	maintenance := &serviceMaintenanceFake{repository: repository}
	service := NewService(repository, maintenance, serviceObserverFake{snapshot: inventory.Snapshot{NodeID: serviceNodeA, CollectionStatus: "succeeded", Document: &configuration.Document{}}}, serviceStatusProbeFake{err: errors.New("API unavailable")}, serviceCredentialFake{}, serviceDNSProbeFake{})
	service.now = func() time.Time { return now }

	validation, err := service.ReturnToService(context.Background(), domain.Actor{UserID: serviceNodeB, RequestID: "request-failed"}, serviceNodeA, 2)
	if err == nil || validation.Succeeded || !repository.nodes[0].MaintenanceMode || len(maintenance.calls) != 0 {
		t.Fatalf("validation=%#v node=%#v calls=%v err=%v", validation, repository.nodes[0], maintenance.calls, err)
	}
	if len(repository.events) != 2 || repository.events[1].EventType != "maintenance.return_validation_failed" {
		t.Fatalf("events=%#v", repository.events)
	}
	if len(repository.audits) != 1 || repository.audits[0].Action != "node.maintenance_return_failed" {
		t.Fatalf("audits=%#v", repository.audits)
	}
	if !strings.Contains(err.Error(), "api") {
		t.Fatalf("failure omitted safe failed-check diagnostics: %v", err)
	}
}

func TestReturnToServiceTLSApplicabilityMatrix(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name               string
		tls                configuration.TLSStatus
		observationErr     error
		wantStatus         string
		wantCode           string
		wantSuccess        bool
		wantMessage        string
		httpAdministration bool
	}{
		{
			name:       "not configured ignores retained expired metadata",
			tls:        configuration.TLSStatus{Enabled: false, NotAfter: "2026-08-10T00:00:00Z"},
			wantStatus: "not_applicable", wantSuccess: true, httpAdministration: true,
		},
		{
			name:       "configured and healthy",
			tls:        configuration.TLSStatus{Enabled: true, ValidCertificate: true, ValidChain: true, ValidKey: true, ValidPair: true, NotBefore: "2026-08-01T00:00:00Z", NotAfter: "2027-08-01T00:00:00Z"},
			wantStatus: "pass", wantSuccess: true,
		},
		{
			name:       "configured and expired",
			tls:        configuration.TLSStatus{Enabled: true, ValidCertificate: true, ValidChain: true, ValidKey: true, ValidPair: true, NotAfter: "2026-08-10T00:00:00Z"},
			wantStatus: "fail", wantCode: "TLS_CERTIFICATE_EXPIRED", wantMessage: "TLS certificate expired on 2026-08-10",
		},
		{
			name:           "state unavailable",
			observationErr: errors.New("observation unavailable"),
			wantStatus:     "unknown", wantCode: "TLS_STATE_UNAVAILABLE",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := healthyNode(serviceNodeA)
			node.MaintenanceMode, node.RecordVersion, node.ConvergenceStatus = true, 7, "maintenance"
			if test.httpAdministration {
				node.BaseURL = "http://192.0.2.10:3000"
			}
			document := configuration.Document{ObservedOnly: configuration.ObservedOnly{TLS: test.tls}}
			snapshot := inventory.Snapshot{NodeID: serviceNodeA, ObservedAt: now, CollectionStatus: "succeeded", Document: &document}
			repository := &serviceRepositoryFake{
				nodes: []domain.Node{node}, snapshots: []inventory.Snapshot{snapshot},
				settings: map[string]NodeSettings{serviceNodeA: {NodeID: serviceNodeA, DNSProbeHost: "192.0.2.10", DNSProbePort: 53, DNSProbeName: ".", DNSProbeType: "NS", ProbeUDP: true, ProbeTCP: true}},
			}
			maintenance := &serviceMaintenanceFake{repository: repository}
			var apiRequest domain.NodeProbeRequest
			service := NewService(repository, maintenance, serviceObserverFake{snapshot: snapshot, err: test.observationErr}, serviceStatusProbeFake{request: &apiRequest}, serviceCredentialFake{}, serviceDNSProbeFake{})
			service.now = func() time.Time { return now }

			validation, err := service.ReturnToService(context.Background(), domain.Actor{UserID: serviceNodeB, RequestID: "request-tls"}, serviceNodeA, 7)
			tlsCheck, found := checkByName(validation.Checks, "tls")
			if !found || tlsCheck.Status != test.wantStatus || tlsCheck.ErrorCode != test.wantCode || validation.Succeeded != test.wantSuccess {
				t.Fatalf("validation=%#v tls=%#v found=%t err=%v", validation, tlsCheck, found, err)
			}
			if test.wantSuccess {
				if err != nil || repository.nodes[0].MaintenanceMode || len(maintenance.calls) != 1 {
					t.Fatalf("successful return node=%#v calls=%v err=%v", repository.nodes[0], maintenance.calls, err)
				}
			} else if err == nil || !repository.nodes[0].MaintenanceMode || len(maintenance.calls) != 0 {
				t.Fatalf("failed return node=%#v calls=%v err=%v", repository.nodes[0], maintenance.calls, err)
			}
			if test.wantMessage != "" && (!strings.Contains(tlsCheck.Message, test.wantMessage) || !strings.Contains(err.Error(), test.wantMessage)) {
				t.Fatalf("TLS diagnostic check=%#v err=%v", tlsCheck, err)
			}
			if test.httpAdministration && apiRequest.BaseURL != node.BaseURL {
				t.Fatalf("HTTP administration request=%#v", apiRequest)
			}
		})
	}
}

func TestReturnToServiceDNSFailureRemainsBlocking(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	node := healthyNode(serviceNodeA)
	node.MaintenanceMode, node.RecordVersion, node.ConvergenceStatus = true, 3, "maintenance"
	document := configuration.Document{}
	repository := &serviceRepositoryFake{
		nodes: []domain.Node{node}, snapshots: []inventory.Snapshot{{NodeID: serviceNodeA, CollectionStatus: "succeeded", Document: &document}},
		settings: map[string]NodeSettings{serviceNodeA: {NodeID: serviceNodeA, DNSProbeHost: "192.0.2.10", DNSProbePort: 53, DNSProbeName: ".", DNSProbeType: "NS", ProbeUDP: true, ProbeTCP: true}},
	}
	maintenance := &serviceMaintenanceFake{repository: repository}
	service := NewService(repository, maintenance, serviceObserverFake{snapshot: inventory.Snapshot{NodeID: serviceNodeA, CollectionStatus: "succeeded", Document: &document}}, serviceStatusProbeFake{}, serviceCredentialFake{}, serviceDNSProbeFake{err: errors.New("DNS unavailable")})
	service.now = func() time.Time { return now }

	validation, err := service.ReturnToService(context.Background(), domain.Actor{UserID: serviceNodeB, RequestID: "request-dns"}, serviceNodeA, 3)
	if err == nil || validation.Succeeded || !repository.nodes[0].MaintenanceMode || len(maintenance.calls) != 0 || !strings.Contains(err.Error(), "dns") {
		t.Fatalf("validation=%#v node=%#v calls=%v err=%v", validation, repository.nodes[0], maintenance.calls, err)
	}
}

func checkByName(checks []Check, name string) (Check, bool) {
	for _, check := range checks {
		if check.Name == name {
			return check, true
		}
	}
	return Check{}, false
}

func TestCertificateThresholdsUseRedactedObservation(t *testing.T) {
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	document := configuration.Document{ObservedOnly: configuration.ObservedOnly{TLS: configuration.TLSStatus{Enabled: true, Subject: "DNS certificate", Issuer: "Homelab CA", NotAfter: now.Add(6 * 24 * time.Hour).Format(time.RFC3339)}}}
	repository := &serviceRepositoryFake{nodes: []domain.Node{healthyNode(serviceNodeA)}, snapshots: []inventory.Snapshot{{NodeID: serviceNodeA, ObservedAt: now, Document: &document}}}
	service := NewService(repository, nil, nil, nil, nil, nil)
	service.now = func() time.Time { return now }
	certificates, err := service.Certificates(context.Background(), serviceClusterID)
	if err != nil || len(certificates) != 1 || certificates[0].State != CertificateCritical || certificates[0].DaysRemaining == nil || *certificates[0].DaysRemaining != 6 {
		t.Fatalf("certificates=%#v err=%v", certificates, err)
	}
}

func TestUpgradeRejectsUnsupportedInstallation(t *testing.T) {
	repository := &serviceRepositoryFake{nodes: []domain.Node{healthyNode(serviceNodeA)}, settings: map[string]NodeSettings{serviceNodeA: {NodeID: serviceNodeA, InstallationType: InstallationHomeAssistant}}}
	service := NewService(repository, nil, nil, nil, nil, nil)
	_, err := service.StartUpgrade(context.Background(), domain.Actor{}, serviceNodeA, "v0.107.79")
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Kind != domain.ErrorCapability {
		t.Fatalf("err=%v", err)
	}
}

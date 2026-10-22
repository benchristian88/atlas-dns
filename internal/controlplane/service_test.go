package controlplane

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

type draftRepositoryFake struct {
	Repository
	draft           inventory.Draft
	nodes           []domain.Node
	snapshots       []inventory.Snapshot
	profiles        []inventory.CapabilityProfile
	saved           *inventory.Draft
	expectedVersion int
}

func (f *draftRepositoryFake) DraftByCluster(context.Context, string) (inventory.Draft, error) {
	return f.draft, nil
}

func (f *draftRepositoryFake) ListNodes(context.Context, string) ([]domain.Node, error) {
	return f.nodes, nil
}

func (f *draftRepositoryFake) LatestSnapshots(context.Context, string) ([]inventory.Snapshot, error) {
	return f.snapshots, nil
}

func (f *draftRepositoryFake) CapabilityProfiles(context.Context, string) ([]inventory.CapabilityProfile, error) {
	return f.profiles, nil
}

func (f *draftRepositoryFake) UpdateConfigurationDraft(_ context.Context, draft inventory.Draft, expectedVersion int, _ domain.AuditEvent) error {
	f.saved = &draft
	f.expectedVersion = expectedVersion
	return nil
}

func TestUpdateDraftAfterMultiNodeImport(t *testing.T) {
	const (
		clusterID = "11111111-1111-4111-8111-111111111111"
		nodeAID   = "22222222-2222-4222-8222-222222222222"
		nodeBID   = "33333333-3333-4333-8333-333333333333"
	)
	listener := configuration.NodeSpecific{BindHosts: []string{"0.0.0.0"}, DNSPort: 53}
	document := configuration.DesiredDocument{
		SchemaVersion: configuration.SchemaVersion,
		Shared: configuration.Shared{
			DNS:       configuration.DNS{UpstreamDNS: []string{"9.9.9.9"}},
			Filtering: configuration.Filtering{UpdateInterval: 24},
		},
		NodeOverrides: map[string]configuration.NodeSpecific{nodeAID: listener, nodeBID: listener},
	}
	repository := &draftRepositoryFake{
		draft: inventory.Draft{ID: "44444444-4444-4444-8444-444444444444", ClusterID: clusterID, Version: 2, Document: document},
		nodes: []domain.Node{{ID: nodeAID, Enabled: true}, {ID: nodeBID, Enabled: true}},
	}
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC) }
	document.Shared.DNS.UpstreamDNS = []string{"1.1.1.1"}
	document.Shared.Services.BlockedServiceIDs = []string{"legacy-service", "youtube"}

	updated, issues, err := service.UpdateDraft(context.Background(), domain.Actor{UserID: "55555555-5555-4555-8555-555555555555", RequestID: "66666666-6666-4666-8666-666666666666"}, clusterID, 2, document)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("validation issues = %#v", issues)
	}
	if repository.saved == nil || repository.expectedVersion != 2 || updated.Version != 3 {
		t.Fatalf("save state = %#v, expected version = %d, updated version = %d", repository.saved, repository.expectedVersion, updated.Version)
	}
	if got := updated.Document.Shared.DNS.UpstreamDNS; len(got) != 1 || got[0] != "1.1.1.1" {
		t.Fatalf("saved upstreams = %#v", got)
	}
	if got := updated.Document.Shared.Services.BlockedServiceIDs; len(got) != 2 || got[0] != "legacy-service" || got[1] != "youtube" {
		t.Fatalf("legacy blocked-service IDs were not preserved: %#v", got)
	}
}

func TestManagedDifferencesIgnoreObservedCompatibilityMetadata(t *testing.T) {
	desired := configuration.Document{SchemaVersion: configuration.SchemaVersion, Shared: configuration.Shared{DNS: configuration.DNS{UpstreamDNS: []string{"1.1.1.1"}}}, Unsupported: []configuration.Unsupported{{Section: "tls_mutation", Reason: "inventory only"}}}
	observed := desired
	observed.Unsupported = []configuration.Unsupported{{Section: "dhcp", Reason: "unavailable"}}
	observed.ObservedOnly.TLS = configuration.TLSStatus{Enabled: true, ServerName: "dns.example"}
	observed.ObservedOnly.DHCPLeases = []configuration.DHCPLease{{MAC: "00:11:22:33:44:55", IP: "192.0.2.20", Hostname: "laptop", ExpiresAt: "2026-08-02T12:00:00Z"}}
	if differences := managedDifferences(desired, observed); len(differences) != 0 {
		t.Fatalf("unmanaged metadata caused drift: %#v", differences)
	}
}

func TestManagedDifferencesDetectDHCPConfigurationResetButIgnoreLeaseReset(t *testing.T) {
	dhcp := &configuration.DHCPConfig{
		Enabled: true, InterfaceName: "eth0",
		IPv4: configuration.DHCPIPv4{Gateway: "192.0.2.1", SubnetMask: "255.255.255.0", RangeStart: "192.0.2.100", RangeEnd: "192.0.2.200", LeaseDuration: 3600},
	}
	desired := configuration.Document{SchemaVersion: configuration.SchemaVersion, NodeSpecific: configuration.NodeSpecific{DHCP: dhcp}}
	leasesReset := desired
	leasesReset.ObservedOnly.DHCPLeases = []configuration.DHCPLease{}
	if differences := managedDifferences(desired, leasesReset); len(differences) != 0 {
		t.Fatalf("dynamic lease reset caused managed drift: %#v", differences)
	}
	configurationReset := desired
	configurationReset.NodeSpecific.DHCP = &configuration.DHCPConfig{}
	if differences := managedDifferences(desired, configurationReset); len(differences) == 0 {
		t.Fatal("DHCP configuration reset did not cause managed drift")
	}
}

func TestPreviewOrdersDHCPDisableBeforeEnable(t *testing.T) {
	const (
		clusterID = "11111111-1111-4111-8111-111111111111"
		activeID  = "22222222-2222-4222-8222-222222222222"
		disableID = "33333333-3333-4333-8333-333333333333"
	)
	document := configuration.DesiredDocument{SchemaVersion: configuration.SchemaVersion, NodeOverrides: map[string]configuration.NodeSpecific{
		activeID: {
			BindHosts: []string{"192.0.2.2"}, DNSPort: 53,
			DHCP: &configuration.DHCPConfig{Enabled: true, InterfaceName: "eth0", IPv4: configuration.DHCPIPv4{Gateway: "192.0.2.1", SubnetMask: "255.255.255.0", RangeStart: "192.0.2.100", RangeEnd: "192.0.2.200", LeaseDuration: 3600}},
		},
		disableID: {BindHosts: []string{"192.0.2.3"}, DNSPort: 53, DHCP: &configuration.DHCPConfig{}},
	}}
	repository := &draftRepositoryFake{nodes: []domain.Node{{ID: activeID, ClusterID: clusterID, Enabled: true}, {ID: disableID, ClusterID: clusterID, Enabled: true}}}
	preview, err := NewService(repository).previewDocument(context.Background(), clusterID, "revision", document, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Nodes) != 2 || preview.Nodes[0].NodeID != disableID || preview.Nodes[1].NodeID != activeID {
		t.Fatalf("preview order = %#v", preview.Nodes)
	}
}

func TestReplacementNodeRequiresNewPublishedIdentityAndThenPreviewsSuccessfully(t *testing.T) {
	const (
		clusterID     = "11111111-1111-4111-8111-111111111111"
		deletedID     = "22222222-2222-4222-8222-222222222222"
		replacementID = "33333333-3333-4333-8333-333333333333"
	)
	listener := configuration.NodeSpecific{BindHosts: []string{"192.0.2.10"}, DNSPort: 53}
	oldRevision := configuration.DesiredDocument{
		SchemaVersion: configuration.SchemaVersion,
		NodeOverrides: map[string]configuration.NodeSpecific{deletedID: listener},
	}
	replacementRevision := oldRevision
	replacementRevision.NodeOverrides = map[string]configuration.NodeSpecific{replacementID: listener}
	replacementEffective, err := configuration.Effective(replacementRevision, replacementID)
	if err != nil {
		t.Fatal(err)
	}
	features := map[string]bool{}
	for _, feature := range []string{"dns", "filtering", "clients", "rewrites", "blocked_services", "safety", "query_log", "statistics", "tls", "rewrite_toggle"} {
		features[feature] = true
	}
	repository := &draftRepositoryFake{
		nodes:     []domain.Node{{ID: replacementID, ClusterID: clusterID, Name: "Replacement DNS", Enabled: true}},
		snapshots: []inventory.Snapshot{{NodeID: replacementID, Document: &replacementEffective, CollectionStatus: "succeeded"}},
		profiles:  []inventory.CapabilityProfile{{NodeID: replacementID, Compatibility: string(domain.CompatibilitySupported), Features: features}},
	}
	service := NewService(repository)

	stalePreview, err := service.previewDocument(context.Background(), clusterID, "old-revision", oldRevision, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stalePreview.Valid || len(stalePreview.Nodes) != 1 || stalePreview.Nodes[0].Valid {
		t.Fatalf("pre-replacement revision unexpectedly valid: %#v", stalePreview)
	}
	foundGuidance := false
	for _, issue := range stalePreview.Issues {
		if issue.Field == "nodeOverrides."+replacementID && strings.Contains(issue.Message, "Replacement DNS") && strings.Contains(issue.Message, "publish a new revision") {
			foundGuidance = true
		}
	}
	if !foundGuidance {
		t.Fatalf("replacement guidance missing: %#v", stalePreview.Issues)
	}

	currentPreview, err := service.previewDocument(context.Background(), clusterID, "replacement-revision", replacementRevision, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !currentPreview.Valid || len(currentPreview.Issues) != 0 || len(currentPreview.Nodes) != 1 || !currentPreview.Nodes[0].Valid {
		t.Fatalf("replacement revision did not preview: %#v", currentPreview)
	}
}

func TestPreviewExplainsOnboardingAndMaintenancePrerequisites(t *testing.T) {
	const (
		clusterID = "11111111-1111-4111-8111-111111111111"
		nodeID    = "22222222-2222-4222-8222-222222222222"
	)
	document := configuration.DesiredDocument{
		SchemaVersion: configuration.SchemaVersion,
		NodeOverrides: map[string]configuration.NodeSpecific{nodeID: {BindHosts: []string{"192.0.2.10"}, DNSPort: 53}},
	}
	repository := &draftRepositoryFake{nodes: []domain.Node{{ID: nodeID, ClusterID: clusterID, Name: "New DNS", Enabled: true}}}
	preview, err := NewService(repository).previewDocument(context.Background(), clusterID, "revision", document, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"capabilities have not been refreshed", "successful current configuration observation is required"} {
		found := false
		for _, issue := range preview.Issues {
			found = found || strings.Contains(strings.ToLower(issue.Message), expected)
		}
		if !found {
			t.Fatalf("missing actionable %q issue: %#v", expected, preview.Issues)
		}
	}

	repository.nodes[0].MaintenanceMode = true
	maintenancePreview, err := NewService(repository).previewDocument(context.Background(), clusterID, "revision", document, nil)
	if err != nil {
		t.Fatal(err)
	}
	foundMaintenance := false
	for _, issue := range maintenancePreview.Issues {
		foundMaintenance = foundMaintenance || strings.Contains(issue.Message, "Maintenance Mode")
	}
	if maintenancePreview.Valid || !foundMaintenance {
		t.Fatalf("all-maintenance preview was not actionable: %#v", maintenancePreview)
	}
}

type blockedServiceValidatorFake struct {
	issues []configuration.ValidationIssue
	ids    []string
}

type lifecycleRepositoryFake struct {
	Repository
	revision           Revision
	deployment         Deployment
	revisionArchived   *bool
	deploymentArchived *bool
	revisionDeleted    bool
	deploymentDeleted  bool
	event              domain.AuditEvent
}

func (f *lifecycleRepositoryFake) RevisionByID(context.Context, string) (Revision, error) {
	return f.revision, nil
}

func (f *lifecycleRepositoryFake) DeploymentByID(context.Context, string) (Deployment, error) {
	return f.deployment, nil
}

func (f *lifecycleRepositoryFake) SetRevisionArchived(_ context.Context, _ string, _ string, archived bool, _ time.Time, event domain.AuditEvent) error {
	f.revisionArchived = &archived
	f.event = event
	return nil
}

func (f *lifecycleRepositoryFake) DeleteUnusedRevision(_ context.Context, _ string, event domain.AuditEvent) error {
	f.revisionDeleted = true
	f.event = event
	return nil
}

func (f *lifecycleRepositoryFake) SetDeploymentArchived(_ context.Context, _ string, _ string, archived bool, _ time.Time, event domain.AuditEvent) error {
	f.deploymentArchived = &archived
	f.event = event
	return nil
}

func (f *lifecycleRepositoryFake) DeleteUnstartedDeployment(_ context.Context, _ string, event domain.AuditEvent) error {
	f.deploymentDeleted = true
	f.event = event
	return nil
}

func TestLifecycleMutationsRequireExplicitConfirmationAndAudit(t *testing.T) {
	const (
		revisionID   = "11111111-1111-4111-8111-111111111111"
		deploymentID = "22222222-2222-4222-8222-222222222222"
		clusterID    = "33333333-3333-4333-8333-333333333333"
		userID       = "44444444-4444-4444-8444-444444444444"
	)
	repository := &lifecycleRepositoryFake{
		revision:   Revision{ID: revisionID, ClusterID: clusterID, RevisionNumber: 7},
		deployment: Deployment{ID: deploymentID, ClusterID: clusterID, RevisionID: revisionID},
	}
	service := NewService(repository)
	actor := domain.Actor{UserID: userID, RequestID: "55555555-5555-4555-8555-555555555555"}
	if err := service.SetRevisionArchived(context.Background(), actor, revisionID, true, false); err == nil || repository.revisionArchived != nil {
		t.Fatal("revision archive did not require confirmation")
	}
	if err := service.SetRevisionArchived(context.Background(), actor, revisionID, true, true); err != nil || repository.revisionArchived == nil || !*repository.revisionArchived || repository.event.Action != "configuration.revision_archived" {
		t.Fatalf("revision archive state=%v event=%#v err=%v", repository.revisionArchived, repository.event, err)
	}
	if err := service.DeleteUnusedRevision(context.Background(), actor, revisionID, "DELETE REVISION #6"); err == nil || repository.revisionDeleted {
		t.Fatal("revision deletion accepted the wrong phrase")
	}
	if err := service.DeleteUnusedRevision(context.Background(), actor, revisionID, "DELETE REVISION #7"); err != nil || !repository.revisionDeleted || repository.event.Action != "configuration.revision_deleted_unused" {
		t.Fatalf("revision delete event=%#v err=%v", repository.event, err)
	}
	if err := service.SetDeploymentArchived(context.Background(), actor, deploymentID, true, true); err != nil || repository.deploymentArchived == nil || !*repository.deploymentArchived || repository.event.Action != "deployment.archived" {
		t.Fatalf("deployment archive state=%v event=%#v err=%v", repository.deploymentArchived, repository.event, err)
	}
	if err := service.DeleteUnstartedDeployment(context.Background(), actor, deploymentID, "DELETE DEPLOYMENT wrong"); err == nil || repository.deploymentDeleted {
		t.Fatal("deployment deletion accepted the wrong phrase")
	}
	if err := service.DeleteUnstartedDeployment(context.Background(), actor, deploymentID, "DELETE DEPLOYMENT "+deploymentID); err != nil || !repository.deploymentDeleted || repository.event.Action != "deployment.deleted_unstarted" {
		t.Fatalf("deployment delete event=%#v err=%v", repository.event, err)
	}
}

func (f *blockedServiceValidatorFake) ValidateBlockedServiceIDs(_ context.Context, _ string, ids []string) ([]configuration.ValidationIssue, error) {
	f.ids = append([]string(nil), ids...)
	return f.issues, nil
}

func TestPublicationPreflightIncludesNodeAttributedBlockedServiceCompatibility(t *testing.T) {
	const (
		clusterID = "11111111-1111-4111-8111-111111111111"
		nodeID    = "22222222-2222-4222-8222-222222222222"
	)
	document := configuration.DesiredDocument{
		SchemaVersion: configuration.SchemaVersion,
		Shared: configuration.Shared{
			DNS: configuration.DNS{UpstreamDNS: []string{"1.1.1.1"}}, Filtering: configuration.Filtering{UpdateInterval: 24},
			Services: configuration.Services{BlockedServiceIDs: []string{"chatgpt"}},
		},
		NodeOverrides: map[string]configuration.NodeSpecific{nodeID: {BindHosts: []string{"0.0.0.0"}, DNSPort: 53}},
	}
	effective, err := configuration.Effective(document, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	repository := &draftRepositoryFake{
		draft:     inventory.Draft{ClusterID: clusterID, SchemaVersion: configuration.SchemaVersion, Document: document, Version: 1},
		nodes:     []domain.Node{{ID: nodeID, Enabled: true}},
		snapshots: []inventory.Snapshot{{NodeID: nodeID, Document: &effective, CollectionStatus: "succeeded"}},
		profiles: []inventory.CapabilityProfile{{NodeID: nodeID, Compatibility: string(domain.CompatibilitySupported), Features: map[string]bool{
			"dns": true, "filtering": true, "clients": true, "rewrites": true, "blocked_services": true, "safety": true, "query_log": true, "statistics": true, "tls": true,
		}}},
	}
	validator := &blockedServiceValidatorFake{issues: []configuration.ValidationIssue{{Field: "nodes." + nodeID + ".blockedServices", Message: "Blocked service chatgpt is unsupported."}}}
	preview, err := NewService(repository, validator).ValidateDraft(context.Background(), clusterID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Valid || len(validator.ids) != 1 || validator.ids[0] != "chatgpt" {
		t.Fatalf("compatibility validation was not applied: preview=%#v ids=%#v", preview, validator.ids)
	}
	found := false
	for _, issue := range preview.Issues {
		found = found || issue.Field == "nodes."+nodeID+".blockedServices"
	}
	if !found {
		t.Fatalf("node-attributed issue missing: %#v", preview.Issues)
	}
}

func TestPreviewAcceptsWildcardListenerWithChangedObservedAddresses(t *testing.T) {
	const (
		clusterID = "11111111-1111-4111-8111-111111111111"
		nodeID    = "44444444-4444-4444-8444-444444444444"
	)
	features := map[string]bool{}
	for _, feature := range []string{"dns", "filtering", "clients", "rewrites", "blocked_services", "safety", "query_log", "statistics", "tls", "rewrite_toggle"} {
		features[feature] = true
	}
	observed := configuration.Document{SchemaVersion: configuration.SchemaVersion, NodeSpecific: configuration.NodeSpecific{BindHosts: []string{"10.244.4.137", "127.0.0.1", "::1"}, DNSPort: 5353}}
	preview := func(bindHosts []string) Preview {
		revision := configuration.DesiredDocument{
			SchemaVersion: configuration.SchemaVersion,
			NodeOverrides: map[string]configuration.NodeSpecific{nodeID: {BindHosts: bindHosts, DNSPort: 5353}},
		}
		repository := &draftRepositoryFake{
			nodes:     []domain.Node{{ID: nodeID, ClusterID: clusterID, Name: "Cluster", Enabled: true}},
			snapshots: []inventory.Snapshot{{NodeID: nodeID, Document: &observed, CollectionStatus: "succeeded"}},
			profiles:  []inventory.CapabilityProfile{{NodeID: nodeID, Compatibility: string(domain.CompatibilitySupported), Features: features}},
		}
		result, err := NewService(repository).previewDocument(context.Background(), clusterID, "revision", revision, nil)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	stale := preview([]string{"10.244.6.35", "127.0.0.1", "::1"})
	if stale.Valid || len(stale.Nodes) != 1 || stale.Nodes[0].Valid || !strings.Contains(stale.Nodes[0].Warning, "DNS bind hosts or port differ") {
		t.Fatalf("explicit stale listener unexpectedly valid: %#v", stale)
	}

	wildcard := preview([]string{"0.0.0.0"})
	if !wildcard.Valid || len(wildcard.Issues) != 0 || len(wildcard.Nodes) != 1 || !wildcard.Nodes[0].Valid {
		t.Fatalf("wildcard listener did not preview: %#v", wildcard)
	}
}

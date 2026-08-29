package inventory

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
)

const (
	dhcpOperationClusterID = "11111111-1111-4111-8111-111111111111"
	dhcpOperationNodeID    = "22222222-2222-4222-8222-222222222222"
	dhcpOperationUserID    = "33333333-3333-4333-8333-333333333333"
	dhcpOperationRequestID = "44444444-4444-4444-8444-444444444444"
	dhcpOperationKey       = "55555555-5555-4555-8555-555555555555"
)

type dhcpOperationRepositoryFake struct {
	*fakeRepository
	activeDeployment bool
	policy           domain.ReconciliationPolicy
	operations       map[string]DHCPOperation
	audits           []domain.AuditEvent
	snapshots        []Snapshot
}

func (f *dhcpOperationRepositoryFake) ClusterByID(context.Context, string) (domain.Cluster, error) {
	return domain.Cluster{ID: dhcpOperationClusterID, Name: "Home", ReconciliationPolicy: f.policy}, nil
}

func (f *dhcpOperationRepositoryFake) BeginDHCPOperation(_ context.Context, operation DHCPOperation, event domain.AuditEvent) (DHCPOperation, bool, error) {
	if existing, ok := f.operations[operation.IdempotencyKey]; ok {
		return existing, false, nil
	}
	f.operations[operation.IdempotencyKey] = operation
	f.audits = append(f.audits, event)
	return operation, true, nil
}

func (f *dhcpOperationRepositoryFake) FinishDHCPOperation(_ context.Context, operation DHCPOperation, event domain.AuditEvent) error {
	f.operations[operation.IdempotencyKey] = operation
	f.audits = append(f.audits, event)
	return nil
}

func (f *dhcpOperationRepositoryFake) ListDHCPOperations(context.Context, string, int) ([]DHCPOperation, error) {
	items := make([]DHCPOperation, 0, len(f.operations))
	for _, operation := range f.operations {
		items = append(items, operation)
	}
	return items, nil
}

func (f *dhcpOperationRepositoryFake) ClusterHasActiveDeployment(context.Context, string) (bool, error) {
	return f.activeDeployment, nil
}

func (f *dhcpOperationRepositoryFake) SaveObservation(_ context.Context, snapshot Snapshot, _ CapabilityProfile) error {
	f.snapshots = append(f.snapshots, snapshot)
	return nil
}

type dhcpOperationReaderFake struct {
	leasesCalls int
	configCalls int
	err         error
	observed    configuration.Document
}

func (f *dhcpOperationReaderFake) ResetDHCPLeases(context.Context, domain.NodeProbeRequest) error {
	f.leasesCalls++
	return f.err
}

func (f *dhcpOperationReaderFake) ResetDHCPConfiguration(context.Context, domain.NodeProbeRequest) error {
	f.configCalls++
	return f.err
}

func (f *dhcpOperationReaderFake) ReadConfiguration(context.Context, domain.NodeProbeRequest, string) (configuration.Document, CapabilityProfile, error) {
	return f.observed, CapabilityProfile{SchemaVersion: configuration.SchemaVersion, Features: map[string]bool{}}, nil
}

func dhcpOperationFixture(reader *dhcpOperationReaderFake) (*Service, *dhcpOperationRepositoryFake, *Draft) {
	dhcp := &configuration.DHCPConfig{Enabled: true, InterfaceName: "eth0", IPv4: configuration.DHCPIPv4{Gateway: "192.0.2.1", SubnetMask: "255.255.255.0", RangeStart: "192.0.2.100", RangeEnd: "192.0.2.200", LeaseDuration: 3600}}
	draft := &Draft{Document: configuration.DesiredDocument{SchemaVersion: configuration.SchemaVersion, NodeOverrides: map[string]configuration.NodeSpecific{dhcpOperationNodeID: {DHCP: dhcp}}}}
	base := &fakeRepository{draft: draft, nodeRecord: domain.NodeRecord{Node: domain.Node{
		ID: dhcpOperationNodeID, ClusterID: dhcpOperationClusterID, Name: "Primary",
		Enabled: true, MaintenanceMode: true, BaseURL: "http://private-node.test",
		CertificatePolicy: domain.CertificateInsecureHTTP, Version: "v0.107.78",
	}}}
	repository := &dhcpOperationRepositoryFake{fakeRepository: base, policy: domain.ReconciliationManual, operations: map[string]DHCPOperation{}}
	reader.observed = configuration.Document{
		SchemaVersion: configuration.SchemaVersion,
		NodeSpecific:  configuration.NodeSpecific{DHCP: &configuration.DHCPConfig{}},
		ObservedOnly:  configuration.ObservedOnly{DHCPLeases: []configuration.DHCPLease{}},
	}
	return NewService(repository, unusedCredentials{}, reader), repository, draft
}

func TestResetDHCPLeasesPersistsAuditsAndRefreshesObservation(t *testing.T) {
	reader := &dhcpOperationReaderFake{}
	service, repository, draft := dhcpOperationFixture(reader)
	before := draft.Document
	operation, err := service.RunDHCPOperation(context.Background(), domain.Actor{UserID: dhcpOperationUserID, RequestID: dhcpOperationRequestID}, dhcpOperationNodeID, DHCPOperationResetLeases, DHCPResetLeasesConfirmation, dhcpOperationKey)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != "succeeded" || operation.ObservationStatus != "succeeded" || operation.ObservationSnapshotID == "" || operation.AuditReference == "" || reader.leasesCalls != 1 {
		t.Fatalf("operation=%#v calls=%d", operation, reader.leasesCalls)
	}
	if len(repository.snapshots) != 1 || len(repository.audits) != 2 || repository.audits[0].Action != "dhcp.reset_leases_requested" || repository.audits[1].Action != "dhcp.reset_leases_succeeded" {
		t.Fatalf("snapshots=%#v audits=%#v", repository.snapshots, repository.audits)
	}
	if repository.audits[1].ID != operation.AuditReference || repository.audits[1].RequestID != dhcpOperationRequestID {
		t.Fatalf("terminal audit mismatch: %#v", repository.audits[1])
	}
	if !reflect.DeepEqual(before, draft.Document) {
		t.Fatalf("desired state changed: before=%#v after=%#v", before, draft.Document)
	}
	for _, event := range repository.audits {
		text := strings.ToLower(event.Action + event.RequestID)
		for key, value := range event.Metadata {
			text += strings.ToLower(key) + strings.ToLower(strings.TrimSpace(toString(value)))
		}
		for _, forbidden := range []string{"private-node", "password", "credential", "upstream body"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("audit exposed %q: %#v", forbidden, event)
			}
		}
	}
}

func TestResetDHCPConfigurationPersistsSafeFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		kind domain.ErrorKind
	}{
		{name: "node rejected", err: domain.NewError(domain.ErrorNodeApply, "AdGuard Home rejected POST /control/dhcp/reset with HTTP 500"), kind: domain.ErrorNodeApply},
		{name: "timeout", err: domain.NewError(domain.ErrorNodeUnreachable, "the AdGuard Home node could not be reached"), kind: domain.ErrorNodeUnreachable},
		{name: "invalid response", err: domain.NewError(domain.ErrorNodeResponse, "unsafe upstream text"), kind: domain.ErrorNodeResponse},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &dhcpOperationReaderFake{err: test.err}
			service, repository, _ := dhcpOperationFixture(reader)
			key := []string{
				"55555555-5555-4555-8555-555555555551",
				"55555555-5555-4555-8555-555555555552",
				"55555555-5555-4555-8555-555555555553",
			}[index]
			operation, err := service.RunDHCPOperation(context.Background(), domain.Actor{UserID: dhcpOperationUserID, RequestID: dhcpOperationRequestID}, dhcpOperationNodeID, DHCPOperationResetConfiguration, DHCPResetConfigurationConfirmation, key)
			var domainError *domain.Error
			if !errors.As(err, &domainError) || domainError.Kind != test.kind {
				t.Fatalf("error=%#v", err)
			}
			if operation.Status != "failed" || operation.NodeResults[0].ErrorCode != string(test.kind) || operation.AuditReference == "" || len(repository.snapshots) != 0 {
				t.Fatalf("operation=%#v snapshots=%#v", operation, repository.snapshots)
			}
			terminal := repository.audits[1]
			if terminal.Action != "dhcp.reset_configuration_failed" || terminal.Metadata["errorCode"] != string(test.kind) {
				t.Fatalf("terminal audit=%#v", terminal)
			}
			if strings.Contains(strings.ToLower(toString(terminal.Metadata)), "unsafe upstream") {
				t.Fatalf("audit leaked raw error: %#v", terminal)
			}
		})
	}
}

func TestDHCPOperationGuardsMaintenanceAndActiveDeployment(t *testing.T) {
	tests := []struct {
		name   string
		change func(*dhcpOperationRepositoryFake)
	}{
		{name: "maintenance required", change: func(repository *dhcpOperationRepositoryFake) { repository.nodeRecord.Node.MaintenanceMode = false }},
		{name: "active deployment", change: func(repository *dhcpOperationRepositoryFake) { repository.activeDeployment = true }},
		{name: "enabled required", change: func(repository *dhcpOperationRepositoryFake) { repository.nodeRecord.Node.Enabled = false }},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &dhcpOperationReaderFake{}
			service, repository, _ := dhcpOperationFixture(reader)
			test.change(repository)
			key := []string{
				"66666666-6666-4666-8666-666666666661",
				"66666666-6666-4666-8666-666666666662",
				"66666666-6666-4666-8666-666666666663",
			}[index]
			operation, err := service.RunDHCPOperation(context.Background(), domain.Actor{UserID: dhcpOperationUserID, RequestID: dhcpOperationRequestID}, dhcpOperationNodeID, DHCPOperationResetLeases, DHCPResetLeasesConfirmation, key)
			if err == nil || operation.Status != "failed" || reader.leasesCalls != 0 || len(repository.audits) != 2 {
				t.Fatalf("operation=%#v err=%v calls=%d audits=%#v", operation, err, reader.leasesCalls, repository.audits)
			}
		})
	}
}

func TestResetDHCPConfigurationRejectsEnforceReconciliation(t *testing.T) {
	reader := &dhcpOperationReaderFake{}
	service, repository, _ := dhcpOperationFixture(reader)
	repository.policy = domain.ReconciliationEnforce
	operation, err := service.RunDHCPOperation(
		context.Background(),
		domain.Actor{UserID: dhcpOperationUserID, RequestID: dhcpOperationRequestID},
		dhcpOperationNodeID, DHCPOperationResetConfiguration,
		DHCPResetConfigurationConfirmation, "66666666-6666-4666-8666-666666666664",
	)
	if err == nil || operation.Status != "failed" || reader.configCalls != 0 || len(repository.audits) != 2 {
		t.Fatalf("operation=%#v err=%v calls=%d audits=%#v", operation, err, reader.configCalls, repository.audits)
	}
}

func TestDHCPOperationDuplicateSubmitCallsNodeOnce(t *testing.T) {
	reader := &dhcpOperationReaderFake{}
	service, repository, _ := dhcpOperationFixture(reader)
	actor := domain.Actor{UserID: dhcpOperationUserID, RequestID: dhcpOperationRequestID}
	first, err := service.RunDHCPOperation(context.Background(), actor, dhcpOperationNodeID, DHCPOperationResetConfiguration, DHCPResetConfigurationConfirmation, dhcpOperationKey)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.RunDHCPOperation(context.Background(), actor, dhcpOperationNodeID, DHCPOperationResetConfiguration, DHCPResetConfigurationConfirmation, dhcpOperationKey)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || !second.Duplicate || reader.configCalls != 1 || len(repository.audits) != 2 || len(repository.snapshots) != 1 {
		t.Fatalf("first=%#v second=%#v calls=%d audits=%d snapshots=%d", first, second, reader.configCalls, len(repository.audits), len(repository.snapshots))
	}
}

func TestDHCPOperationRequiresExplicitScopeConfirmationAndIdempotency(t *testing.T) {
	service, _, _ := dhcpOperationFixture(&dhcpOperationReaderFake{})
	actor := domain.Actor{UserID: dhcpOperationUserID, RequestID: dhcpOperationRequestID}
	for name, values := range map[string][3]string{
		"node":         {"", DHCPResetLeasesConfirmation, dhcpOperationKey},
		"confirmation": {dhcpOperationNodeID, "RESET EVERYTHING", dhcpOperationKey},
		"idempotency":  {dhcpOperationNodeID, DHCPResetLeasesConfirmation, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.RunDHCPOperation(context.Background(), actor, values[0], DHCPOperationResetLeases, values[1], values[2]); err == nil {
				t.Fatal("invalid destructive request succeeded")
			}
		})
	}
}

func toString(value any) string {
	return fmt.Sprint(value)
}

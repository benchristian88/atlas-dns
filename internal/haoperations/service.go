package haoperations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

const BreakGlassConfirmation = "CONTINUE_WITHOUT_DNS_REDUNDANCY"

type Repository interface {
	domain.ManagementRepository
	PollableNodes(context.Context) ([]domain.NodeRecord, error)
	NodeLifecycleSettings(context.Context, string) (NodeSettings, error)
	SaveNodeLifecycleSettings(context.Context, NodeSettings, int, domain.AuditEvent) error
	LatestDNSProbe(context.Context, string) (DNSProbeResult, error)
	LatestDNSProbes(context.Context, string) ([]DNSProbeResult, error)
	SaveDNSProbe(context.Context, DNSProbeResult, *Event) error
	ListHAEvents(context.Context, string, string, int) ([]Event, error)
	ListHAHistory(context.Context, HistoryQuery) ([]HistoryItem, error)
	RecordHAEvent(context.Context, Event) error
	RecordHAEventAndAudit(context.Context, Event, domain.AuditEvent) error
	LatestSuccessfulSnapshots(context.Context, string) ([]inventory.Snapshot, error)
	LatestSnapshots(context.Context, string) ([]inventory.Snapshot, error)
	ActiveDeploymentExists(context.Context, string) (bool, error)
	OpenDriftExists(context.Context, string) (bool, error)
	CreateUpgrade(context.Context, Upgrade, domain.AuditEvent, Event) error
	UpdateUpgrade(context.Context, Upgrade, domain.AuditEvent, Event) error
	UpgradeByID(context.Context, string) (Upgrade, error)
	ListUpgrades(context.Context, string, int) ([]Upgrade, error)
	ReleaseCache(context.Context) (ReleaseCache, error)
	SaveReleaseCache(context.Context, ReleaseCache) error
	CollectorChecks(context.Context, string) ([]Check, error)
}

type MaintenanceManager interface {
	SetNodeMaintenance(context.Context, domain.Actor, string, bool, int) (domain.Node, error)
}

type CredentialDecrypter interface {
	Decrypt(string, domain.EncryptedCredentials) (domain.NodeCredentials, error)
}

type Observer interface {
	Observe(context.Context, string) (inventory.Snapshot, error)
}

type Service struct {
	repository    Repository
	maintenance   MaintenanceManager
	observer      Observer
	apiProbe      domain.NodeStatusProbe
	credentials   CredentialDecrypter
	dnsProbe      DNSProber
	now           func() time.Time
	warningDays   int
	criticalDays  int
	compatibility func(string) domain.Compatibility
}

func NewService(repository Repository, maintenance MaintenanceManager, observer Observer, apiProbe domain.NodeStatusProbe, credentials CredentialDecrypter, dnsProbe DNSProber) *Service {
	return &Service{repository: repository, maintenance: maintenance, observer: observer, apiProbe: apiProbe, credentials: credentials, dnsProbe: dnsProbe, now: time.Now, warningDays: 30, criticalDays: 7, compatibility: func(string) domain.Compatibility { return domain.CompatibilityUnknown }}
}

func (s *Service) SetVersionCompatibility(check func(string) domain.Compatibility) {
	if check != nil {
		s.compatibility = check
	}
}

func (s *Service) Settings(ctx context.Context, nodeID string) (NodeSettings, error) {
	if !domain.ValidID(nodeID) {
		return NodeSettings{}, domain.Validation("nodeId", "must be a valid UUID")
	}
	settings, err := s.repository.NodeLifecycleSettings(ctx, nodeID)
	if err == nil {
		return settings, nil
	}
	var de *domain.Error
	if !errors.As(err, &de) || de.Kind != domain.ErrorNotFound {
		return NodeSettings{}, err
	}
	node, nodeErr := s.repository.NodeByID(ctx, nodeID)
	if nodeErr != nil {
		return NodeSettings{}, nodeErr
	}
	now := s.now().UTC()
	dnsPort := 53
	if snapshots, snapshotErr := s.repository.LatestSuccessfulSnapshots(ctx, node.ClusterID); snapshotErr == nil {
		for _, snapshot := range snapshots {
			if snapshot.NodeID == nodeID && snapshot.Document != nil && snapshot.Document.NodeSpecific.DNSPort > 0 {
				dnsPort = snapshot.Document.NodeSpecific.DNSPort
				break
			}
		}
	}
	return NodeSettings{NodeID: node.ID, DNSProbePort: dnsPort, DNSProbeName: ".", DNSProbeType: "NS", ProbeUDP: true, ProbeTCP: true, InstallationType: InstallationUnknown, RecordVersion: 0, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) UpdateSettings(ctx context.Context, actor domain.Actor, nodeID string, input NodeSettings, expectedVersion int) (NodeSettings, error) {
	node, err := s.repository.NodeByID(ctx, nodeID)
	if err != nil {
		return NodeSettings{}, err
	}
	input.NodeID = nodeID
	input.DNSProbeHost = strings.TrimSpace(input.DNSProbeHost)
	input.DNSProbeName = strings.TrimSpace(strings.ToLower(input.DNSProbeName))
	input.DNSProbeType = strings.ToUpper(strings.TrimSpace(input.DNSProbeType))
	if input.DNSProbePort < 1 || input.DNSProbePort > 65535 {
		return NodeSettings{}, domain.Validation("dnsProbePort", "must be between 1 and 65535")
	}
	if input.DNSProbeName == "" || len(input.DNSProbeName) > 253 {
		return NodeSettings{}, domain.Validation("dnsProbeName", "must be a valid DNS name")
	}
	if input.DNSProbeType != "A" && input.DNSProbeType != "AAAA" && input.DNSProbeType != "NS" {
		return NodeSettings{}, domain.Validation("dnsProbeType", "must be A, AAAA, or NS")
	}
	if input.ExpectedRCode < 0 || input.ExpectedRCode > 15 {
		return NodeSettings{}, domain.Validation("expectedRcode", "must be between 0 and 15")
	}
	if !input.ProbeUDP && !input.ProbeTCP {
		return NodeSettings{}, domain.Validation("probeProtocols", "must enable UDP or TCP")
	}
	if !input.InstallationType.Valid() {
		return NodeSettings{}, domain.Validation("installationType", "is not supported")
	}
	if input.DNSProbeHost != "" {
		if strings.ContainsAny(input.DNSProbeHost, "/?#@") || len(input.DNSProbeHost) > 255 {
			return NodeSettings{}, domain.Validation("dnsProbeHost", "must be a hostname or IP address")
		}
	}
	now := s.now().UTC()
	input.CreatedAt, input.UpdatedAt = now, now
	input.RecordVersion = expectedVersion + 1
	event, err := audit(actor, "node.lifecycle_settings_changed", "node", nodeID, map[string]any{"clusterId": node.ClusterID, "installationType": input.InstallationType}, now)
	if err != nil {
		return NodeSettings{}, err
	}
	if err := s.repository.SaveNodeLifecycleSettings(ctx, input, expectedVersion, event); err != nil {
		return NodeSettings{}, err
	}
	return input, nil
}

func (s *Service) ProbeNode(ctx context.Context, nodeID string) (DNSProbeResult, error) {
	record, err := s.repository.NodeRecordByID(ctx, nodeID)
	if err != nil {
		return DNSProbeResult{}, err
	}
	if !record.Node.Enabled {
		return DNSProbeResult{}, domain.NewError(domain.ErrorConflict, "DNS probe requires an enabled node")
	}
	settings, err := s.Settings(ctx, nodeID)
	if err != nil {
		return DNSProbeResult{}, err
	}
	host := settings.DNSProbeHost
	if host == "" {
		parsed, parseErr := url.Parse(record.Node.BaseURL)
		if parseErr != nil {
			return DNSProbeResult{}, domain.NewError(domain.ErrorValidation, "node URL cannot provide a DNS probe target")
		}
		host = parsed.Hostname()
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsUnspecified() || ip.IsLoopback()) {
		return DNSProbeResult{}, domain.NewError(domain.ErrorValidation, "DNS probe target must be remotely reachable from the controller")
	}
	result, probeErr := s.dnsProbe.Probe(ctx, DNSProbeRequest{Host: host, Port: settings.DNSProbePort, Name: settings.DNSProbeName, Type: settings.DNSProbeType, ExpectedRCode: settings.ExpectedRCode, UDP: settings.ProbeUDP, TCP: settings.ProbeTCP})
	id, idErr := domain.NewID()
	if idErr != nil {
		return DNSProbeResult{}, idErr
	}
	result.ID, result.NodeID, result.ClusterID = id, nodeID, record.Node.ClusterID
	previous, previousErr := s.repository.LatestDNSProbe(ctx, nodeID)
	var transition *Event
	if previousErr != nil || previous.Status != result.Status {
		eventType, severity, summary := "dns.recovered", "info", "DNS service recovered"
		if result.Status != "healthy" {
			eventType, severity, summary = "dns.failed", "critical", "DNS service probe failed"
		}
		transitionValue, eventErr := s.newEvent(record.Node.ClusterID, &nodeID, eventType, severity, summary, map[string]any{"errorCode": result.ErrorCode}, result.ProbedAt)
		if eventErr != nil {
			return DNSProbeResult{}, eventErr
		}
		transition = &transitionValue
	}
	if err := s.repository.SaveDNSProbe(ctx, result, transition); err != nil {
		return DNSProbeResult{}, err
	}
	return result, probeErr
}

func (s *Service) PollAll(ctx context.Context) error {
	records, err := s.repository.PollableNodes(ctx)
	if err != nil {
		return err
	}
	clusters := map[string]bool{}
	semaphore := make(chan struct{}, 4)
	errorsChannel := make(chan error, len(records))
	done := make(chan struct{})
	for _, record := range records {
		record := record
		clusters[record.Node.ClusterID] = true
		go func() {
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore; done <- struct{}{} }()
			case <-ctx.Done():
				done <- struct{}{}
				return
			}
			if _, probeErr := s.ProbeNode(ctx, record.Node.ID); probeErr != nil {
				errorsChannel <- probeErr
			}
		}()
	}
	for range records {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	close(errorsChannel)
	for clusterID := range clusters {
		if transitionErr := s.recordRedundancyTransition(ctx, clusterID); transitionErr != nil {
			return transitionErr
		}
		if transitionErr := s.recordCertificateTransitions(ctx, clusterID); transitionErr != nil {
			return transitionErr
		}
		if transitionErr := s.recordVersionTransitions(ctx, clusterID); transitionErr != nil {
			return transitionErr
		}
	}
	// Per-node failures are durable probe evidence and must not stop healthy-node
	// collection or mark the process worker failed.
	return nil
}

func (s *Service) recordVersionTransitions(ctx context.Context, clusterID string) error {
	versions, err := s.Versions(ctx, clusterID)
	if err != nil {
		return err
	}
	events, err := s.repository.ListHAEvents(ctx, clusterID, "", 200)
	if err != nil {
		return err
	}
	for _, version := range versions {
		if version.LatestVersion == "" || version.ReleaseCheckStale {
			continue
		}
		lastState, lastTarget := "", ""
		for _, event := range events {
			if event.NodeID == nil || *event.NodeID != version.NodeID || !strings.HasPrefix(event.EventType, "version.") {
				continue
			}
			lastState = event.EventType
			lastTarget = fmt.Sprint(event.Details["latestVersion"])
			break
		}
		eventType, severity, summary := "version.current", "info", "AdGuard Home is current"
		if version.UpdateAvailable {
			eventType, severity, summary = "version.update_available", "warning", "AdGuard Home update available"
		}
		if lastState == eventType && lastTarget == version.LatestVersion {
			continue
		}
		nodeID := version.NodeID
		event, eventErr := s.newEvent(clusterID, &nodeID, eventType, severity, summary, map[string]any{"installedVersion": version.InstalledVersion, "latestVersion": version.LatestVersion}, s.now().UTC())
		if eventErr != nil {
			return eventErr
		}
		if err := s.repository.RecordHAEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recordCertificateTransitions(ctx context.Context, clusterID string) error {
	certificates, err := s.Certificates(ctx, clusterID)
	if err != nil {
		return err
	}
	events, err := s.repository.ListHAEvents(ctx, clusterID, "", 200)
	if err != nil {
		return err
	}
	for _, certificate := range certificates {
		previous := ""
		for _, event := range events {
			if event.NodeID == nil || *event.NodeID != certificate.NodeID || !strings.HasPrefix(event.EventType, "certificate.") {
				continue
			}
			previous = strings.TrimPrefix(event.EventType, "certificate.")
			if previous == "recovered" {
				previous = "healthy"
			}
			break
		}
		current := string(certificate.State)
		if current == "unknown" || current == previous {
			continue
		}
		eventType, severity, summary := "certificate."+current, "warning", "Certificate expiry warning"
		if current == "healthy" {
			eventType, severity, summary = "certificate.recovered", "info", "Certificate status recovered"
			current = "recovered"
		}
		if certificate.State == CertificateCritical || certificate.State == CertificateExpired {
			severity = "critical"
		}
		event, eventErr := s.newEvent(clusterID, &certificate.NodeID, eventType, severity, summary, map[string]any{"state": certificate.State, "daysRemaining": certificate.DaysRemaining}, s.now().UTC())
		if eventErr != nil {
			return eventErr
		}
		if err := s.repository.RecordHAEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recordRedundancyTransition(ctx context.Context, clusterID string) error {
	summary, err := s.Summary(ctx, clusterID)
	if err != nil {
		return err
	}
	events, err := s.repository.ListHAEvents(ctx, clusterID, "", 50)
	if err != nil {
		return err
	}
	previous := ""
	for _, event := range events {
		if event.EventType == "redundancy.restored" {
			previous = "healthy"
			break
		}
		if event.EventType == "redundancy.degraded" {
			previous = "degraded"
			break
		}
		if event.EventType == "redundancy.at_risk" {
			previous = "at_risk"
			break
		}
	}
	if previous == summary.State {
		return nil
	}
	eventType, severity, message := "redundancy.restored", "info", "DNS redundancy restored"
	if summary.State == "degraded" {
		eventType, severity, message = "redundancy.degraded", "warning", "DNS redundancy degraded"
	} else if summary.State == "at_risk" {
		eventType, severity, message = "redundancy.at_risk", "critical", "DNS service is at risk"
	}
	event, err := s.newEvent(clusterID, nil, eventType, severity, message, map[string]any{"servingDnsNodes": summary.ServingDNSNodes, "totalNodes": summary.TotalNodes}, s.now().UTC())
	if err != nil {
		return err
	}
	return s.repository.RecordHAEvent(ctx, event)
}

func (s *Service) Summary(ctx context.Context, clusterID string) (HASummary, error) {
	nodes, err := s.repository.ListNodes(ctx, clusterID)
	if err != nil {
		return HASummary{}, err
	}
	probes, err := s.repository.LatestDNSProbes(ctx, clusterID)
	if err != nil {
		return HASummary{}, err
	}
	probeByNode := map[string]DNSProbeResult{}
	for _, probe := range probes {
		probeByNode[probe.NodeID] = probe
	}
	// Keep the collection shape stable for a valid cluster with no configured
	// nodes.  A nil slice serializes as null and forces browser clients to treat
	// an ordinary first-run state as a malformed response.
	summary := HASummary{Nodes: []HANodeStatus{}}
	for _, node := range nodes {
		dnsState := HANodeStatus{NodeID: node.ID, DNSStatus: "unknown", UDPStatus: "unknown", TCPStatus: "unknown"}
		if probe, ok := probeByNode[node.ID]; ok {
			dnsState.DNSStatus, dnsState.UDPStatus, dnsState.TCPStatus = probe.Status, probe.UDPStatus, probe.TCPStatus
			dnsState.DNSProbedAt, dnsState.ErrorCode = &probe.ProbedAt, probe.ErrorCode
			if s.now().UTC().Sub(probe.ProbedAt) > 2*time.Minute {
				dnsState.DNSStatus = "stale"
			}
		}
		if node.MaintenanceMode {
			dnsState.DNSStatus = "maintenance"
		} else if !node.Enabled {
			dnsState.DNSStatus = "disabled"
		}
		summary.Nodes = append(summary.Nodes, dnsState)
		if !node.Enabled {
			continue
		}
		summary.TotalNodes++
		if node.MaintenanceMode {
			summary.MaintenanceNodes++
		}
		if node.Enabled && node.HealthStatus == domain.NodeHealthy {
			summary.APIReachableNodes++
		}
		if node.Enabled && !node.MaintenanceMode && node.ConvergenceStatus == "converged" {
			summary.ConvergedNodes++
		}
		if probe, ok := probeByNode[node.ID]; ok && node.Enabled && !node.MaintenanceMode && probe.Status == "healthy" && s.now().UTC().Sub(probe.ProbedAt) <= 2*time.Minute {
			summary.ServingDNSNodes++
		}
	}
	certificates, _ := s.Certificates(ctx, clusterID)
	for _, certificate := range certificates {
		if certificate.State == CertificateWarning || certificate.State == CertificateCritical || certificate.State == CertificateExpired {
			summary.CertificateWarnings++
		}
	}
	versions, _ := s.Versions(ctx, clusterID)
	for _, version := range versions {
		if version.UpdateAvailable {
			summary.UpdateAvailableNodes++
		}
	}
	switch {
	case summary.ServingDNSNodes < 2:
		summary.State, summary.Message = "at_risk", "DNS is serving with no verified node redundancy."
	case summary.APIReachableNodes < summary.TotalNodes || summary.ConvergedNodes < summary.TotalNodes || summary.MaintenanceNodes > 0:
		summary.State, summary.Message = "degraded", "DNS redundancy remains, but one or more operational dimensions need attention."
	default:
		summary.State, summary.Message = "healthy", "Verified DNS capacity can tolerate one node failure."
	}
	return summary, nil
}

func (s *Service) Certificates(ctx context.Context, clusterID string) ([]Certificate, error) {
	nodes, err := s.repository.ListNodes(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	snapshots, err := s.repository.LatestSuccessfulSnapshots(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	byNode := map[string]inventory.Snapshot{}
	for _, snapshot := range snapshots {
		byNode[snapshot.NodeID] = snapshot
	}
	result := make([]Certificate, 0, len(nodes))
	for _, node := range nodes {
		certificate := Certificate{NodeID: node.ID, NodeName: node.Name, State: CertificateUnknown}
		if snapshot, ok := byNode[node.ID]; ok && snapshot.Document != nil {
			certificate.Subject, certificate.Issuer, certificate.ObservedAt = snapshot.Document.ObservedOnly.TLS.Subject, snapshot.Document.ObservedOnly.TLS.Issuer, &snapshot.ObservedAt
			if expiry, ok := parseCertificateTime(snapshot.Document.ObservedOnly.TLS.NotAfter); ok {
				certificate.NotAfter = &expiry
				days := int(expiry.Sub(s.now().UTC()).Hours() / 24)
				certificate.DaysRemaining = &days
				switch {
				case days < 0:
					certificate.State = CertificateExpired
				case days <= s.criticalDays:
					certificate.State = CertificateCritical
				case days <= s.warningDays:
					certificate.State = CertificateWarning
				default:
					certificate.State = CertificateHealthy
				}
			} else if snapshot.Document.ObservedOnly.TLS.Enabled && snapshot.Document.ObservedOnly.TLS.ValidCertificate {
				certificate.State = CertificateHealthy
			}
		}
		result = append(result, certificate)
	}
	return result, nil
}

func (s *Service) Versions(ctx context.Context, clusterID string) ([]VersionState, error) {
	nodes, err := s.repository.ListNodes(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	cache, cacheErr := s.repository.ReleaseCache(ctx)
	result := make([]VersionState, 0, len(nodes))
	for _, node := range nodes {
		settings, settingsErr := s.Settings(ctx, node.ID)
		if settingsErr != nil {
			return nil, settingsErr
		}
		value := VersionState{NodeID: node.ID, NodeName: node.Name, InstalledVersion: node.Version, Compatibility: string(node.CompatibilityStatus), InstallationType: settings.InstallationType, UpgradeSupport: SupportForInstallation(settings.InstallationType), ReleaseCheckStale: cacheErr != nil || s.now().UTC().After(cache.ExpiresAt)}
		if cacheErr == nil {
			value.LatestVersion = cache.Version
			value.UpdateAvailable = compareVersions(node.Version, cache.Version) < 0
		}
		result = append(result, value)
	}
	return result, nil
}

func (s *Service) MaintenancePreflight(ctx context.Context, nodeID string) (MaintenancePreflight, error) {
	node, err := s.repository.NodeByID(ctx, nodeID)
	if err != nil {
		return MaintenancePreflight{}, err
	}
	summary, err := s.Summary(ctx, node.ClusterID)
	if err != nil {
		return MaintenancePreflight{}, err
	}
	activeDeployment, err := s.repository.ActiveDeploymentExists(ctx, node.ClusterID)
	if err != nil {
		return MaintenancePreflight{}, err
	}
	openDrift, err := s.repository.OpenDriftExists(ctx, nodeID)
	if err != nil {
		return MaintenancePreflight{}, err
	}
	activeDHCP := s.nodeActiveDHCP(ctx, node.ClusterID, nodeID)
	remaining := summary.ServingDNSNodes
	if latest, latestErr := s.repository.LatestDNSProbe(ctx, nodeID); latestErr == nil && latest.Status == "healthy" && !node.MaintenanceMode && s.now().UTC().Sub(latest.ProbedAt) <= 2*time.Minute {
		remaining--
	}
	if remaining < 0 {
		remaining = 0
	}
	preflight := MaintenancePreflight{NodeID: nodeID, HealthyDNSNodesRemaining: remaining, ActiveDeployment: activeDeployment, OpenDrift: openDrift, ActiveDHCP: activeDHCP, ExpectedRedundancy: "healthy"}
	if remaining < 2 {
		preflight.ExpectedRedundancy = "at_risk"
	}
	if remaining == 0 {
		preflight.BreakGlassRequired = true
	}
	targetDNSHealthy := false
	if latest, latestErr := s.repository.LatestDNSProbe(ctx, nodeID); latestErr == nil {
		targetDNSHealthy = latest.Status == "healthy" && s.now().UTC().Sub(latest.ProbedAt) <= 2*time.Minute
	}
	tlsCheck := Check{Name: "tls", Status: "unknown", ErrorCode: "TLS_STATE_UNAVAILABLE", Message: "TLS configuration state is unavailable"}
	if snapshots, snapshotErr := s.repository.LatestSuccessfulSnapshots(ctx, node.ClusterID); snapshotErr == nil {
		for _, snapshot := range snapshots {
			if snapshot.NodeID == nodeID {
				tlsCheck = returnTLSCheck(snapshot, nil, s.now().UTC())
				tlsCheck.Required = false // TLS is informational when entering maintenance.
				break
			}
		}
	}
	peersCompatible := true
	if nodes, nodesErr := s.repository.ListNodes(ctx, node.ClusterID); nodesErr == nil {
		for _, peer := range nodes {
			if peer.ID != nodeID && peer.Enabled && peer.CompatibilityStatus != domain.CompatibilitySupported {
				peersCompatible = false
			}
		}
	}
	preflight.Checks = []Check{
		{Name: "dns_capacity", Status: ternary(remaining > 0, "pass", "warning"), Required: true, Message: fmt.Sprintf("%d verified DNS nodes remain", remaining)},
		{Name: "target_dns", Status: ternary(targetDNSHealthy, "pass", "warning"), Message: ternary(targetDNSHealthy, "Target currently answers the configured DNS probe", "Target has no fresh healthy DNS proof")},
		{Name: "active_deployment", Status: ternary(!activeDeployment, "pass", "fail"), Required: true, Message: ternary(!activeDeployment, "No active deployment", "An active deployment must finish first")},
		{Name: "drift", Status: ternary(!openDrift, "pass", "warning"), Message: ternary(!openDrift, "No open drift", "Open drift remains visible during maintenance")},
		{Name: "dhcp", Status: ternary(!activeDHCP, "pass", "fail"), Required: true, Message: ternary(!activeDHCP, "Node is not active DHCP", "Complete an audited DHCP handoff before maintenance")},
		tlsCheck,
		{Name: "peer_compatibility", Status: ternary(peersCompatible, "pass", "warning"), Message: ternary(peersCompatible, "Enabled peers are inside the tested controller compatibility range", "One or more enabled peers are outside the tested controller compatibility range")},
		{Name: "api", Status: ternary(node.HealthStatus == domain.NodeHealthy, "pass", "warning"), Message: "Management API health is evaluated independently"},
	}
	preflight.Allowed = !activeDeployment && !activeDHCP && (remaining > 0 || preflight.BreakGlassRequired)
	return preflight, nil
}

func (s *Service) EnterMaintenance(ctx context.Context, actor domain.Actor, nodeID string, expectedVersion int, breakGlass bool, confirmation string) (domain.Node, error) {
	node, err := s.repository.NodeByID(ctx, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	if node.MaintenanceMode {
		return node, nil
	}
	preflight, err := s.MaintenancePreflight(ctx, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	if !preflight.Allowed {
		return domain.Node{}, domain.NewError(domain.ErrorConflict, "maintenance preflight contains a blocking check")
	}
	if preflight.BreakGlassRequired && (!breakGlass || confirmation != BreakGlassConfirmation) {
		return domain.Node{}, domain.Validation("confirmation", "break-glass confirmation is required")
	}
	node, err = s.maintenance.SetNodeMaintenance(ctx, actor, nodeID, true, expectedVersion)
	if err != nil {
		return domain.Node{}, err
	}
	event, eventErr := s.newEvent(node.ClusterID, &nodeID, "maintenance.started", ternary(preflight.BreakGlassRequired, "critical", "warning"), "Planned maintenance started", map[string]any{"breakGlass": preflight.BreakGlassRequired, "healthyDnsNodesRemaining": preflight.HealthyDNSNodesRemaining}, s.now().UTC())
	if eventErr != nil {
		return domain.Node{}, eventErr
	}
	if err := s.repository.RecordHAEvent(ctx, event); err != nil {
		return domain.Node{}, err
	}
	return node, nil
}

func (s *Service) ReturnToService(ctx context.Context, actor domain.Actor, nodeID string, expectedVersion int) (ReturnValidation, error) {
	record, err := s.repository.NodeRecordByID(ctx, nodeID)
	if err != nil {
		return ReturnValidation{}, err
	}
	validation := ReturnValidation{NodeID: nodeID}
	if !record.Node.MaintenanceMode {
		validation.Succeeded = true
		validation.Checks = []Check{{Name: "maintenance_state", Status: "pass", Required: true, Message: "Node is already outside maintenance"}}
		return validation, nil
	}
	credentials, credentialErr := s.credentials.Decrypt(nodeID, record.Secrets.Credentials)
	var apiErr error
	if credentialErr == nil {
		_, apiErr = s.apiProbe.Status(ctx, domain.NodeProbeRequest{BaseURL: record.Node.BaseURL, CertificatePolicy: record.Node.CertificatePolicy, CustomCAPEM: record.Secrets.CustomCAPEM, Credentials: credentials})
	} else {
		apiErr = credentialErr
	}
	validation.Checks = append(validation.Checks, Check{Name: "api", Status: ternary(apiErr == nil, "pass", "fail"), Required: true, ErrorCode: safeErrorCode(apiErr), Message: "Authenticated AdGuard Home management API"})
	snapshot, observationErr := s.observer.Observe(ctx, nodeID)
	validation.Checks = append(validation.Checks, Check{Name: "observation_capabilities", Status: ternary(observationErr == nil && snapshot.CollectionStatus == "succeeded", "pass", "fail"), Required: true, ErrorCode: safeErrorCode(observationErr), Message: "Fresh configuration observation and capability refresh"})
	dnsResult, dnsErr := s.ProbeNode(ctx, nodeID)
	validation.Checks = append(validation.Checks, Check{Name: "dns", Status: ternary(dnsErr == nil && dnsResult.Status == "healthy", "pass", "fail"), Required: true, ErrorCode: dnsResult.ErrorCode, Message: "Active DNS query over configured protocols"})
	openDrift, driftErr := s.repository.OpenDriftExists(ctx, nodeID)
	cluster, clusterErr := s.repository.ClusterByID(ctx, record.Node.ClusterID)
	configurationStateAvailable := driftErr == nil && clusterErr == nil
	converged := configurationStateAvailable && !openDrift && record.Node.ConvergenceStatus != "drifted"
	if clusterErr == nil && cluster.ActiveRevisionID != nil {
		converged = converged && record.Node.AppliedRevisionID != nil && *record.Node.AppliedRevisionID == *cluster.ActiveRevisionID
	}
	configurationStatus := "pass"
	configurationCode := ""
	configurationMessage := "No unresolved drift and the active revision is applied"
	configurationRequired := false
	if !configurationStateAvailable {
		configurationStatus = "fail"
		configurationCode = "CONFIGURATION_STATE_UNAVAILABLE"
		configurationMessage = "Configuration and drift state could not be verified"
		configurationRequired = true
	} else if !converged {
		// Maintenance suppresses deployment and reconciliation.  Existing drift
		// therefore cannot be a blocking return check: leaving maintenance is the
		// transition that makes the node eligible for repair again.
		configurationStatus = "warning"
		configurationCode = "CONFIGURATION_RECONCILIATION_PENDING"
		configurationMessage = "Configuration reconciliation is pending and will resume after maintenance"
	}
	validation.Checks = append(validation.Checks, Check{Name: "convergence_drift", Status: configurationStatus, Required: configurationRequired, ErrorCode: configurationCode, Message: configurationMessage})
	activeDHCP := s.nodeActiveDHCP(ctx, record.Node.ClusterID, nodeID)
	activeDHCPCount, activeDHCPError := s.activeDHCPNodes(ctx, record.Node.ClusterID)
	dhcpSafe := activeDHCPError == nil && activeDHCPCount <= 1
	dhcpCode, dhcpMessage := "", ternary(activeDHCP, "Node is the single observed active DHCP node", "No active DHCP responsibility observed")
	if activeDHCPError != nil {
		dhcpCode, dhcpMessage = "DHCP_SAFETY_UNAVAILABLE", "DHCP responsibility could not be verified"
	} else if activeDHCPCount > 1 {
		dhcpCode, dhcpMessage = "MULTIPLE_ACTIVE_DHCP_NODES", "Multiple active DHCP nodes were observed"
	}
	validation.Checks = append(validation.Checks, Check{Name: "dhcp_safety", Status: ternary(dhcpSafe, "pass", "fail"), Required: true, ErrorCode: dhcpCode, Message: dhcpMessage})
	validation.Checks = append(validation.Checks, returnTLSCheck(snapshot, observationErr, s.now().UTC()))
	collectorChecks, collectorErr := s.repository.CollectorChecks(ctx, nodeID)
	if collectorErr != nil {
		collectorChecks = []Check{{Name: "collectors", Status: "fail", Required: true, ErrorCode: "COLLECTOR_VALIDATION_FAILED", Message: "Collector state could not be verified"}}
	}
	validation.Checks = append(validation.Checks, collectorChecks...)
	validation.Succeeded = true
	for _, check := range validation.Checks {
		if check.Required && check.Status != "pass" {
			validation.Succeeded = false
		}
	}
	if !validation.Succeeded {
		now := s.now().UTC()
		failedChecks := failedCheckNames(validation.Checks)
		event, eventErr := s.newEvent(record.Node.ClusterID, &nodeID, "maintenance.return_validation_failed", "critical", "Return-to-service validation failed", map[string]any{"checks": failedChecks}, now)
		if eventErr != nil {
			return validation, eventErr
		}
		auditEvent, auditErr := audit(actor, "node.maintenance_return_failed", "node", nodeID, map[string]any{"clusterId": record.Node.ClusterID, "failedChecks": failedChecks}, now)
		if auditErr != nil {
			return validation, auditErr
		}
		if err := s.repository.RecordHAEventAndAudit(ctx, event, auditEvent); err != nil {
			return validation, err
		}
		return validation, domain.NewError(domain.ErrorVerification, returnValidationFailureMessage(validation.Checks))
	}
	node, err := s.maintenance.SetNodeMaintenance(ctx, actor, nodeID, false, expectedVersion)
	if err != nil {
		return validation, err
	}
	event, _ := s.newEvent(node.ClusterID, &nodeID, "maintenance.ended", "info", "Node returned to service", map[string]any{"validation": "passed"}, s.now().UTC())
	if err := s.repository.RecordHAEvent(ctx, event); err != nil {
		return validation, err
	}
	return validation, nil
}

func (s *Service) Lifecycle(ctx context.Context, nodeID string) (NodeLifecycle, error) {
	node, err := s.repository.NodeByID(ctx, nodeID)
	if err != nil {
		return NodeLifecycle{}, err
	}
	settings, err := s.Settings(ctx, nodeID)
	if err != nil {
		return NodeLifecycle{}, err
	}
	probe, probeErr := s.repository.LatestDNSProbe(ctx, nodeID)
	certificates, _ := s.Certificates(ctx, node.ClusterID)
	versions, _ := s.Versions(ctx, node.ClusterID)
	events, err := s.repository.ListHAEvents(ctx, node.ClusterID, nodeID, 50)
	if err != nil {
		return NodeLifecycle{}, err
	}
	result := NodeLifecycle{GeneratedAt: s.now().UTC(), Settings: settings, Events: events, Certificate: Certificate{NodeID: nodeID, NodeName: node.Name, State: CertificateUnknown}, Version: VersionState{NodeID: nodeID, NodeName: node.Name}}
	if probeErr == nil {
		result.DNS = &probe
	}
	for _, value := range certificates {
		if value.NodeID == nodeID {
			result.Certificate = value
		}
	}
	for _, value := range versions {
		if value.NodeID == nodeID {
			result.Version = value
		}
	}
	return result, nil
}

func (s *Service) StartUpgrade(ctx context.Context, actor domain.Actor, nodeID, targetVersion string) (Upgrade, error) {
	node, err := s.repository.NodeByID(ctx, nodeID)
	if err != nil {
		return Upgrade{}, err
	}
	settings, err := s.Settings(ctx, nodeID)
	if err != nil {
		return Upgrade{}, err
	}
	if SupportForInstallation(settings.InstallationType) != UpgradeGuided {
		return Upgrade{}, domain.NewError(domain.ErrorCapability, "this installation type has no supported controller upgrade workflow")
	}
	if node.CompatibilityStatus != domain.CompatibilitySupported {
		return Upgrade{}, domain.NewError(domain.ErrorCapability, "the current node version is outside the supported controller compatibility range")
	}
	if !node.MaintenanceMode {
		return Upgrade{}, domain.NewError(domain.ErrorConflict, "guided upgrade requires the node to be in maintenance")
	}
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" || len(targetVersion) > 128 {
		return Upgrade{}, domain.Validation("targetVersion", "is required")
	}
	if s.compatibility(targetVersion) != domain.CompatibilitySupported {
		return Upgrade{}, domain.NewError(domain.ErrorCapability, "the target version is outside the tested controller compatibility range")
	}
	preflight, err := s.MaintenancePreflight(ctx, nodeID)
	if err != nil {
		return Upgrade{}, err
	}
	if !preflight.Allowed {
		return Upgrade{}, domain.NewError(domain.ErrorConflict, "upgrade preflight contains a blocking check")
	}
	if preflight.OpenDrift {
		return Upgrade{}, domain.NewError(domain.ErrorConflict, "resolve or adopt node drift before upgrading")
	}
	id, err := domain.NewID()
	if err != nil {
		return Upgrade{}, err
	}
	now := s.now().UTC()
	upgrade := Upgrade{ID: id, ClusterID: node.ClusterID, NodeID: nodeID, FromVersion: node.Version, TargetVersion: targetVersion, InstallationType: settings.InstallationType, Mode: "guided", Status: "awaiting_operator", RequestedBy: actor.UserID, RequestID: actor.RequestID, Preflight: map[string]any{"healthyDnsNodesRemaining": preflight.HealthyDNSNodesRemaining, "expectedRedundancy": preflight.ExpectedRedundancy}, Validation: map[string]any{}, StartedAt: now}
	auditEvent, err := audit(actor, "upgrade.started", "upgrade", id, map[string]any{"nodeId": nodeID, "targetVersion": targetVersion, "mode": "guided"}, now)
	if err != nil {
		return Upgrade{}, err
	}
	event, err := s.newEvent(node.ClusterID, &nodeID, "upgrade.started", "warning", "Guided AdGuard Home upgrade started", map[string]any{"upgradeId": id, "targetVersion": targetVersion}, now)
	if err != nil {
		return Upgrade{}, err
	}
	if err := s.repository.CreateUpgrade(ctx, upgrade, auditEvent, event); err != nil {
		return Upgrade{}, err
	}
	return upgrade, nil
}

func (s *Service) CompleteUpgrade(ctx context.Context, actor domain.Actor, upgradeID string, expectedNodeVersion int) (Upgrade, error) {
	upgrade, err := s.repository.UpgradeByID(ctx, upgradeID)
	if err != nil {
		return Upgrade{}, err
	}
	if upgrade.Status != "awaiting_operator" {
		return Upgrade{}, domain.NewError(domain.ErrorConflict, "upgrade is not awaiting validation")
	}
	upgrade.Status = "validating"
	record, nodeErr := s.repository.NodeRecordByID(ctx, upgrade.NodeID)
	var validation ReturnValidation
	var validationErr error
	observedVersion := ""
	if nodeErr == nil {
		credentials, credentialErr := s.credentials.Decrypt(upgrade.NodeID, record.Secrets.Credentials)
		if credentialErr == nil {
			status, statusErr := s.apiProbe.Status(ctx, domain.NodeProbeRequest{BaseURL: record.Node.BaseURL, CertificatePolicy: record.Node.CertificatePolicy, CustomCAPEM: record.Secrets.CustomCAPEM, Credentials: credentials})
			if statusErr == nil {
				observedVersion = status.Version
			}
		}
	}
	if observedVersion == "" || compareVersions(observedVersion, upgrade.TargetVersion) != 0 {
		validation = ReturnValidation{NodeID: upgrade.NodeID, Checks: []Check{{Name: "expected_version", Status: "fail", Required: true, ErrorCode: "UPGRADE_VERSION_MISMATCH", Message: "Installed version does not match the requested target"}}}
		validationErr = domain.NewError(domain.ErrorVerification, "node remains in maintenance because the expected target version was not observed")
	} else {
		validation, validationErr = s.ReturnToService(ctx, actor, upgrade.NodeID, expectedNodeVersion)
	}
	upgrade.Validation = map[string]any{"succeeded": validation.Succeeded, "checks": validation.Checks}
	now := s.now().UTC()
	upgrade.CompletedAt = &now
	eventType, severity, summary, action := "upgrade.succeeded", "info", "AdGuard Home upgrade validated", "upgrade.succeeded"
	if validationErr != nil {
		upgrade.Status, upgrade.ErrorCode, upgrade.ErrorSummary = "failed", "POST_UPGRADE_VALIDATION_FAILED", "Post-upgrade validation failed; node remains in maintenance"
		eventType, severity, summary, action = "upgrade.validation_failed", "critical", "Post-upgrade validation failed", "upgrade.failed"
	} else {
		upgrade.Status = "succeeded"
	}
	auditEvent, err := audit(actor, action, "upgrade", upgrade.ID, map[string]any{"nodeId": upgrade.NodeID, "status": upgrade.Status}, now)
	if err != nil {
		return Upgrade{}, err
	}
	event, err := s.newEvent(upgrade.ClusterID, &upgrade.NodeID, eventType, severity, summary, map[string]any{"upgradeId": upgrade.ID, "errorCode": upgrade.ErrorCode}, now)
	if err != nil {
		return Upgrade{}, err
	}
	if err := s.repository.UpdateUpgrade(ctx, upgrade, auditEvent, event); err != nil {
		return Upgrade{}, err
	}
	return upgrade, validationErr
}

type historyCursorPayload struct {
	Version int    `json:"v"`
	At      string `json:"at"`
	ID      string `json:"id"`
}

func (s *Service) History(ctx context.Context, request HistoryRequest) (HistoryPage, error) {
	if !domain.ValidID(request.ClusterID) {
		return HistoryPage{}, domain.Validation("clusterId", "must be a valid UUID")
	}
	if request.NodeID != "" && !domain.ValidID(request.NodeID) {
		return HistoryPage{}, domain.Validation("nodeId", "must be a valid UUID")
	}
	if request.Limit < 1 || request.Limit > 100 {
		return HistoryPage{}, domain.Validation("limit", "must be between 1 and 100")
	}
	if _, err := s.repository.ClusterByID(ctx, request.ClusterID); err != nil {
		return HistoryPage{}, err
	}
	if request.NodeID != "" {
		node, err := s.repository.NodeByID(ctx, request.NodeID)
		if err != nil {
			return HistoryPage{}, err
		}
		if node.ClusterID != request.ClusterID {
			return HistoryPage{}, domain.NewError(domain.ErrorNotFound, "node was not found in the cluster")
		}
	}
	query := HistoryQuery{ClusterID: request.ClusterID, NodeID: request.NodeID, Limit: request.Limit + 1}
	if request.Cursor != "" {
		at, id, err := decodeHistoryCursor(request.Cursor)
		if err != nil {
			return HistoryPage{}, domain.Validation("cursor", "is invalid or unsupported")
		}
		query.BeforeAt, query.BeforeID = &at, id
	}
	items, err := s.repository.ListHAHistory(ctx, query)
	if err != nil {
		return HistoryPage{}, err
	}
	page := HistoryPage{Items: items}
	if len(page.Items) > request.Limit {
		page.Items = page.Items[:request.Limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeHistoryCursor(last.OccurredAt, last.ID)
		page.HasMore = true
	}
	return page, nil
}

func encodeHistoryCursor(at time.Time, id string) string {
	body, _ := json.Marshal(historyCursorPayload{Version: 1, At: at.UTC().Format(time.RFC3339Nano), ID: id})
	return base64.RawURLEncoding.EncodeToString(body)
}

func decodeHistoryCursor(value string) (time.Time, string, error) {
	body, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decode cursor")
	}
	var payload historyCursorPayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.Version != 1 || !domain.ValidID(payload.ID) {
		return time.Time{}, "", fmt.Errorf("decode cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, payload.At)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decode cursor")
	}
	return at.UTC(), payload.ID, nil
}
func (s *Service) Upgrades(ctx context.Context, clusterID string, limit int) ([]Upgrade, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	return s.repository.ListUpgrades(ctx, clusterID, limit)
}

func (s *Service) nodeActiveDHCP(ctx context.Context, clusterID, nodeID string) bool {
	snapshots, err := s.repository.LatestSuccessfulSnapshots(ctx, clusterID)
	if err != nil {
		return false
	}
	for _, snapshot := range snapshots {
		if snapshot.NodeID == nodeID && snapshot.Document != nil && snapshot.Document.NodeSpecific.DHCP != nil && snapshot.Document.NodeSpecific.DHCP.Enabled {
			return true
		}
	}
	return false
}

func (s *Service) activeDHCPNodes(ctx context.Context, clusterID string) (int, error) {
	snapshots, err := s.repository.LatestSuccessfulSnapshots(ctx, clusterID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, snapshot := range snapshots {
		if snapshot.Document != nil && snapshot.Document.NodeSpecific.DHCP != nil && snapshot.Document.NodeSpecific.DHCP.Enabled {
			count++
		}
	}
	return count, nil
}

func (s *Service) newEvent(clusterID string, nodeID *string, eventType, severity, summary string, details map[string]any, at time.Time) (Event, error) {
	id, err := domain.NewID()
	if err != nil {
		return Event{}, err
	}
	return Event{ID: id, ClusterID: clusterID, NodeID: nodeID, EventType: eventType, Severity: severity, Summary: summary, Details: details, OccurredAt: at.UTC()}, nil
}

func audit(actor domain.Actor, action, resourceType, resourceID string, metadata map[string]any, at time.Time) (domain.AuditEvent, error) {
	id, err := domain.NewID()
	if err != nil {
		return domain.AuditEvent{}, err
	}
	userID := actor.UserID
	return domain.AuditEvent{ID: id, ActorType: "user", ActorUserID: &userID, Action: action, ResourceType: resourceType, ResourceID: &resourceID, RequestID: actor.RequestID, Metadata: metadata, CreatedAt: at.UTC()}, nil
}
func parseCertificateTime(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05Z07:00"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

// returnTLSCheck validates the redacted AdGuard Home /control/tls/status data
// collected by the fresh configuration observation.  It does not connect to an
// HTTPS, DNS-over-TLS, or DNS-over-QUIC listener; HTTPS administration transport
// is independently verified by the required API check when the node URL is HTTPS.
func returnTLSCheck(snapshot inventory.Snapshot, observationErr error, now time.Time) Check {
	unknown := Check{
		Name:      "tls",
		Status:    "unknown",
		Required:  true,
		ErrorCode: "TLS_STATE_UNAVAILABLE",
		Message:   "TLS configuration state could not be verified from the fresh observation",
	}
	if observationErr != nil || snapshot.CollectionStatus != "succeeded" || snapshot.Document == nil {
		return unknown
	}

	tls := snapshot.Document.ObservedOnly.TLS
	if !tls.Enabled {
		return Check{
			Name:     "tls",
			Status:   "not_applicable",
			Required: false,
			Message:  "TLS encryption is not enabled on this node; check is not applicable",
		}
	}

	failure := func(code, message string) Check {
		return Check{Name: "tls", Status: "fail", Required: true, ErrorCode: code, Message: message}
	}
	if strings.TrimSpace(tls.NotBefore) != "" {
		notBefore, ok := parseCertificateTime(tls.NotBefore)
		if !ok {
			return failure("TLS_CERTIFICATE_TIME_INVALID", "TLS certificate start time could not be interpreted")
		}
		if now.Before(notBefore) {
			return failure("TLS_CERTIFICATE_NOT_YET_VALID", "TLS certificate is not valid before "+notBefore.Format("2006-01-02"))
		}
	}
	if strings.TrimSpace(tls.NotAfter) != "" {
		notAfter, ok := parseCertificateTime(tls.NotAfter)
		if !ok {
			return failure("TLS_CERTIFICATE_TIME_INVALID", "TLS certificate expiry could not be interpreted")
		}
		if !now.Before(notAfter) {
			return failure("TLS_CERTIFICATE_EXPIRED", "TLS certificate expired on "+notAfter.Format("2006-01-02"))
		}
	}
	if !tls.ValidCertificate {
		return failure("TLS_CERTIFICATE_INVALID", "AdGuard Home reports that the TLS certificate is invalid")
	}
	if !tls.ValidChain {
		return failure("TLS_CERTIFICATE_CHAIN_INVALID", "AdGuard Home reports that the TLS certificate chain is invalid")
	}
	if !tls.ValidKey {
		return failure("TLS_PRIVATE_KEY_INVALID", "AdGuard Home reports that the TLS private key is invalid")
	}
	if !tls.ValidPair {
		return failure("TLS_CERTIFICATE_KEY_MISMATCH", "AdGuard Home reports that the TLS certificate and private key do not match")
	}
	return Check{Name: "tls", Status: "pass", Required: true, Message: "TLS encryption is enabled and AdGuard Home reports valid certificate metadata"}
}

func returnValidationFailureMessage(checks []Check) string {
	names := failedCheckNames(checks)
	details := make([]string, 0, len(names))
	for _, name := range names {
		for _, check := range checks {
			if check.Name != name || check.Message == "" {
				continue
			}
			label := strings.ToUpper(strings.ReplaceAll(name, "_", " "))
			details = append(details, label+": "+check.Message)
			break
		}
	}
	message := "node remains in maintenance because required return-to-service checks failed: " + strings.Join(names, ", ")
	if len(details) > 0 {
		message += ". " + strings.Join(details, "; ")
	}
	return message
}
func compareVersions(left, right string) int {
	clean := func(value string) []int {
		value = strings.TrimPrefix(strings.TrimSpace(value), "v")
		parts := strings.Split(value, ".")
		result := make([]int, 3)
		for index := range result {
			if index < len(parts) {
				fmt.Sscanf(parts[index], "%d", &result[index])
			}
		}
		return result
	}
	a, b := clean(left), clean(right)
	for index := range a {
		if a[index] < b[index] {
			return -1
		}
		if a[index] > b[index] {
			return 1
		}
	}
	return 0
}
func ternary[T any](condition bool, yes, no T) T {
	if condition {
		return yes
	}
	return no
}
func safeErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return string(de.Kind)
	}
	return "OPERATION_FAILED"
}
func failedCheckNames(checks []Check) []string {
	result := []string{}
	for _, check := range checks {
		if check.Required && check.Status != "pass" {
			result = append(result, check.Name)
		}
	}
	sort.Strings(result)
	return result
}

var _ = configuration.SchemaVersion

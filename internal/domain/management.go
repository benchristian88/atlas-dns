package domain

import (
	"context"
	"errors"
	"strings"
	"time"
)

type ManagementRepository interface {
	CreateCluster(context.Context, Cluster, AuditEvent) error
	ListClusters(context.Context) ([]Cluster, error)
	ClusterByID(context.Context, string) (Cluster, error)
	UpdateCluster(context.Context, Cluster, int, AuditEvent) error
	CreateNode(context.Context, NodeRecord, AuditEvent) error
	ListNodes(context.Context, string) ([]Node, error)
	NodeByID(context.Context, string) (Node, error)
	NodeRecordByID(context.Context, string) (NodeRecord, error)
	UpdateNode(context.Context, NodeRecord, int, AuditEvent) error
	SoftDeleteNode(context.Context, string, int, time.Time, AuditEvent) error
	UpdateNodeHealth(context.Context, string, NodeHealth, Compatibility, string, *int, string, time.Time, bool) error
	RecordNodeTestResult(context.Context, string, NodeHealth, Compatibility, string, *int, string, time.Time, bool, AuditEvent) error
	SetNodeMaintenance(context.Context, string, bool, int, time.Time, AuditEvent) error
}

type CredentialProtector interface {
	Encrypt(string, NodeCredentials) (EncryptedCredentials, error)
	Decrypt(string, EncryptedCredentials) (NodeCredentials, error)
}

type NodeStatusProbe interface {
	Status(context.Context, NodeProbeRequest) (NodeProbeResult, error)
}

type ManagementService struct {
	repository  ManagementRepository
	credentials CredentialProtector
	probe       NodeStatusProbe
	now         func() time.Time
}

func NewManagementService(repository ManagementRepository, credentials CredentialProtector, probe NodeStatusProbe) *ManagementService {
	return &ManagementService{repository: repository, credentials: credentials, probe: probe, now: time.Now}
}

type Actor struct {
	UserID    string
	RequestID string
}

type CreateClusterInput struct {
	Name                 string
	Description          string
	ReconciliationPolicy ReconciliationPolicy
}

func (s *ManagementService) CreateCluster(ctx context.Context, actor Actor, input CreateClusterInput) (Cluster, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if err := ValidateResourceName("name", input.Name); err != nil {
		return Cluster{}, err
	}
	if len(input.Description) > 2000 {
		return Cluster{}, Validation("description", "must not exceed 2000 characters")
	}
	if input.ReconciliationPolicy == "" {
		input.ReconciliationPolicy = ReconciliationManual
	}
	if !ValidReconciliationPolicy(input.ReconciliationPolicy) {
		return Cluster{}, Validation("reconciliationPolicy", "must be enforce, alert, or manual")
	}
	id, err := NewID()
	if err != nil {
		return Cluster{}, err
	}
	now := s.now().UTC()
	cluster := Cluster{ID: id, Name: input.Name, Description: input.Description, ReconciliationPolicy: input.ReconciliationPolicy, Version: 1, CreatedAt: now, UpdatedAt: now}
	event, err := newUserAudit(actor, "cluster.created", "cluster", id, map[string]any{"name": cluster.Name, "reconciliationPolicy": cluster.ReconciliationPolicy}, now)
	if err != nil {
		return Cluster{}, err
	}
	if err := s.repository.CreateCluster(ctx, cluster, event); err != nil {
		return Cluster{}, err
	}
	return cluster, nil
}

func (s *ManagementService) ListClusters(ctx context.Context) ([]Cluster, error) {
	return s.repository.ListClusters(ctx)
}

func (s *ManagementService) Cluster(ctx context.Context, id string) (Cluster, error) {
	if !ValidID(id) {
		return Cluster{}, Validation("clusterId", "must be a valid UUID")
	}
	return s.repository.ClusterByID(ctx, id)
}

func (s *ManagementService) UpdateCluster(ctx context.Context, actor Actor, id string, expectedVersion int, input CreateClusterInput) (Cluster, error) {
	if expectedVersion < 1 {
		return Cluster{}, Validation("version", "must be a positive integer")
	}
	cluster, err := s.Cluster(ctx, id)
	if err != nil {
		return Cluster{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if err := ValidateResourceName("name", input.Name); err != nil {
		return Cluster{}, err
	}
	if len(input.Description) > 2000 {
		return Cluster{}, Validation("description", "must not exceed 2000 characters")
	}
	if input.ReconciliationPolicy == "" {
		input.ReconciliationPolicy = cluster.ReconciliationPolicy
	}
	if !ValidReconciliationPolicy(input.ReconciliationPolicy) {
		return Cluster{}, Validation("reconciliationPolicy", "must be enforce, alert, or manual")
	}
	previousPolicy := cluster.ReconciliationPolicy
	cluster.Name = input.Name
	cluster.Description = input.Description
	cluster.ReconciliationPolicy = input.ReconciliationPolicy
	cluster.UpdatedAt = s.now().UTC()
	action := "cluster.updated"
	metadata := map[string]any{"name": cluster.Name, "reconciliationPolicy": cluster.ReconciliationPolicy}
	if previousPolicy != cluster.ReconciliationPolicy {
		action = "cluster.reconciliation_policy_changed"
		metadata["previousReconciliationPolicy"] = previousPolicy
	}
	event, err := newUserAudit(actor, action, "cluster", id, metadata, cluster.UpdatedAt)
	if err != nil {
		return Cluster{}, err
	}
	if err := s.repository.UpdateCluster(ctx, cluster, expectedVersion, event); err != nil {
		return Cluster{}, err
	}
	cluster.Version = expectedVersion + 1
	return cluster, nil
}

type CreateNodeInput struct {
	ClusterID         string
	Name              string
	BaseURL           string
	CertificatePolicy CertificatePolicy
	CustomCAPEM       string
	Username          string
	Password          string
	Enabled           bool
}

func (s *ManagementService) ValidateNodeCandidate(ctx context.Context, input CreateNodeInput) (NodeProbeResult, error) {
	if !ValidID(input.ClusterID) {
		return NodeProbeResult{}, Validation("clusterId", "must be a valid UUID")
	}
	if _, err := s.repository.ClusterByID(ctx, input.ClusterID); err != nil {
		return NodeProbeResult{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if err := ValidateResourceName("name", input.Name); err != nil {
		return NodeProbeResult{}, err
	}
	if err := validateNodeCredentials(input.Username, input.Password); err != nil {
		return NodeProbeResult{}, err
	}
	if err := ValidateCertificatePolicy(input.CertificatePolicy, input.CustomCAPEM); err != nil {
		return NodeProbeResult{}, err
	}
	baseURL, err := NormaliseNodeURL(input.BaseURL, input.CertificatePolicy)
	if err != nil {
		return NodeProbeResult{}, err
	}
	return s.probe.Status(ctx, NodeProbeRequest{
		BaseURL: baseURL, CertificatePolicy: input.CertificatePolicy, CustomCAPEM: input.CustomCAPEM,
		Credentials: NodeCredentials{Username: input.Username, Password: input.Password},
	})
}

func (s *ManagementService) CreateNode(ctx context.Context, actor Actor, input CreateNodeInput) (Node, error) {
	if !ValidID(input.ClusterID) {
		return Node{}, Validation("clusterId", "must be a valid UUID")
	}
	if _, err := s.repository.ClusterByID(ctx, input.ClusterID); err != nil {
		return Node{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if err := ValidateResourceName("name", input.Name); err != nil {
		return Node{}, err
	}
	if err := validateNodeCredentials(input.Username, input.Password); err != nil {
		return Node{}, err
	}
	if err := ValidateCertificatePolicy(input.CertificatePolicy, input.CustomCAPEM); err != nil {
		return Node{}, err
	}
	baseURL, err := NormaliseNodeURL(input.BaseURL, input.CertificatePolicy)
	if err != nil {
		return Node{}, err
	}
	probeResult, err := s.ValidateNodeCandidate(ctx, input)
	if err != nil {
		return Node{}, err
	}
	id, err := NewID()
	if err != nil {
		return Node{}, err
	}
	envelope, err := s.credentials.Encrypt(id, NodeCredentials{Username: input.Username, Password: input.Password})
	if err != nil {
		return Node{}, err
	}
	now := s.now().UTC()
	health, errorCode := healthFromProbe(probeResult, input.Enabled)
	latency := probeResult.LatencyMS
	node := Node{
		ID: id, ClusterID: input.ClusterID, Name: input.Name, BaseURL: baseURL,
		CertificatePolicy: input.CertificatePolicy, Enabled: input.Enabled,
		HealthStatus: health, CompatibilityStatus: probeResult.Compatibility, Version: probeResult.Version,
		LastSeenAt: &now, LastPolledAt: &now, LatencyMS: &latency, LastErrorCode: errorCode,
		ConvergenceStatus: "pending",
		RecordVersion:     1, CreatedAt: now, UpdatedAt: now,
	}
	event, err := newUserAudit(actor, "node.created", "node", id, map[string]any{
		"clusterId": input.ClusterID, "name": input.Name, "certificatePolicy": input.CertificatePolicy,
	}, now)
	if err != nil {
		return Node{}, err
	}
	record := NodeRecord{Node: node, Secrets: NodeSecretMaterial{Credentials: envelope, CustomCAPEM: strings.TrimSpace(input.CustomCAPEM)}}
	if err := s.repository.CreateNode(ctx, record, event); err != nil {
		return Node{}, err
	}
	return node, nil
}

func (s *ManagementService) ListNodes(ctx context.Context, clusterID string) ([]Node, error) {
	if !ValidID(clusterID) {
		return nil, Validation("clusterId", "must be a valid UUID")
	}
	if _, err := s.repository.ClusterByID(ctx, clusterID); err != nil {
		return nil, err
	}
	return s.repository.ListNodes(ctx, clusterID)
}

func (s *ManagementService) Node(ctx context.Context, id string) (Node, error) {
	if !ValidID(id) {
		return Node{}, Validation("nodeId", "must be a valid UUID")
	}
	return s.repository.NodeByID(ctx, id)
}

type UpdateNodeInput struct {
	Name              string
	BaseURL           string
	CertificatePolicy CertificatePolicy
	CustomCAPEM       *string
	Username          *string
	Password          *string
	Enabled           bool
	ExpectedVersion   int
}

func (s *ManagementService) UpdateNode(ctx context.Context, actor Actor, id string, input UpdateNodeInput) (Node, error) {
	if !ValidID(id) {
		return Node{}, Validation("nodeId", "must be a valid UUID")
	}
	if input.ExpectedVersion < 1 {
		return Node{}, Validation("version", "must be a positive integer")
	}
	record, err := s.repository.NodeRecordByID(ctx, id)
	if err != nil {
		return Node{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if err := ValidateResourceName("name", input.Name); err != nil {
		return Node{}, err
	}
	customCAPEM := ""
	if input.CertificatePolicy == CertificateCustomCA {
		customCAPEM = record.Secrets.CustomCAPEM
		if input.CustomCAPEM != nil {
			customCAPEM = strings.TrimSpace(*input.CustomCAPEM)
		}
	}
	if err := ValidateCertificatePolicy(input.CertificatePolicy, customCAPEM); err != nil {
		return Node{}, err
	}
	baseURL, err := NormaliseNodeURL(input.BaseURL, input.CertificatePolicy)
	if err != nil {
		return Node{}, err
	}
	credentials, err := s.credentials.Decrypt(id, record.Secrets.Credentials)
	if err != nil {
		return Node{}, errors.New("stored node credentials could not be decrypted")
	}
	credentialsChanged := input.Username != nil || input.Password != nil
	if (input.Username == nil) != (input.Password == nil) {
		return Node{}, Validation("credentials", "username and password must be supplied together")
	}
	if credentialsChanged {
		credentials = NodeCredentials{Username: *input.Username, Password: *input.Password}
		if err := validateNodeCredentials(credentials.Username, credentials.Password); err != nil {
			return Node{}, err
		}
	}
	if input.Enabled {
		result, err := s.probe.Status(ctx, NodeProbeRequest{
			BaseURL: baseURL, CertificatePolicy: input.CertificatePolicy,
			CustomCAPEM: customCAPEM, Credentials: credentials,
		})
		if err != nil {
			return Node{}, err
		}
		now := s.now().UTC()
		health, errorCode := healthFromProbe(result, true)
		latency := result.LatencyMS
		record.Node.HealthStatus = health
		record.Node.CompatibilityStatus = result.Compatibility
		record.Node.Version = result.Version
		record.Node.LastSeenAt = &now
		record.Node.LastPolledAt = &now
		record.Node.LatencyMS = &latency
		record.Node.LastErrorCode = errorCode
	}
	if credentialsChanged {
		record.Secrets.Credentials, err = s.credentials.Encrypt(id, credentials)
		if err != nil {
			return Node{}, err
		}
	}
	record.Node.Name = input.Name
	record.Node.BaseURL = baseURL
	record.Node.CertificatePolicy = input.CertificatePolicy
	record.Node.Enabled = input.Enabled
	if record.Node.MaintenanceMode {
		record.Node.ConvergenceStatus = "maintenance"
	} else {
		record.Node.ConvergenceStatus = "pending"
	}
	if !input.Enabled {
		record.Node.HealthStatus = NodeDisabled
		record.Node.LastErrorCode = ""
	}
	record.Secrets.CustomCAPEM = customCAPEM
	record.Node.UpdatedAt = s.now().UTC()
	action := "node.updated"
	if credentialsChanged {
		action = "node.credentials_rotated"
	}
	event, err := newUserAudit(actor, action, "node", id, map[string]any{
		"name": input.Name, "enabled": input.Enabled, "certificatePolicy": input.CertificatePolicy,
	}, record.Node.UpdatedAt)
	if err != nil {
		return Node{}, err
	}
	if err := s.repository.UpdateNode(ctx, record, input.ExpectedVersion, event); err != nil {
		return Node{}, err
	}
	record.Node.RecordVersion = input.ExpectedVersion + 1
	return record.Node, nil
}

func (s *ManagementService) TestNodeConnection(ctx context.Context, actor Actor, id string) (NodeProbeResult, error) {
	if !ValidID(id) {
		return NodeProbeResult{}, Validation("nodeId", "must be a valid UUID")
	}
	record, err := s.repository.NodeRecordByID(ctx, id)
	if err != nil {
		return NodeProbeResult{}, err
	}
	credentials, err := s.credentials.Decrypt(id, record.Secrets.Credentials)
	if err != nil {
		return NodeProbeResult{}, errors.New("stored node credentials could not be decrypted")
	}
	now := s.now().UTC()
	result, probeErr := s.probe.Status(ctx, NodeProbeRequest{
		BaseURL: record.Node.BaseURL, CertificatePolicy: record.Node.CertificatePolicy,
		CustomCAPEM: record.Secrets.CustomCAPEM, Credentials: credentials,
	})
	if probeErr != nil {
		code := string(ErrorNodeUnreachable)
		var domainError *Error
		if errors.As(probeErr, &domainError) {
			code = string(domainError.Kind)
		}
		event, eventErr := newUserAudit(actor, "node.connection_tested", "node", id, map[string]any{
			"outcome": "failed", "errorCode": code,
		}, now)
		if eventErr != nil {
			return NodeProbeResult{}, eventErr
		}
		if updateErr := s.repository.RecordNodeTestResult(ctx, id, NodeUnreachable, record.Node.CompatibilityStatus, record.Node.Version, nil, code, now, false, event); updateErr != nil {
			return NodeProbeResult{}, updateErr
		}
		return NodeProbeResult{}, probeErr
	}
	health, errorCode := healthFromProbe(result, true)
	latency := result.LatencyMS
	event, err := newUserAudit(actor, "node.connection_tested", "node", id, map[string]any{
		"outcome": "succeeded", "compatibility": result.Compatibility, "version": result.Version,
	}, now)
	if err != nil {
		return NodeProbeResult{}, err
	}
	if err := s.repository.RecordNodeTestResult(ctx, id, health, result.Compatibility, result.Version, &latency, errorCode, now, true, event); err != nil {
		return NodeProbeResult{}, err
	}
	return result, nil
}

func (s *ManagementService) DeleteNode(ctx context.Context, actor Actor, id, confirmName string, expectedVersion int) error {
	node, err := s.Node(ctx, id)
	if err != nil {
		return err
	}
	if expectedVersion < 1 {
		return Validation("version", "must be a positive integer")
	}
	if confirmName != node.Name {
		return Validation("confirmName", "must exactly match the node name")
	}
	now := s.now().UTC()
	event, err := newUserAudit(actor, "node.removed", "node", id, map[string]any{
		"clusterId": node.ClusterID, "name": node.Name, "credentialsDestroyed": true,
	}, now)
	if err != nil {
		return err
	}
	return s.repository.SoftDeleteNode(ctx, id, expectedVersion, now, event)
}

func (s *ManagementService) SetNodeMaintenance(ctx context.Context, actor Actor, id string, enabled bool, expectedVersion int) (Node, error) {
	node, err := s.Node(ctx, id)
	if err != nil {
		return Node{}, err
	}
	if expectedVersion < 1 {
		return Node{}, Validation("recordVersion", "must be a positive integer")
	}
	now := s.now().UTC()
	event, err := newUserAudit(actor, "node.maintenance_changed", "node", id, map[string]any{
		"clusterId": node.ClusterID, "enabled": enabled,
	}, now)
	if err != nil {
		return Node{}, err
	}
	if err := s.repository.SetNodeMaintenance(ctx, id, enabled, expectedVersion, now, event); err != nil {
		return Node{}, err
	}
	node.MaintenanceMode = enabled
	node.RecordVersion = expectedVersion + 1
	if enabled {
		node.ConvergenceStatus = "maintenance"
	} else {
		node.ConvergenceStatus = "pending"
	}
	node.UpdatedAt = now
	return node, nil
}

func validateNodeCredentials(username, password string) error {
	if length := len(strings.TrimSpace(username)); length < 1 || length > 256 {
		return Validation("username", "must contain between 1 and 256 characters")
	}
	if len(password) < 1 || len(password) > 4096 {
		return Validation("password", "must contain between 1 and 4096 bytes")
	}
	return nil
}

func healthFromProbe(result NodeProbeResult, enabled bool) (NodeHealth, string) {
	if !enabled {
		return NodeDisabled, ""
	}
	if !result.Running {
		return NodeUnreachable, "NODE_DNS_NOT_RUNNING"
	}
	if result.Compatibility != CompatibilitySupported {
		return NodeIncompatible, ""
	}
	return NodeHealthy, ""
}

func newUserAudit(actor Actor, action, resourceType, resourceID string, metadata map[string]any, at time.Time) (AuditEvent, error) {
	id, err := NewID()
	if err != nil {
		return AuditEvent{}, err
	}
	userID := actor.UserID
	return AuditEvent{
		ID: id, ActorType: "user", ActorUserID: &userID, Action: action,
		ResourceType: resourceType, ResourceID: &resourceID, RequestID: actor.RequestID,
		Metadata: metadata, CreatedAt: at,
	}, nil
}

package inventory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
)

type CapabilityProfile struct {
	NodeID         string          `json:"nodeId"`
	ProductVersion string          `json:"productVersion"`
	Compatibility  string          `json:"compatibility"`
	SchemaVersion  int             `json:"schemaVersion"`
	Features       map[string]bool `json:"features"`
	Warnings       []string        `json:"warnings"`
	RefreshedAt    time.Time       `json:"refreshedAt"`
}

type Snapshot struct {
	ID               string                  `json:"id"`
	NodeID           string                  `json:"nodeId"`
	ObservedAt       time.Time               `json:"observedAt"`
	SchemaVersion    int                     `json:"schemaVersion"`
	Document         *configuration.Document `json:"document,omitempty"`
	CanonicalHash    string                  `json:"canonicalHash,omitempty"`
	NodeVersion      string                  `json:"nodeVersion,omitempty"`
	CollectionStatus string                  `json:"collectionStatus"`
	ErrorCode        string                  `json:"errorCode,omitempty"`
}

type Draft struct {
	ID               string                        `json:"id"`
	ClusterID        string                        `json:"clusterId"`
	SourceSnapshotID string                        `json:"sourceSnapshotId"`
	BaseRevisionID   *string                       `json:"baseRevisionId,omitempty"`
	SchemaVersion    int                           `json:"schemaVersion"`
	Document         configuration.DesiredDocument `json:"document"`
	CanonicalHash    string                        `json:"canonicalHash"`
	Version          int                           `json:"version"`
	UpdatedBy        string                        `json:"updatedBy"`
	UpdatedAt        time.Time                     `json:"updatedAt"`
}

type Reader interface {
	ReadConfiguration(context.Context, domain.NodeProbeRequest, string) (configuration.Document, CapabilityProfile, error)
}

type Repository interface {
	domain.ManagementRepository
	SaveObservation(context.Context, Snapshot, CapabilityProfile) error
	LatestSnapshots(context.Context, string) ([]Snapshot, error)
	SnapshotByID(context.Context, string) (Snapshot, error)
	CapabilityProfiles(context.Context, string) ([]CapabilityProfile, error)
	DraftByCluster(context.Context, string) (Draft, error)
	ImportDraft(context.Context, Draft, int, domain.AuditEvent) error
}

type FilterRefresher interface {
	RefreshFilters(context.Context, domain.NodeProbeRequest, bool) error
}

type BlockedServicesCatalogueReader interface {
	ReadBlockedServicesCatalogue(context.Context, domain.NodeProbeRequest, string) (NodeBlockedServicesCatalogue, error)
}

type BlocklistReader interface {
	ReadBlocklists(context.Context, domain.NodeProbeRequest, string) ([]FilterListMetadata, error)
}

type AllowlistReader interface {
	ReadAllowlists(context.Context, domain.NodeProbeRequest, string) ([]FilterListMetadata, error)
}

type DHCPInterfaceReader interface {
	ReadDHCPInterfaces(context.Context, domain.NodeProbeRequest) ([]DHCPInterface, error)
}

type DHCPActiveChecker interface {
	FindActiveDHCP(context.Context, domain.NodeProbeRequest, string) (DHCPActiveCheck, error)
}

type CredentialProtector interface {
	Decrypt(string, domain.EncryptedCredentials) (domain.NodeCredentials, error)
}

type Service struct {
	repository    Repository
	credentials   CredentialProtector
	reader        Reader
	now           func() time.Time
	catalogueTTL  time.Duration
	catalogueMu   sync.Mutex
	catalogues    map[string]catalogueCacheEntry
	filterListTTL time.Duration
	filterListMu  sync.Mutex
	filterLists   map[string]filterListCacheEntry
}

func NewService(repository Repository, credentials CredentialProtector, reader Reader) *Service {
	return &Service{
		repository: repository, credentials: credentials, reader: reader, now: time.Now,
		catalogueTTL: 15 * time.Minute, catalogues: map[string]catalogueCacheEntry{},
		filterListTTL: 0, filterLists: map[string]filterListCacheEntry{},
	}
}

func (s *Service) Observe(ctx context.Context, nodeID string) (Snapshot, error) {
	if !domain.ValidID(nodeID) {
		return Snapshot{}, domain.Validation("nodeId", "must be a valid UUID")
	}
	record, err := s.repository.NodeRecordByID(ctx, nodeID)
	if err != nil {
		return Snapshot{}, err
	}
	if !record.Node.Enabled {
		return Snapshot{}, domain.Validation("nodeId", "disabled nodes cannot be observed")
	}
	credentials, err := s.credentials.Decrypt(nodeID, record.Secrets.Credentials)
	if err != nil {
		return Snapshot{}, errors.New("stored node credentials could not be decrypted")
	}
	id, err := domain.NewID()
	if err != nil {
		return Snapshot{}, err
	}
	now := s.now().UTC()
	request := domain.NodeProbeRequest{BaseURL: record.Node.BaseURL, CertificatePolicy: record.Node.CertificatePolicy, CustomCAPEM: record.Secrets.CustomCAPEM, Credentials: credentials}
	document, capability, readErr := s.reader.ReadConfiguration(ctx, request, record.Node.Version)
	schemaVersion := capability.SchemaVersion
	if schemaVersion == 0 {
		schemaVersion = configuration.SchemaVersion
	}
	snapshot := Snapshot{ID: id, NodeID: nodeID, ObservedAt: now, SchemaVersion: schemaVersion, NodeVersion: record.Node.Version}
	capability.NodeID, capability.RefreshedAt = nodeID, now
	if readErr != nil {
		snapshot.CollectionStatus, snapshot.ErrorCode = "failed", errorCode(readErr)
		if err := s.repository.SaveObservation(ctx, snapshot, capability); err != nil {
			return Snapshot{}, err
		}
		var domainError *domain.Error
		if errors.As(readErr, &domainError) {
			return snapshot, &domain.Error{
				Kind: domainError.Kind, Field: domainError.Field, Cause: readErr,
				Message: fmt.Sprintf("node %s (AdGuard Home %s): %s", nodeID, record.Node.Version, domainError.Message),
			}
		}
		return snapshot, fmt.Errorf("observe node %s (AdGuard Home %s): %w", nodeID, record.Node.Version, readErr)
	}
	_, hash, err := configuration.Marshal(document)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Document, snapshot.CanonicalHash, snapshot.CollectionStatus = &document, hash, "succeeded"
	if err := s.repository.SaveObservation(ctx, snapshot, capability); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (s *Service) RefreshFilters(ctx context.Context, actor domain.Actor, nodeID string, whitelist bool) error {
	if !domain.ValidID(nodeID) {
		return domain.Validation("nodeId", "must be a valid UUID")
	}
	refresher, ok := s.reader.(FilterRefresher)
	if !ok {
		return domain.NewError(domain.ErrorCapability, "filter refresh is unavailable")
	}
	audits, ok := s.repository.(interface {
		RecordAuditEvent(context.Context, domain.AuditEvent) error
	})
	if !ok {
		return domain.NewError(domain.ErrorCapability, "audited filter refresh is unavailable")
	}
	record, err := s.repository.NodeRecordByID(ctx, nodeID)
	if err != nil {
		return err
	}
	if !record.Node.Enabled || record.Node.MaintenanceMode {
		return domain.NewError(domain.ErrorConflict, "filter refresh requires an enabled node outside maintenance")
	}
	credentials, err := s.credentials.Decrypt(nodeID, record.Secrets.Credentials)
	if err != nil {
		return errors.New("stored node credentials could not be decrypted")
	}
	now := s.now().UTC()
	requested, err := inventoryAudit(actor, "filters.refresh_requested", nodeID, map[string]any{"whitelist": whitelist}, now)
	if err != nil {
		return err
	}
	if err := audits.RecordAuditEvent(ctx, requested); err != nil {
		return err
	}
	request := domain.NodeProbeRequest{BaseURL: record.Node.BaseURL, CertificatePolicy: record.Node.CertificatePolicy, CustomCAPEM: record.Secrets.CustomCAPEM, Credentials: credentials}
	refreshErr := refresher.RefreshFilters(ctx, request, whitelist)
	action := "filters.refresh_succeeded"
	if refreshErr != nil {
		action = "filters.refresh_failed"
	}
	code := ""
	if refreshErr != nil {
		code = errorCode(refreshErr)
	}
	completed, eventErr := inventoryAudit(actor, action, nodeID, map[string]any{"whitelist": whitelist, "errorCode": code}, s.now().UTC())
	if eventErr != nil {
		return eventErr
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := audits.RecordAuditEvent(auditCtx, completed); err != nil {
		return err
	}
	return refreshErr
}

func inventoryAudit(actor domain.Actor, action, resourceID string, metadata map[string]any, at time.Time) (domain.AuditEvent, error) {
	id, err := domain.NewID()
	if err != nil {
		return domain.AuditEvent{}, err
	}
	userID := actor.UserID
	return domain.AuditEvent{ID: id, ActorType: "user", ActorUserID: &userID, Action: action, ResourceType: "node", ResourceID: &resourceID, RequestID: actor.RequestID, Metadata: metadata, CreatedAt: at}, nil
}

func (s *Service) Inventory(ctx context.Context, clusterID string) ([]Snapshot, []CapabilityProfile, *Draft, error) {
	if !domain.ValidID(clusterID) {
		return nil, nil, nil, domain.Validation("clusterId", "must be a valid UUID")
	}
	if _, err := s.repository.ClusterByID(ctx, clusterID); err != nil {
		return nil, nil, nil, err
	}
	snapshots, err := s.repository.LatestSnapshots(ctx, clusterID)
	if err != nil {
		return nil, nil, nil, err
	}
	profiles, err := s.repository.CapabilityProfiles(ctx, clusterID)
	if err != nil {
		return nil, nil, nil, err
	}
	draft, err := s.repository.DraftByCluster(ctx, clusterID)
	if err != nil {
		var de *domain.Error
		if !errors.As(err, &de) || de.Kind != domain.ErrorNotFound {
			return nil, nil, nil, err
		}
		return snapshots, profiles, nil, nil
	}
	return snapshots, profiles, &draft, nil
}

func (s *Service) Compare(ctx context.Context, leftID, rightID string) ([]configuration.Difference, error) {
	left, err := s.repository.SnapshotByID(ctx, leftID)
	if err != nil {
		return nil, err
	}
	right, err := s.repository.SnapshotByID(ctx, rightID)
	if err != nil {
		return nil, err
	}
	if left.Document == nil || right.Document == nil {
		return nil, domain.Validation("snapshot", "only successful snapshots can be compared")
	}
	return configuration.Diff(*left.Document, *right.Document), nil
}

func (s *Service) Import(ctx context.Context, actor domain.Actor, clusterID, snapshotID string, expectedVersion int, confirmed bool) (Draft, error) {
	if !confirmed {
		return Draft{}, domain.Validation("confirmed", "must be true after reviewing the snapshot")
	}
	snapshot, err := s.repository.SnapshotByID(ctx, snapshotID)
	if err != nil {
		return Draft{}, err
	}
	node, err := s.repository.NodeByID(ctx, snapshot.NodeID)
	if err != nil {
		return Draft{}, err
	}
	if node.ClusterID != clusterID {
		return Draft{}, domain.Validation("snapshotId", "snapshot does not belong to the cluster")
	}
	if snapshot.Document == nil {
		return Draft{}, domain.Validation("snapshotId", "failed snapshots cannot be imported")
	}
	if issues := configuration.ValidateNodeSpecific("nodeSpecific", snapshot.Document.NodeSpecific); len(issues) > 0 {
		return Draft{}, domain.Validation("snapshotId", "listener identity is incomplete; refresh the node and import its latest successful snapshot")
	}
	id, err := domain.NewID()
	if err != nil {
		return Draft{}, err
	}
	desired := configuration.DesiredFromObservation(snapshot.NodeID, *snapshot.Document)
	var baseRevisionID *string
	if current, currentErr := s.repository.DraftByCluster(ctx, clusterID); currentErr == nil {
		id = current.ID
		if expectedVersion != current.Version {
			return Draft{}, domain.NewError(domain.ErrorConflict, "the configuration draft was changed by another request")
		}
		if current.Document.SchemaVersion > desired.SchemaVersion {
			return Draft{}, domain.Validation("snapshotId", "an older schema observation cannot replace a newer configuration draft")
		}
		for existingNodeID, override := range current.Document.NodeOverrides {
			desired.NodeOverrides[existingNodeID] = override
		}
		desired.NodeOverrides[snapshot.NodeID] = snapshot.Document.NodeSpecific
		baseRevisionID = current.BaseRevisionID
	} else if expectedVersion != 0 {
		return Draft{}, domain.NewError(domain.ErrorConflict, "the configuration draft does not exist")
	}
	now := s.now().UTC()
	_, desiredHash, err := configuration.MarshalDesired(desired)
	if err != nil {
		return Draft{}, err
	}
	draft := Draft{ID: id, ClusterID: clusterID, SourceSnapshotID: snapshotID, BaseRevisionID: baseRevisionID, SchemaVersion: desired.SchemaVersion, Document: desired, CanonicalHash: desiredHash, Version: expectedVersion + 1, UpdatedBy: actor.UserID, UpdatedAt: now}
	eventID, err := domain.NewID()
	if err != nil {
		return Draft{}, err
	}
	resourceID := id
	userID := actor.UserID
	event := domain.AuditEvent{ID: eventID, ActorType: "user", ActorUserID: &userID, Action: "configuration.draft_imported", ResourceType: "configuration_draft", ResourceID: &resourceID, RequestID: actor.RequestID, Metadata: map[string]any{"clusterId": clusterID, "sourceSnapshotId": snapshotID}, CreatedAt: now}
	if err := s.repository.ImportDraft(ctx, draft, expectedVersion, event); err != nil {
		return Draft{}, err
	}
	return draft, nil
}

func errorCode(err error) string {
	var de *domain.Error
	if errors.As(err, &de) {
		return string(de.Kind)
	}
	return string(domain.ErrorInternal)
}

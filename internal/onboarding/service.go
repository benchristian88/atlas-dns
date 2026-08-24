package onboarding

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/benchristian88/atlas-dns/internal/adguard"
	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/controlplane"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
	"github.com/benchristian88/atlas-dns/internal/inventory"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

type Step string

const (
	StepWelcome       Step = "welcome"
	StepIdentity      Step = "controller_identity"
	StepPrimaryNode   Step = "primary_node"
	StepSecondaryNode Step = "secondary_node"
	StepTopology      Step = "topology_validation"
	StepBaseline      Step = "baseline_selection"
	StepMonitoring    Step = "monitoring"
	StepNotifications Step = "notifications"
	StepReview        Step = "review"
	StepComplete      Step = "completed"
)

type State struct {
	ClusterID              string     `json:"clusterId"`
	RedundancySkippedAt    *time.Time `json:"redundancySkippedAt,omitempty"`
	MonitoringReviewedAt   *time.Time `json:"monitoringReviewedAt,omitempty"`
	NotificationsSkippedAt *time.Time `json:"notificationsSkippedAt,omitempty"`
	CompletedAt            *time.Time `json:"completedAt,omitempty"`
	CompletedBy            *string    `json:"completedBy,omitempty"`
	RecordVersion          int        `json:"recordVersion"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

type Repository interface {
	ListClusters(context.Context) ([]domain.Cluster, error)
	ClusterByID(context.Context, string) (domain.Cluster, error)
	ListNodes(context.Context, string) ([]domain.Node, error)
	LatestSnapshots(context.Context, string) ([]inventory.Snapshot, error)
	CapabilityProfiles(context.Context, string) ([]inventory.CapabilityProfile, error)
	ListRevisions(context.Context, string, bool) ([]controlplane.Revision, error)
	ListNotificationChannels(context.Context, string) ([]haoperations.NotificationChannel, error)
	OnboardingState(context.Context, string) (State, error)
	SaveOnboardingState(context.Context, State, int, domain.AuditEvent) error
}

type SettingsReader interface {
	Get(context.Context) (systemsettings.Settings, error)
}

type Status struct {
	SetupRequired      bool                    `json:"setupRequired"`
	Completed          bool                    `json:"completed"`
	ResumeStep         Step                    `json:"resumeStep"`
	PublicBaseURL      string                  `json:"publicBaseUrl"`
	Cluster            *domain.Cluster         `json:"cluster,omitempty"`
	State              State                   `json:"state"`
	Nodes              []NodeStatus            `json:"nodes"`
	NodeCount          int                     `json:"nodeCount"`
	EligibleNodeCount  int                     `json:"eligibleNodeCount"`
	Redundant          bool                    `json:"redundant"`
	TopologyReady      bool                    `json:"topologyReady"`
	AuthoritativeReady bool                    `json:"authoritativeReady"`
	Revision           *controlplane.Revision  `json:"revision,omitempty"`
	Monitoring         systemsettings.Settings `json:"monitoring"`
	NotificationCount  int                     `json:"notificationCount"`
	NotificationsReady bool                    `json:"notificationsReady"`
	CanFinish          bool                    `json:"canFinish"`
}

type NodeStatus struct {
	Node                   domain.Node                  `json:"node"`
	OnboardingCompatible   bool                         `json:"onboardingCompatible"`
	ConfigurationAvailable bool                         `json:"configurationAvailable"`
	Capability             *inventory.CapabilityProfile `json:"capability,omitempty"`
	Snapshot               *inventory.Snapshot          `json:"snapshot,omitempty"`
}

type Service struct {
	repository    Repository
	settings      SettingsReader
	publicBaseURL string
	now           func() time.Time
}

func NewService(repository Repository, settings SettingsReader, publicBaseURL string) *Service {
	return &Service{repository: repository, settings: settings, publicBaseURL: publicBaseURL, now: time.Now}
}

func (s *Service) Status(ctx context.Context, clusterID string) (Status, error) {
	result := Status{SetupRequired: true, ResumeStep: StepIdentity, PublicBaseURL: s.publicBaseURL, Nodes: []NodeStatus{}}
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("read onboarding monitoring settings: %w", err)
	}
	result.Monitoring = settings
	if strings.TrimSpace(clusterID) == "" {
		clusters, listErr := s.repository.ListClusters(ctx)
		if listErr != nil {
			return Status{}, listErr
		}
		if len(clusters) == 0 {
			return result, nil
		}
		clusterID = clusters[0].ID
	}
	if !domain.ValidID(clusterID) {
		return Status{}, domain.Validation("clusterId", "must be a valid UUID")
	}
	cluster, err := s.repository.ClusterByID(ctx, clusterID)
	if err != nil {
		return Status{}, err
	}
	result.Cluster, result.ResumeStep = &cluster, StepWelcome
	state, err := s.repository.OnboardingState(ctx, clusterID)
	if err != nil {
		var de *domain.Error
		if !errors.As(err, &de) || de.Kind != domain.ErrorNotFound {
			return Status{}, err
		}
		state = State{ClusterID: clusterID, RecordVersion: 0}
	}
	result.State = state
	nodes, err := s.repository.ListNodes(ctx, clusterID)
	if err != nil {
		return Status{}, err
	}
	snapshots, err := s.repository.LatestSnapshots(ctx, clusterID)
	if err != nil {
		return Status{}, err
	}
	profiles, err := s.repository.CapabilityProfiles(ctx, clusterID)
	if err != nil {
		return Status{}, err
	}
	snapshotByNode := map[string]inventory.Snapshot{}
	for _, snapshot := range snapshots {
		snapshotByNode[snapshot.NodeID] = snapshot
	}
	profileByNode := map[string]inventory.CapabilityProfile{}
	for _, profile := range profiles {
		profileByNode[profile.NodeID] = profile
	}
	allConfigurationAvailable := true
	for _, node := range nodes {
		if !node.Enabled {
			continue
		}
		result.NodeCount++
		item := NodeStatus{Node: node}
		item.OnboardingCompatible = adguard.OnboardingCompatibility(node.Version) == domain.CompatibilitySupported && node.HealthStatus == domain.NodeHealthy
		if state.CompletedAt != nil && node.CompatibilityStatus == domain.CompatibilitySupported && node.HealthStatus == domain.NodeHealthy {
			// Preserve established supported clusters across the v1.1 upgrade;
			// the stricter floor applies to newly guided onboarding.
			item.OnboardingCompatible = true
		}
		if snapshot, ok := snapshotByNode[node.ID]; ok {
			item.Snapshot = &snapshot
			item.ConfigurationAvailable = snapshot.CollectionStatus == "succeeded" && snapshot.Document != nil && snapshot.SchemaVersion == configuration.SchemaVersion
		}
		if profile, ok := profileByNode[node.ID]; ok {
			item.Capability = &profile
			item.ConfigurationAvailable = item.ConfigurationAvailable && profile.Compatibility == string(domain.CompatibilitySupported) && profile.SchemaVersion == configuration.SchemaVersion
		} else {
			item.ConfigurationAvailable = false
		}
		if item.OnboardingCompatible {
			result.EligibleNodeCount++
		}
		if !item.OnboardingCompatible || !item.ConfigurationAvailable {
			allConfigurationAvailable = false
		}
		result.Nodes = append(result.Nodes, item)
	}
	result.Redundant = result.EligibleNodeCount >= 2
	result.TopologyReady = result.EligibleNodeCount >= 1 && allConfigurationAvailable
	revisions, err := s.repository.ListRevisions(ctx, clusterID, false)
	if err != nil {
		return Status{}, err
	}
	for index := range revisions {
		if revisions[index].SchemaVersion == configuration.SchemaVersion {
			result.Revision = &revisions[index]
			result.AuthoritativeReady = true
			break
		}
	}
	channels, err := s.repository.ListNotificationChannels(ctx, clusterID)
	if err != nil {
		return Status{}, err
	}
	result.NotificationCount = len(channels)
	result.NotificationsReady = len(channels) > 0 || state.NotificationsSkippedAt != nil
	result.CanFinish = result.TopologyReady && result.AuthoritativeReady && (result.Redundant || state.RedundancySkippedAt != nil) && state.MonitoringReviewedAt != nil && result.NotificationsReady
	// Completion is an explicit, audited lifecycle event.  Once recorded it is
	// not revoked by a transient node outage or a later operator change; the
	// current topology facts above remain visible for review and remediation.
	result.Completed = state.CompletedAt != nil
	result.SetupRequired = !result.Completed
	result.ResumeStep = resumeStep(result)
	return result, nil
}

func resumeStep(status Status) Step {
	if status.Completed {
		return StepComplete
	}
	if status.Cluster == nil {
		return StepIdentity
	}
	if status.EligibleNodeCount == 0 {
		return StepPrimaryNode
	}
	if !status.Redundant && status.State.RedundancySkippedAt == nil {
		return StepSecondaryNode
	}
	if !status.TopologyReady {
		return StepTopology
	}
	if !status.AuthoritativeReady {
		return StepBaseline
	}
	if status.State.MonitoringReviewedAt == nil {
		return StepMonitoring
	}
	if !status.NotificationsReady {
		return StepNotifications
	}
	return StepReview
}

type ProgressInput struct {
	RedundancySkipped    *bool `json:"redundancySkipped,omitempty"`
	MonitoringReviewed   *bool `json:"monitoringReviewed,omitempty"`
	NotificationsSkipped *bool `json:"notificationsSkipped,omitempty"`
}

func (s *Service) Progress(ctx context.Context, actor domain.Actor, clusterID string, expectedVersion int, input ProgressInput) (Status, error) {
	status, err := s.Status(ctx, clusterID)
	if err != nil {
		return Status{}, err
	}
	state := status.State
	if expectedVersion != state.RecordVersion {
		return Status{}, domain.NewError(domain.ErrorConflict, "onboarding state changed; reload and try again")
	}
	now := s.now().UTC()
	setTime := func(value *bool, target **time.Time) {
		if value == nil {
			return
		}
		if *value {
			*target = &now
		} else {
			*target = nil
		}
	}
	setTime(input.RedundancySkipped, &state.RedundancySkippedAt)
	setTime(input.MonitoringReviewed, &state.MonitoringReviewedAt)
	setTime(input.NotificationsSkipped, &state.NotificationsSkippedAt)
	state.ClusterID, state.RecordVersion, state.UpdatedAt = clusterID, expectedVersion+1, now
	event, err := onboardingAudit(actor, "onboarding.progress_updated", clusterID, map[string]any{
		"redundancySkipped": state.RedundancySkippedAt != nil, "monitoringReviewed": state.MonitoringReviewedAt != nil, "notificationsSkipped": state.NotificationsSkippedAt != nil,
	}, now)
	if err != nil {
		return Status{}, err
	}
	if err := s.repository.SaveOnboardingState(ctx, state, expectedVersion, event); err != nil {
		return Status{}, err
	}
	return s.Status(ctx, clusterID)
}

func (s *Service) Finish(ctx context.Context, actor domain.Actor, clusterID string, expectedVersion int) (Status, error) {
	status, err := s.Status(ctx, clusterID)
	if err != nil {
		return Status{}, err
	}
	if expectedVersion != status.State.RecordVersion {
		return Status{}, domain.NewError(domain.ErrorConflict, "onboarding state changed; reload and try again")
	}
	if !status.CanFinish {
		return Status{}, domain.NewError(domain.ErrorConflict, "onboarding prerequisites are incomplete")
	}
	now := s.now().UTC()
	state := status.State
	state.CompletedAt, state.CompletedBy, state.UpdatedAt = &now, &actor.UserID, now
	state.RecordVersion = expectedVersion + 1
	event, err := onboardingAudit(actor, "onboarding.completed", clusterID, map[string]any{"eligibleNodeCount": status.EligibleNodeCount, "revisionId": status.Revision.ID}, now)
	if err != nil {
		return Status{}, err
	}
	if err := s.repository.SaveOnboardingState(ctx, state, expectedVersion, event); err != nil {
		return Status{}, err
	}
	return s.Status(ctx, clusterID)
}

func onboardingAudit(actor domain.Actor, action, clusterID string, metadata map[string]any, at time.Time) (domain.AuditEvent, error) {
	id, err := domain.NewID()
	if err != nil {
		return domain.AuditEvent{}, err
	}
	actorID := actor.UserID
	return domain.AuditEvent{ID: id, ActorType: "user", ActorUserID: &actorID, Action: action, ResourceType: "cluster", ResourceID: &clusterID, RequestID: actor.RequestID, Metadata: metadata, CreatedAt: at}, nil
}

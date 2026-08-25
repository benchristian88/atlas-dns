package systemsettings

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

type RuntimeSettings struct {
	SessionDuration             time.Duration
	NodeHealthInterval          time.Duration
	NodeRequestTimeout          time.Duration
	StatisticsPollInterval      time.Duration
	QueryLogCollection          bool
	QueryLogPollInterval        time.Duration
	QueryLogRetention           time.Duration
	LogLevel                    string
	OperationalHistoryRetention time.Duration
}

func Recommended() RuntimeSettings {
	return RuntimeSettings{
		SessionDuration: 12 * time.Hour, NodeHealthInterval: 30 * time.Second, NodeRequestTimeout: 10 * time.Second,
		StatisticsPollInterval: time.Hour, QueryLogCollection: true, QueryLogPollInterval: 30 * time.Second,
		QueryLogRetention: 7 * 24 * time.Hour, LogLevel: "info", OperationalHistoryRetention: 90 * 24 * time.Hour,
	}
}

type Settings struct {
	UpdateChecksEnabled             bool   `json:"updateChecksEnabled"`
	RecordVersion                   int    `json:"recordVersion"`
	SessionDurationSeconds          int64  `json:"sessionDurationSeconds"`
	NodeHealthIntervalSeconds       int64  `json:"nodeHealthIntervalSeconds"`
	NodeRequestTimeoutSeconds       int64  `json:"nodeRequestTimeoutSeconds"`
	StatisticsPollIntervalSeconds   int64  `json:"statisticsPollIntervalSeconds"`
	QueryLogCollectionEnabled       bool   `json:"queryLogCollectionEnabled"`
	QueryLogPollIntervalSeconds     int64  `json:"queryLogPollIntervalSeconds"`
	QueryLogRetentionSeconds        int64  `json:"queryLogRetentionSeconds"`
	QueryLogRetention               string `json:"queryLogRetention"`
	LogLevel                        string `json:"logLevel"`
	OperationalHistoryRetentionDays int    `json:"operationalHistoryRetentionDays"`
	StatisticsRetention             string `json:"statisticsRetention"`
	InstallationType                string `json:"installationType"`
}

type StoredSettings struct {
	UpdateChecksEnabled bool
	RuntimeInitialized  bool
	Runtime             RuntimeSettings
	RecordVersion       int
}

type Repository interface {
	SystemSettings(context.Context) (StoredSettings, error)
	InitializeRuntimeSettings(context.Context, RuntimeSettings) (StoredSettings, error)
	UpdateSystemSettings(context.Context, StoredSettings, int, time.Time, domain.AuditEvent) (StoredSettings, error)
	ClearOperationalHistory(context.Context, domain.AuditEvent) (ClearOperationalHistoryResult, error)
}

const ClearOperationalHistoryConfirmation = "CLEAR OPERATIONAL HISTORY"

type ClearOperationalHistoryResult struct {
	EventsDeleted     int64 `json:"eventsDeleted"`
	DeliveriesDeleted int64 `json:"deliveriesDeleted"`
}

type RuntimeStore struct {
	mu      sync.RWMutex
	value   RuntimeSettings
	changed chan struct{}
}

func NewRuntimeStore(value RuntimeSettings) *RuntimeStore {
	return &RuntimeStore{value: value, changed: make(chan struct{})}
}
func (s *RuntimeStore) RuntimeSettings() RuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}
func (s *RuntimeStore) Update(value RuntimeSettings) {
	s.mu.Lock()
	if s.value == value {
		s.mu.Unlock()
		return
	}
	s.value = value
	close(s.changed)
	s.changed = make(chan struct{})
	s.mu.Unlock()
}

func (s *RuntimeStore) RuntimeSettingsChanged() <-chan struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.changed
}

type Service struct {
	repository       Repository
	fallback         RuntimeSettings
	installationType string
	runtime          *RuntimeStore
	now              func() time.Time
}

func NewService(repository Repository, fallback RuntimeSettings, installationType string, stores ...*RuntimeStore) *Service {
	runtime := NewRuntimeStore(fallback)
	if len(stores) > 0 && stores[0] != nil {
		runtime = stores[0]
	}
	return &Service{repository: repository, fallback: fallback, installationType: installationType, runtime: runtime, now: time.Now}
}

func (s *Service) Initialize(ctx context.Context) error {
	stored, err := s.repository.InitializeRuntimeSettings(ctx, s.fallback)
	if err != nil {
		return fmt.Errorf("initialize runtime settings: %w", err)
	}
	if err := validateRuntime(stored.Runtime); err != nil {
		return fmt.Errorf("load runtime settings: %w", err)
	}
	s.runtime.Update(stored.Runtime)
	return nil
}

func (s *Service) Get(ctx context.Context) (Settings, error) {
	stored, err := s.repository.SystemSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	if !stored.RuntimeInitialized {
		stored, err = s.repository.InitializeRuntimeSettings(ctx, s.fallback)
		if err != nil {
			return Settings{}, err
		}
	}
	if err := validateRuntime(stored.Runtime); err != nil {
		return Settings{}, err
	}
	s.runtime.Update(stored.Runtime)
	return present(stored, s.installationType), nil
}

func (s *Service) Update(ctx context.Context, actor domain.Actor, input Settings, expectedVersion int) (Settings, error) {
	runtime := RuntimeSettings{
		SessionDuration:             time.Duration(input.SessionDurationSeconds) * time.Second,
		NodeHealthInterval:          time.Duration(input.NodeHealthIntervalSeconds) * time.Second,
		NodeRequestTimeout:          time.Duration(input.NodeRequestTimeoutSeconds) * time.Second,
		StatisticsPollInterval:      time.Duration(input.StatisticsPollIntervalSeconds) * time.Second,
		QueryLogCollection:          input.QueryLogCollectionEnabled,
		QueryLogPollInterval:        time.Duration(input.QueryLogPollIntervalSeconds) * time.Second,
		QueryLogRetention:           time.Duration(input.QueryLogRetentionSeconds) * time.Second,
		LogLevel:                    input.LogLevel,
		OperationalHistoryRetention: time.Duration(input.OperationalHistoryRetentionDays) * 24 * time.Hour,
	}
	if err := validateRuntime(runtime); err != nil {
		return Settings{}, err
	}
	now := s.now().UTC()
	id, err := domain.NewID()
	if err != nil {
		return Settings{}, err
	}
	actorID := actor.UserID
	event := domain.AuditEvent{ID: id, ActorType: "user", ActorUserID: &actorID, Action: "system_settings.updated", ResourceType: "system_settings", RequestID: actor.RequestID, Metadata: map[string]any{
		"updateChecksEnabled":             input.UpdateChecksEnabled,
		"sessionDurationSeconds":          input.SessionDurationSeconds,
		"nodeHealthIntervalSeconds":       input.NodeHealthIntervalSeconds,
		"nodeRequestTimeoutSeconds":       input.NodeRequestTimeoutSeconds,
		"statisticsPollIntervalSeconds":   input.StatisticsPollIntervalSeconds,
		"queryLogCollectionEnabled":       input.QueryLogCollectionEnabled,
		"queryLogPollIntervalSeconds":     input.QueryLogPollIntervalSeconds,
		"queryLogRetentionSeconds":        input.QueryLogRetentionSeconds,
		"logLevel":                        input.LogLevel,
		"operationalHistoryRetentionDays": input.OperationalHistoryRetentionDays,
	}, CreatedAt: now}
	stored, err := s.repository.UpdateSystemSettings(ctx, StoredSettings{UpdateChecksEnabled: input.UpdateChecksEnabled, RuntimeInitialized: true, Runtime: runtime}, expectedVersion, now, event)
	if err != nil {
		return Settings{}, fmt.Errorf("update system settings: %w", err)
	}
	s.runtime.Update(stored.Runtime)
	return present(stored, s.installationType), nil
}

func (s *Service) ClearOperationalHistory(ctx context.Context, actor domain.Actor, confirmation string) (ClearOperationalHistoryResult, error) {
	if confirmation != ClearOperationalHistoryConfirmation {
		return ClearOperationalHistoryResult{}, domain.Validation("confirmation", "must exactly confirm clearing Operational History")
	}
	id, err := domain.NewID()
	if err != nil {
		return ClearOperationalHistoryResult{}, err
	}
	actorID := actor.UserID
	event := domain.AuditEvent{ID: id, ActorType: "user", ActorUserID: &actorID, Action: "operational_history.cleared",
		ResourceType: "operational_history", RequestID: actor.RequestID, Metadata: map[string]any{}, CreatedAt: s.now().UTC()}
	result, err := s.repository.ClearOperationalHistory(ctx, event)
	if err != nil {
		return ClearOperationalHistoryResult{}, fmt.Errorf("clear operational history: %w", err)
	}
	return result, nil
}

func present(value StoredSettings, installationType string) Settings {
	return Settings{
		UpdateChecksEnabled: value.UpdateChecksEnabled, RecordVersion: value.RecordVersion,
		SessionDurationSeconds:        int64(value.Runtime.SessionDuration / time.Second),
		NodeHealthIntervalSeconds:     int64(value.Runtime.NodeHealthInterval / time.Second),
		NodeRequestTimeoutSeconds:     int64(value.Runtime.NodeRequestTimeout / time.Second),
		StatisticsPollIntervalSeconds: int64(value.Runtime.StatisticsPollInterval / time.Second),
		QueryLogCollectionEnabled:     value.Runtime.QueryLogCollection,
		QueryLogPollIntervalSeconds:   int64(value.Runtime.QueryLogPollInterval / time.Second),
		QueryLogRetentionSeconds:      int64(value.Runtime.QueryLogRetention / time.Second),
		QueryLogRetention:             value.Runtime.QueryLogRetention.String(), LogLevel: value.Runtime.LogLevel,
		OperationalHistoryRetentionDays: int(value.Runtime.OperationalHistoryRetention / (24 * time.Hour)),
		StatisticsRetention:             "32 days detailed; 400 days daily", InstallationType: installationType,
	}
}

func validateRuntime(value RuntimeSettings) error {
	switch {
	case value.SessionDuration < 15*time.Minute || value.SessionDuration > 30*24*time.Hour:
		return domain.Validation("sessionDurationSeconds", "must be between 900 and 2592000 seconds")
	case value.NodeHealthInterval < 5*time.Second || value.NodeHealthInterval > time.Hour:
		return domain.Validation("nodeHealthIntervalSeconds", "must be between 5 and 3600 seconds")
	case value.NodeRequestTimeout < time.Second || value.NodeRequestTimeout > 2*time.Minute:
		return domain.Validation("nodeRequestTimeoutSeconds", "must be between 1 and 120 seconds")
	case value.StatisticsPollInterval < time.Minute || value.StatisticsPollInterval > 24*time.Hour:
		return domain.Validation("statisticsPollIntervalSeconds", "must be between 60 and 86400 seconds")
	case value.QueryLogPollInterval < 5*time.Second || value.QueryLogPollInterval > time.Hour:
		return domain.Validation("queryLogPollIntervalSeconds", "must be between 5 and 3600 seconds")
	case value.QueryLogRetention < time.Hour || value.QueryLogRetention > 90*24*time.Hour:
		return domain.Validation("queryLogRetentionSeconds", "must be between 3600 and 7776000 seconds")
	case value.LogLevel != "debug" && value.LogLevel != "info" && value.LogLevel != "warn" && value.LogLevel != "error":
		return domain.Validation("logLevel", "must be debug, info, warn, or error")
	case !validOperationalHistoryRetention(value.OperationalHistoryRetention):
		return domain.Validation("operationalHistoryRetentionDays", "must be 7, 14, 30, 90, 180, or 365 days")
	default:
		return nil
	}
}

func validOperationalHistoryRetention(value time.Duration) bool {
	for _, days := range []time.Duration{7, 14, 30, 90, 180, 365} {
		if value == days*24*time.Hour {
			return true
		}
	}
	return false
}

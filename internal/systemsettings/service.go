package systemsettings

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

type RuntimeSettings struct {
	NodeHealthInterval     time.Duration
	StatisticsPollInterval time.Duration
	QueryLogCollection     bool
	QueryLogPollInterval   time.Duration
	QueryLogRetention      time.Duration
}

func Recommended() RuntimeSettings {
	return RuntimeSettings{
		NodeHealthInterval: 30 * time.Second, StatisticsPollInterval: time.Hour,
		QueryLogCollection: true, QueryLogPollInterval: 30 * time.Second, QueryLogRetention: 7 * 24 * time.Hour,
	}
}

type Settings struct {
	UpdateChecksEnabled           bool   `json:"updateChecksEnabled"`
	RecordVersion                 int    `json:"recordVersion"`
	NodeHealthIntervalSeconds     int64  `json:"nodeHealthIntervalSeconds"`
	StatisticsPollIntervalSeconds int64  `json:"statisticsPollIntervalSeconds"`
	QueryLogCollectionEnabled     bool   `json:"queryLogCollectionEnabled"`
	QueryLogPollIntervalSeconds   int64  `json:"queryLogPollIntervalSeconds"`
	QueryLogRetentionSeconds      int64  `json:"queryLogRetentionSeconds"`
	QueryLogRetention             string `json:"queryLogRetention"`
	StatisticsRetention           string `json:"statisticsRetention"`
	InstallationType              string `json:"installationType"`
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
}

type RuntimeStore struct {
	mu    sync.RWMutex
	value RuntimeSettings
}

func NewRuntimeStore(value RuntimeSettings) *RuntimeStore { return &RuntimeStore{value: value} }
func (s *RuntimeStore) RuntimeSettings() RuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}
func (s *RuntimeStore) Update(value RuntimeSettings) {
	s.mu.Lock()
	s.value = value
	s.mu.Unlock()
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
		NodeHealthInterval:     time.Duration(input.NodeHealthIntervalSeconds) * time.Second,
		StatisticsPollInterval: time.Duration(input.StatisticsPollIntervalSeconds) * time.Second,
		QueryLogCollection:     input.QueryLogCollectionEnabled,
		QueryLogPollInterval:   time.Duration(input.QueryLogPollIntervalSeconds) * time.Second,
		QueryLogRetention:      time.Duration(input.QueryLogRetentionSeconds) * time.Second,
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
		"updateChecksEnabled":           input.UpdateChecksEnabled,
		"nodeHealthIntervalSeconds":     input.NodeHealthIntervalSeconds,
		"statisticsPollIntervalSeconds": input.StatisticsPollIntervalSeconds,
		"queryLogCollectionEnabled":     input.QueryLogCollectionEnabled,
		"queryLogPollIntervalSeconds":   input.QueryLogPollIntervalSeconds,
		"queryLogRetentionSeconds":      input.QueryLogRetentionSeconds,
	}, CreatedAt: now}
	stored, err := s.repository.UpdateSystemSettings(ctx, StoredSettings{UpdateChecksEnabled: input.UpdateChecksEnabled, RuntimeInitialized: true, Runtime: runtime}, expectedVersion, now, event)
	if err != nil {
		return Settings{}, fmt.Errorf("update system settings: %w", err)
	}
	s.runtime.Update(stored.Runtime)
	return present(stored, s.installationType), nil
}

func present(value StoredSettings, installationType string) Settings {
	return Settings{
		UpdateChecksEnabled: value.UpdateChecksEnabled, RecordVersion: value.RecordVersion,
		NodeHealthIntervalSeconds:     int64(value.Runtime.NodeHealthInterval / time.Second),
		StatisticsPollIntervalSeconds: int64(value.Runtime.StatisticsPollInterval / time.Second),
		QueryLogCollectionEnabled:     value.Runtime.QueryLogCollection,
		QueryLogPollIntervalSeconds:   int64(value.Runtime.QueryLogPollInterval / time.Second),
		QueryLogRetentionSeconds:      int64(value.Runtime.QueryLogRetention / time.Second),
		QueryLogRetention:             value.Runtime.QueryLogRetention.String(), StatisticsRetention: "32 days detailed; 400 days daily", InstallationType: installationType,
	}
}

func validateRuntime(value RuntimeSettings) error {
	switch {
	case value.NodeHealthInterval < 5*time.Second || value.NodeHealthInterval > time.Hour:
		return domain.Validation("nodeHealthIntervalSeconds", "must be between 5 and 3600 seconds")
	case value.StatisticsPollInterval < time.Minute || value.StatisticsPollInterval > 24*time.Hour:
		return domain.Validation("statisticsPollIntervalSeconds", "must be between 60 and 86400 seconds")
	case value.QueryLogPollInterval < 5*time.Second || value.QueryLogPollInterval > time.Hour:
		return domain.Validation("queryLogPollIntervalSeconds", "must be between 5 and 3600 seconds")
	case value.QueryLogRetention < time.Hour || value.QueryLogRetention > 90*24*time.Hour:
		return domain.Validation("queryLogRetentionSeconds", "must be between 3600 and 7776000 seconds")
	default:
		return nil
	}
}

package systemsettings

import (
	"context"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

type repositoryStub struct {
	stored StoredSettings
	event  domain.AuditEvent
}

func (r *repositoryStub) SystemSettings(context.Context) (StoredSettings, error) {
	return r.stored, nil
}
func (r *repositoryStub) InitializeRuntimeSettings(_ context.Context, fallback RuntimeSettings) (StoredSettings, error) {
	if !r.stored.RuntimeInitialized {
		r.stored.RuntimeInitialized = true
		r.stored.Runtime = fallback
	}
	return r.stored, nil
}
func (r *repositoryStub) UpdateSystemSettings(_ context.Context, value StoredSettings, expectedVersion int, _ time.Time, event domain.AuditEvent) (StoredSettings, error) {
	value.RecordVersion = expectedVersion + 1
	r.stored = value
	r.event = event
	return value, nil
}

func TestGetReturnsPersistedSettingAndReadOnlyRuntimeFacts(t *testing.T) {
	runtime := Recommended()
	repository := &repositoryStub{stored: StoredSettings{UpdateChecksEnabled: true, RuntimeInitialized: true, Runtime: runtime, RecordVersion: 3}}
	store := NewRuntimeStore(RuntimeSettings{})
	service := NewService(repository, Recommended(), "docker", store)
	settings, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !settings.UpdateChecksEnabled || settings.RecordVersion != 3 || settings.QueryLogRetention != "168h0m0s" || settings.InstallationType != "docker" {
		t.Fatalf("unexpected settings: %#v", settings)
	}
	if store.RuntimeSettings() != runtime {
		t.Fatalf("runtime store = %#v, want %#v", store.RuntimeSettings(), runtime)
	}
}

func TestGetInitializesRuntimeFromDeploymentFallbackOnce(t *testing.T) {
	fallback := RuntimeSettings{NodeHealthInterval: 45 * time.Second, StatisticsPollInterval: 2 * time.Hour, QueryLogCollection: false, QueryLogPollInterval: time.Minute, QueryLogRetention: 24 * time.Hour}
	repository := &repositoryStub{stored: StoredSettings{UpdateChecksEnabled: true, RecordVersion: 2}}
	service := NewService(repository, fallback, "native_systemd")
	settings, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.NodeHealthIntervalSeconds != 45 || settings.StatisticsPollIntervalSeconds != 7200 || settings.QueryLogCollectionEnabled {
		t.Fatalf("unexpected initialized settings: %#v", settings)
	}
}

func TestUpdateUsesExpectedVersionAuditsAndPublishesRuntime(t *testing.T) {
	repository := &repositoryStub{stored: StoredSettings{UpdateChecksEnabled: true, RuntimeInitialized: true, Runtime: Recommended(), RecordVersion: 3}}
	store := NewRuntimeStore(Recommended())
	service := NewService(repository, Recommended(), "native_systemd", store)
	service.now = func() time.Time { return time.Unix(1, 0).UTC() }
	input := Settings{UpdateChecksEnabled: false, NodeHealthIntervalSeconds: 10, StatisticsPollIntervalSeconds: 300, QueryLogCollectionEnabled: false, QueryLogPollIntervalSeconds: 15, QueryLogRetentionSeconds: 86400}
	settings, err := service.Update(context.Background(), domain.Actor{UserID: "11111111-1111-4111-8111-111111111111", RequestID: "request"}, input, 3)
	if err != nil {
		t.Fatal(err)
	}
	if settings.UpdateChecksEnabled || settings.RecordVersion != 4 || repository.event.Action != "system_settings.updated" || repository.event.Metadata["nodeHealthIntervalSeconds"] != int64(10) {
		t.Fatalf("unexpected update/audit: settings=%#v event=%#v", settings, repository.event)
	}
	if got := store.RuntimeSettings(); got.NodeHealthInterval != 10*time.Second || got.QueryLogCollection {
		t.Fatalf("runtime store not updated: %#v", got)
	}
}

func TestUpdateRejectsUnsafeRuntimeBounds(t *testing.T) {
	repository := &repositoryStub{stored: StoredSettings{RuntimeInitialized: true, Runtime: Recommended(), RecordVersion: 1}}
	service := NewService(repository, Recommended(), "docker")
	_, err := service.Update(context.Background(), domain.Actor{}, Settings{NodeHealthIntervalSeconds: 1, StatisticsPollIntervalSeconds: 60, QueryLogPollIntervalSeconds: 5, QueryLogRetentionSeconds: 3600}, 1)
	if err == nil {
		t.Fatal("unsafe health interval accepted")
	}
}

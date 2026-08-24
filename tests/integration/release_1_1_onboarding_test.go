package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
	"github.com/benchristian88/atlas-dns/internal/onboarding"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

func TestRelease11OnboardingRuntimeSettingsAndNotificationCategoriesPersist(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.August, 24, 8, 0, 0, 0, time.UTC)
	const (
		userID    = "11000000-0000-4000-8000-000000000001"
		clusterID = "11000000-0000-4000-8000-000000000002"
	)
	if _, err := store.Pool().Exec(ctx, `INSERT INTO users(id,email,display_name,password_hash,role,enabled,created_at,updated_at) VALUES($1,'onboarding@example.test','Administrator','$argon2id$v=19$m=65536,t=3,p=2$dGVzdHNhbHQ$dGVzdGhhc2h0ZXN0aGFzaA','administrator',true,$2,$2)`, userID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO clusters(id,name,description,reconciliation_policy,version,created_at,updated_at) VALUES($1,'Home','Release 1.1 onboarding','manual',1,$2,$2)`, clusterID, now); err != nil {
		t.Fatal(err)
	}

	state := onboarding.State{ClusterID: clusterID, RedundancySkippedAt: &now, MonitoringReviewedAt: &now, NotificationsSkippedAt: &now, RecordVersion: 1, UpdatedAt: now}
	resourceID := clusterID
	actorID := userID
	audit := domain.AuditEvent{ID: "11000000-0000-4000-8000-000000000003", ActorType: "user", ActorUserID: &actorID, Action: "onboarding.progress_updated", ResourceType: "cluster", ResourceID: &resourceID, RequestID: "release-1.1", Metadata: map[string]any{"redundancySkipped": true}, CreatedAt: now}
	if err := store.SaveOnboardingState(ctx, state, 0, audit); err != nil {
		t.Fatal(err)
	}
	storedState, err := store.OnboardingState(ctx, clusterID)
	if err != nil || storedState.RecordVersion != 1 || storedState.RedundancySkippedAt == nil || storedState.MonitoringReviewedAt == nil || storedState.NotificationsSkippedAt == nil {
		t.Fatalf("onboarding state=%#v err=%v", storedState, err)
	}

	runtime := systemsettings.RuntimeSettings{NodeHealthInterval: 20 * time.Second, StatisticsPollInterval: 15 * time.Minute, QueryLogCollection: true, QueryLogPollInterval: 45 * time.Second, QueryLogRetention: 72 * time.Hour}
	settings, err := store.InitializeRuntimeSettings(ctx, runtime)
	if err != nil || !settings.RuntimeInitialized || settings.Runtime != runtime {
		t.Fatalf("runtime settings=%#v err=%v", settings, err)
	}

	cipher, err := auth.NewCredentialCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	notifications := haoperations.NewNotificationService(store, cipher)
	channel, err := notifications.Create(ctx, domain.Actor{UserID: userID, RequestID: "release-1.1"}, clusterID, "DNS only", "https://receiver.example.test/hook?token=hidden", true, []string{"dns"})
	if err != nil || len(channel.SubscribedCategories) != 1 || channel.SubscribedCategories[0] != "dns" {
		t.Fatalf("channel=%#v err=%v", channel, err)
	}
	if err := store.RecordHAEvent(ctx, haoperations.Event{ID: "11000000-0000-4000-8000-000000000004", ClusterID: clusterID, EventType: "redundancy.degraded", Severity: "warning", Summary: "Redundancy degraded", Details: map[string]any{}, OccurredAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordHAEvent(ctx, haoperations.Event{ID: "11000000-0000-4000-8000-000000000005", ClusterID: clusterID, EventType: "dns.failed", Severity: "critical", Summary: "DNS failed", Details: map[string]any{}, OccurredAt: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	var deliveries int
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM notification_deliveries WHERE channel_id=$1`, channel.ID).Scan(&deliveries); err != nil || deliveries != 1 {
		t.Fatalf("filtered deliveries=%d err=%v", deliveries, err)
	}
}

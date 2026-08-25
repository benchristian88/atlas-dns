package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/database"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

func TestRelease110FreshRuntimeAndNotificationPolicyDefaults(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	service := systemsettings.NewService(store, systemsettings.Recommended(), "docker")
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	settings, err := service.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.SessionDurationSeconds != 43200 || settings.NodeRequestTimeoutSeconds != 10 || settings.LogLevel != "info" || settings.OperationalHistoryRetentionDays != 90 {
		t.Fatalf("fresh runtime settings = %#v", settings)
	}
	policy, err := store.NotificationPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.EnabledEventTypes) != len(haoperations.RecommendedNotificationEventTypes()) || policy.RecordVersion != 1 {
		t.Fatalf("fresh notification policy = %#v", policy)
	}
}

func TestRelease110MigratesV102RuntimeWithoutLosingPersistedValues(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	if err := database.RollbackLastMigration(ctx, store.Pool()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE system_settings SET runtime_settings_initialized=true,
		node_health_interval_seconds=45,statistics_poll_interval_seconds=7200,
		query_log_collection_enabled=false,query_log_poll_interval_seconds=60,
		query_log_retention_seconds=1209600 WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	if err := database.ApplyMigrations(ctx, store.Pool()); err != nil {
		t.Fatal(err)
	}
	service := systemsettings.NewService(store, systemsettings.Recommended(), "docker")
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	settings, err := service.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.NodeHealthIntervalSeconds != 45 || settings.StatisticsPollIntervalSeconds != 7200 || settings.QueryLogCollectionEnabled || settings.QueryLogPollIntervalSeconds != 60 || settings.QueryLogRetentionSeconds != 1209600 {
		t.Fatalf("v1.0.2 persisted values were not preserved: %#v", settings)
	}
	if settings.SessionDurationSeconds != 43200 || settings.NodeRequestTimeoutSeconds != 10 || settings.LogLevel != "info" || settings.OperationalHistoryRetentionDays != 90 {
		t.Fatalf("new v1.1 values were not safely initialized: %#v", settings)
	}
}

func TestOperationalHistoryClearPreservesAuditAndRecordsResult(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	const (
		userID    = "11000000-0000-4000-8000-000000000001"
		clusterID = "11000000-0000-4000-8000-000000000002"
		eventID   = "11000000-0000-4000-8000-000000000003"
		auditID   = "11000000-0000-4000-8000-000000000004"
		clearID   = "11000000-0000-4000-8000-000000000005"
	)
	if _, err := store.Pool().Exec(ctx, `INSERT INTO users(id,email,display_name,password_hash,role,enabled,created_at,updated_at) VALUES($1,'v110@example.test','Administrator','hash','administrator',true,$2,$2)`, userID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO clusters(id,name,created_at,updated_at) VALUES($1,'v1.1 history',$2,$2)`, clusterID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO ha_operational_events(id,cluster_id,event_type,severity,summary,details_json,occurred_at) VALUES($1,$2,'dns.failed','critical','DNS failed','{}',$3)`, eventID, clusterID, now); err != nil {
		t.Fatal(err)
	}
	actorID := userID
	if err := store.RecordAuditEvent(ctx, domain.AuditEvent{ID: auditID, ActorType: "user", ActorUserID: &actorID, Action: "history.before", ResourceType: "test", RequestID: "before", Metadata: map[string]any{}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	result, err := store.ClearOperationalHistory(ctx, domain.AuditEvent{ID: clearID, ActorType: "user", ActorUserID: &actorID, Action: "operational_history.cleared", ResourceType: "operational_history", RequestID: "clear", Metadata: map[string]any{}, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	var historyCount, auditCount int
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM ha_operational_events`).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action IN ('history.before','operational_history.cleared')`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if result.EventsDeleted != 1 || historyCount != 0 || auditCount != 2 {
		t.Fatalf("result=%#v history=%d audits=%d", result, historyCount, auditCount)
	}
}

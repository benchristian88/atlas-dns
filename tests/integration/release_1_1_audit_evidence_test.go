package integration_test

import (
	"context"
	"testing"
	"time"

	auditservice "github.com/benchristian88/atlas-dns/internal/audit"
	"github.com/benchristian88/atlas-dns/internal/domain"
)

func TestRelease11AuditKeysetScopeAndHistoricalRedaction(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	const (
		userID   = "11000000-0000-4000-8000-000000000001"
		clusterA = "11000000-0000-4000-8000-000000000002"
		clusterB = "11000000-0000-4000-8000-000000000003"
	)
	if _, err := store.Pool().Exec(ctx, `INSERT INTO users (id,email,display_name,password_hash,role) VALUES ($1,'audit@example.test','Current Audit Admin','hash','administrator')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO clusters (id,name) VALUES ($1,'Audit A'),($2,'Audit B')`, clusterA, clusterB); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	insertAudit := func(id, action, resourceType string, createdAt time.Time, metadata string) {
		t.Helper()
		if _, err := store.Pool().Exec(ctx, `INSERT INTO audit_events
			(id,actor_type,actor_user_id,action,resource_type,request_id,metadata_json,created_at)
			VALUES ($1,'user',$2,$3,$4,'11000000-0000-4000-8000-000000000099',$5::jsonb,$6)`,
			id, userID, action, resourceType, metadata, createdAt); err != nil {
			t.Fatal(err)
		}
	}
	insertAudit("11000000-0000-4000-8000-000000000013", "cluster.updated", "cluster", at, `{"clusterId":"`+clusterA+`","name":"Audit A","password":"historical-secret"}`)
	insertAudit("11000000-0000-4000-8000-000000000012", "configuration.draft_updated", "configuration_draft", at, `{"clusterId":"`+clusterA+`","valid":true}`)
	insertAudit("11000000-0000-4000-8000-000000000014", "node.updated", "node", at.Add(time.Second), `{"clusterId":"`+clusterB+`"}`)
	insertAudit("11000000-0000-4000-8000-000000000011", "system_settings.updated", "system_settings", at.Add(-time.Second), `{"queryLogRetentionSeconds":720}`)
	actorID := userID
	clusterResourceID := clusterA
	if err := store.RecordAuditEvent(ctx, domain.AuditEvent{
		ID: "11000000-0000-4000-8000-000000000010", ActorType: "user", ActorUserID: &actorID,
		Action: "cluster.updated", ResourceType: "cluster", ResourceID: &clusterResourceID,
		RequestID: "11000000-0000-4000-8000-000000000098", CreatedAt: at.Add(-2 * time.Second),
		Metadata: map[string]any{"clusterId": clusterA, "password": "producer-secret", "name": "Audit A"},
	}); err != nil {
		t.Fatal(err)
	}
	var storedPassword string
	if err := store.Pool().QueryRow(ctx, `SELECT metadata_json->>'password' FROM audit_events WHERE id='11000000-0000-4000-8000-000000000010'`).Scan(&storedPassword); err != nil {
		t.Fatal(err)
	}
	if storedPassword != "[REDACTED]" {
		t.Fatalf("producer-side stored password = %q", storedPassword)
	}

	service := auditservice.NewService(store)
	first, err := service.List(ctx, auditservice.ListRequest{Limit: 2, ClusterID: clusterA, IncludeController: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "11000000-0000-4000-8000-000000000013" || first.Items[1].ID != "11000000-0000-4000-8000-000000000012" || first.NextCursor == "" {
		t.Fatalf("first scoped page = %+v", first)
	}
	if first.Items[0].ActorDisplayName != "Current Audit Admin" || first.Items[0].Metadata["password"] != "[REDACTED]" {
		t.Fatalf("actor/redaction representation = %+v", first.Items[0])
	}
	for _, event := range first.Items {
		if event.ClusterID != clusterA || event.Scope != "cluster" {
			t.Fatalf("cluster event context = %+v", event)
		}
	}

	// A newer concurrent insert cannot move the page behind the issued cursor.
	insertAudit("11000000-0000-4000-8000-000000000015", "cluster.updated", "cluster", at.Add(2*time.Second), `{"clusterId":"`+clusterA+`"}`)
	next, err := service.List(ctx, auditservice.ListRequest{Limit: 2, Cursor: first.NextCursor, ClusterID: clusterA, IncludeController: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 2 || next.Items[0].Action != "system_settings.updated" || next.Items[0].Scope != "controller" || next.Items[0].ClusterID != "" || next.Items[1].ID != "11000000-0000-4000-8000-000000000010" {
		t.Fatalf("next scoped page = %+v", next)
	}

	var indexDefinition string
	if err := store.Pool().QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname=current_schema() AND indexname='audit_events_created_at_id_idx'`).Scan(&indexDefinition); err != nil {
		t.Fatal(err)
	}
	if indexDefinition == "" {
		t.Fatal("audit keyset index is missing")
	}
	var schemaVersion int
	if err := store.Pool().QueryRow(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&schemaVersion); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != 19 {
		t.Fatalf("schema version = %d, want 19", schemaVersion)
	}
}

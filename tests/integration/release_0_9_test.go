package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
	"github.com/benchristian88/atlas-dns/internal/updates"
)

func TestRelease09UserLifecycleAndSettingsPersistence(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.August, 9, 8, 0, 0, 0, time.UTC)
	const firstID = "90000000-0000-4000-8000-000000000001"
	const secondID = "90000000-0000-4000-8000-000000000002"
	for _, user := range []struct{ id, email string }{{firstID, "first@example.test"}, {secondID, "second@example.test"}} {
		if _, err := store.Pool().Exec(ctx, `INSERT INTO users(id,email,display_name,password_hash,role,enabled,created_at,updated_at) VALUES($1,$2,'Administrator','$argon2id$v=19$m=65536,t=3,p=2$dGVzdHNhbHQ$dGVzdGhhc2h0ZXN0aGFzaA','administrator',true,$3,$3)`, user.id, user.email, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_hash,created_at,expires_at,last_seen_at) VALUES('90000000-0000-4000-8000-000000000003',$1,$2,$3,$4,$5,$4)`, firstID, []byte("token"), []byte("csrf"), now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	event := domain.AuditEvent{ID: "90000000-0000-4000-8000-000000000004", ActorType: "user", ActorUserID: stringPointer09(secondID), Action: "user.updated", ResourceType: "user", ResourceID: stringPointer09(firstID), RequestID: "release-0.9-test", Metadata: map[string]any{"enabled": false}, CreatedAt: now.Add(time.Minute)}
	updated, err := store.UpdateUser(ctx, firstID, "first@example.test", "First admin", false, now.Add(time.Minute), event)
	if err != nil || updated.Enabled {
		t.Fatalf("disable first administrator: user=%#v err=%v", updated, err)
	}
	var revoked bool
	if err := store.Pool().QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM sessions WHERE user_id=$1`, firstID).Scan(&revoked); err != nil || !revoked {
		t.Fatalf("disabled session revoked=%v err=%v", revoked, err)
	}
	finalEvent := event
	finalEvent.ID = "90000000-0000-4000-8000-000000000005"
	finalEvent.ActorUserID = stringPointer09(firstID)
	finalEvent.ResourceID = stringPointer09(secondID)
	if _, err := store.UpdateUser(ctx, secondID, "second@example.test", "Second admin", false, now.Add(2*time.Minute), finalEvent); err == nil {
		t.Fatal("final enabled administrator was disabled")
	}

	settings, err := store.InitializeRuntimeSettings(ctx, systemsettings.Recommended())
	if err != nil || !settings.UpdateChecksEnabled || settings.RecordVersion != 1 {
		t.Fatalf("default settings=%#v err=%v", settings, err)
	}
	settingsEvent := domain.AuditEvent{ID: "90000000-0000-4000-8000-000000000006", ActorType: "user", ActorUserID: stringPointer09(secondID), Action: "system_settings.updated", ResourceType: "system_settings", RequestID: "release-0.9-test", Metadata: map[string]any{"updateChecksEnabled": false}, CreatedAt: now}
	settings.UpdateChecksEnabled = false
	settings, err = store.UpdateSystemSettings(ctx, settings, 1, now, settingsEvent)
	if err != nil || settings.UpdateChecksEnabled || settings.RecordVersion != 2 {
		t.Fatalf("updated settings=%#v err=%v", settings, err)
	}

	cache := updates.Cache{Version: "v0.9.1", ReleaseURL: "https://github.com/benchristian88/atlas-dns/releases/tag/v0.9.1", ReleaseNotes: "Security fixes", CheckedAt: now, ExpiresAt: now.Add(6 * time.Hour)}
	if err := store.SaveControllerReleaseCache(ctx, cache); err != nil {
		t.Fatal(err)
	}
	stored, err := store.ControllerReleaseCache(ctx)
	if err != nil || stored.Version != cache.Version || stored.ReleaseURL != cache.ReleaseURL {
		t.Fatalf("release cache=%#v err=%v", stored, err)
	}
}

func stringPointer09(value string) *string { return &value }

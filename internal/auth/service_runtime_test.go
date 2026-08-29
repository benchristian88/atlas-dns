package auth

import (
	"bytes"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

func TestSessionDurationChangeAppliesOnlyToNewSessions(t *testing.T) {
	tokens, err := NewTokenManager(bytes.Repeat([]byte("s"), 32))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 5, 0, 0, 0, time.UTC)
	service := &Service{tokens: tokens, sessionDuration: 12 * time.Hour, now: func() time.Time { return now }}
	user := domain.User{ID: "11111111-1111-4111-8111-111111111111"}
	first, _, err := service.buildSession(user, "request-1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	firstExpiry := first.Session.ExpiresAt
	runtime := systemsettings.NewRuntimeStore(systemsettings.RuntimeSettings{SessionDuration: 24 * time.Hour})
	service.SetRuntimeSettings(runtime)
	second, _, err := service.buildSession(user, "request-2", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Session.ExpiresAt.Equal(firstExpiry) || !first.Session.ExpiresAt.Equal(now.Add(12*time.Hour)) {
		t.Fatalf("existing session expiry changed: %v", first.Session.ExpiresAt)
	}
	if !second.Session.ExpiresAt.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("new session expiry = %v", second.Session.ExpiresAt)
	}
}

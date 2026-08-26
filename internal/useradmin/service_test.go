package useradmin

import (
	"context"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/domain"
)

type repositoryStub struct {
	users        []domain.User
	event        domain.AuditEvent
	passwordHash string
	sessionID    string
}

func (r *repositoryStub) ListUsers(context.Context) ([]domain.User, error) { return r.users, nil }
func (r *repositoryStub) UserByID(_ context.Context, id string) (domain.User, error) {
	for _, user := range r.users {
		if user.ID == id {
			return user, nil
		}
	}
	return domain.User{ID: id, Email: "user@example.test", DisplayName: "User", Role: domain.RoleAdministrator, Enabled: true}, nil
}
func (r *repositoryStub) CreateUser(_ context.Context, user domain.User, event domain.AuditEvent) error {
	r.users = append(r.users, user)
	r.event = event
	return nil
}
func (r *repositoryStub) UpdateUser(_ context.Context, id, email, displayName string, enabled bool, now time.Time, event domain.AuditEvent) (domain.User, error) {
	user := domain.User{ID: id, Email: email, DisplayName: displayName, Role: domain.RoleAdministrator, Enabled: enabled, UpdatedAt: now}
	r.event = event
	return user, nil
}
func (r *repositoryStub) ResetUserPassword(_ context.Context, _ string, hash string, _ time.Time, event domain.AuditEvent) error {
	r.passwordHash = hash
	r.event = event
	return nil
}
func (r *repositoryStub) ChangeOwnPassword(_ context.Context, _ string, sessionID string, hash string, _ time.Time, event domain.AuditEvent) error {
	r.passwordHash = hash
	r.sessionID = sessionID
	r.event = event
	return nil
}

func TestCreateAdministratorHashesPasswordAndAuditsSafely(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	service.now = func() time.Time { return time.Unix(1, 0) }
	user, err := service.Create(context.Background(), domain.Actor{UserID: "11111111-1111-4111-8111-111111111111", RequestID: "request"}, CreateInput{Email: "ADMIN2@example.test", DisplayName: "Second admin", Password: "correct horse battery", Role: domain.RoleAdministrator})
	if err != nil {
		t.Fatal(err)
	}
	if user.PasswordHash == "correct horse battery" {
		t.Fatal("plaintext password retained")
	}
	valid, err := auth.VerifyPassword(user.PasswordHash, "correct horse battery")
	if err != nil || !valid {
		t.Fatal("password hash does not verify")
	}
	if repository.event.Action != "user.created" {
		t.Fatalf("unexpected audit: %#v", repository.event)
	}
	for _, value := range repository.event.Metadata {
		if value == "correct horse battery" {
			t.Fatal("password leaked to audit")
		}
	}
}

func TestRejectsUnsupportedRoleAndSelfDisable(t *testing.T) {
	service := NewService(&repositoryStub{})
	if _, err := service.Create(context.Background(), domain.Actor{}, CreateInput{Email: "user@example.test", DisplayName: "User", Password: "correct horse battery", Role: "operator"}); err == nil {
		t.Fatal("expected invalid role")
	}
	id := "11111111-1111-4111-8111-111111111111"
	if _, err := service.Update(context.Background(), domain.Actor{UserID: id}, id, UpdateInput{Email: "user@example.test", DisplayName: "User", Role: domain.RoleAdministrator, Enabled: false}); err == nil {
		t.Fatal("expected self-disable rejection")
	}
}

func TestPasswordResetHashesAndRequestsSessionRevocation(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	err := service.ResetPassword(context.Background(), domain.Actor{UserID: "22222222-2222-4222-8222-222222222222"}, "11111111-1111-4111-8111-111111111111", "new secure password")
	if err != nil {
		t.Fatal(err)
	}
	if repository.passwordHash == "new secure password" || repository.event.Metadata["sessionsRevoked"] != true {
		t.Fatal("unsafe reset")
	}
}

func TestChangeOwnPasswordVerifiesCurrentCredentialAndKeepsCurrentSession(t *testing.T) {
	userID := "11111111-1111-4111-8111-111111111111"
	sessionID := "22222222-2222-4222-8222-222222222222"
	currentHash, err := auth.HashPassword("current secure password")
	if err != nil {
		t.Fatal(err)
	}
	repository := &repositoryStub{users: []domain.User{{ID: userID, PasswordHash: currentHash, Enabled: true}}}
	service := NewService(repository)
	actor := domain.Actor{UserID: userID, RequestID: "request"}

	if err := service.ChangeOwnPassword(context.Background(), actor, sessionID, "wrong password", "replacement secure password"); err == nil {
		t.Fatal("expected incorrect current password to be rejected")
	}
	if repository.passwordHash != "" {
		t.Fatal("password changed after failed current-password verification")
	}
	if err := service.ChangeOwnPassword(context.Background(), actor, sessionID, "current secure password", "replacement secure password"); err != nil {
		t.Fatal(err)
	}
	valid, err := auth.VerifyPassword(repository.passwordHash, "replacement secure password")
	if err != nil || !valid {
		t.Fatal("replacement password was not hashed correctly")
	}
	if repository.sessionID != sessionID || repository.event.Action != "user.password_changed" || repository.event.Metadata["otherSessionsRevoked"] != true {
		t.Fatalf("unsafe own-password change evidence: session=%q event=%#v", repository.sessionID, repository.event)
	}
}

func TestEnableDisableAndLoginIdentifierChangesHaveSpecificSafeAudits(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	repository := &repositoryStub{users: []domain.User{{ID: id, Email: "user@example.test", DisplayName: "User", Role: domain.RoleAdministrator, Enabled: true}}}
	service := NewService(repository)
	actor := domain.Actor{UserID: "22222222-2222-4222-8222-222222222222"}
	if _, err := service.Update(context.Background(), actor, id, UpdateInput{Email: "user@example.test", DisplayName: "User", Role: domain.RoleAdministrator, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if repository.event.Action != "user.disabled" {
		t.Fatalf("unexpected disable audit: %#v", repository.event)
	}
	repository.users[0].Enabled = true
	if _, err := service.Update(context.Background(), actor, id, UpdateInput{Email: "renamed@example.test", DisplayName: "User", Role: domain.RoleAdministrator, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if repository.event.Action != "user.login_identifier_changed" || repository.event.Metadata["loginIdentifierChanged"] != true {
		t.Fatalf("unexpected identifier audit: %#v", repository.event)
	}
}

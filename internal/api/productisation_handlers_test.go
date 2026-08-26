package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/useradmin"
)

type userAdministrationFake struct {
	users                   []domain.User
	calls                   int
	passwordActor           domain.Actor
	passwordSessionID       string
	passwordCurrentPassword string
	passwordNewPassword     string
}

func (f *userAdministrationFake) List(context.Context) ([]domain.User, error) {
	f.calls++
	return f.users, nil
}
func (f *userAdministrationFake) Create(context.Context, domain.Actor, useradmin.CreateInput) (domain.User, error) {
	f.calls++
	return domain.User{}, nil
}
func (f *userAdministrationFake) Update(context.Context, domain.Actor, string, useradmin.UpdateInput) (domain.User, error) {
	f.calls++
	return domain.User{}, nil
}
func (f *userAdministrationFake) ResetPassword(context.Context, domain.Actor, string, string) error {
	f.calls++
	return nil
}
func (f *userAdministrationFake) ChangeOwnPassword(_ context.Context, actor domain.Actor, sessionID, currentPassword, newPassword string) error {
	f.calls++
	f.passwordActor = actor
	f.passwordSessionID = sessionID
	f.passwordCurrentPassword = currentPassword
	f.passwordNewPassword = newPassword
	return nil
}

func TestProductisationRoutesRejectUnauthenticatedRequests(t *testing.T) {
	tests := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/users"},
		{http.MethodPost, "/api/v1/users"},
		{http.MethodPatch, "/api/v1/users/11111111-1111-4111-8111-111111111111"},
		{http.MethodPost, "/api/v1/users/11111111-1111-4111-8111-111111111111/password-reset"},
		{http.MethodPost, "/api/v1/auth/password"},
		{http.MethodPost, "/api/v1/system/backups"},
		{http.MethodPost, "/api/v1/system/restore-preflight"},
		{http.MethodGet, "/api/v1/system/update"},
		{http.MethodPost, "/api/v1/system/update/check"},
		{http.MethodGet, "/api/v1/system/settings"},
		{http.MethodPatch, "/api/v1/system/settings"},
	}
	for _, test := range tests {
		t.Run(test.method+test.path, func(t *testing.T) {
			server := &Server{mux: http.NewServeMux()}
			server.routes()
			response := httptest.NewRecorder()
			server.mux.ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(`{}`)))
			if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"code":"AUTHENTICATION_REQUIRED"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestUserAdministrationRequiresServerSideAdministratorAndCSRF(t *testing.T) {
	tokens, err := auth.NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	const csrf = "productisation-csrf"
	repository := &apiAuthRepositoryFake{
		session: domain.Session{ID: "session-a", CSRFHash: tokens.HashCSRFToken(csrf)},
		user:    domain.User{ID: "33333333-3333-4333-8333-333333333333", Enabled: true, Role: "viewer"},
	}
	authService, err := auth.NewService(repository, tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	users := &userAdministrationFake{}
	server := &Server{auth: authService, users: users, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), mux: http.NewServeMux()}
	server.routes()

	list := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	list.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	listResponse := httptest.NewRecorder()
	server.mux.ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusForbidden || users.calls != 0 {
		t.Fatalf("non-administrator status=%d calls=%d", listResponse.Code, users.calls)
	}

	repository.user.Role = domain.RoleAdministrator
	create := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{}`))
	create.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	createResponse := httptest.NewRecorder()
	server.mux.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusForbidden || users.calls != 0 {
		t.Fatalf("missing CSRF status=%d calls=%d", createResponse.Code, users.calls)
	}
}

func TestUserListNeverReturnsPasswordHash(t *testing.T) {
	tokens, err := auth.NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	repository := &apiAuthRepositoryFake{
		session: domain.Session{ID: "session-a"},
		user:    domain.User{ID: "33333333-3333-4333-8333-333333333333", Enabled: true, Role: domain.RoleAdministrator},
	}
	authService, err := auth.NewService(repository, tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	users := &userAdministrationFake{users: []domain.User{{ID: "11111111-1111-4111-8111-111111111111", Email: "admin@example.test", DisplayName: "Admin", PasswordHash: "plaintext-must-not-appear", Role: domain.RoleAdministrator, Enabled: true}}}
	server := &Server{auth: authService, users: users, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), mux: http.NewServeMux()}
	server.routes()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	response := httptest.NewRecorder()
	server.mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "plaintext-must-not-appear") || strings.Contains(response.Body.String(), "passwordHash") {
		t.Fatalf("unsafe response status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestOwnPasswordChangeUsesAuthenticatedIdentityAndCurrentSession(t *testing.T) {
	tokens, err := auth.NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	const csrf = "password-change-csrf"
	const userID = "33333333-3333-4333-8333-333333333333"
	const sessionID = "22222222-2222-4222-8222-222222222222"
	repository := &apiAuthRepositoryFake{
		session: domain.Session{ID: sessionID, CSRFHash: tokens.HashCSRFToken(csrf)},
		user:    domain.User{ID: userID, Enabled: true, Role: domain.RoleAdministrator},
	}
	authService, err := auth.NewService(repository, tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	users := &userAdministrationFake{}
	server := &Server{auth: authService, users: users, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), mux: http.NewServeMux()}
	server.routes()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(`{"currentPassword":"current secret","newPassword":"replacement secret"}`))
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	request.Header.Set(csrfHeader, csrf)
	response := httptest.NewRecorder()
	server.mux.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if users.passwordActor.UserID != userID || users.passwordSessionID != sessionID || users.passwordCurrentPassword != "current secret" || users.passwordNewPassword != "replacement secret" {
		t.Fatalf("self-service identity was not derived from authentication: %#v", users)
	}
}

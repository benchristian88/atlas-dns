package api

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/controlplane"
	"github.com/benchristian88/atlas-dns/internal/domain"
)

// TestProtectedRouteInventoryRequiresAuthentication turns the route table into
// an executable security boundary. A new API route must be deliberately added
// through authenticated or administrator middleware, or explicitly reviewed
// and added to the small public allowlist below.
func TestProtectedRouteInventoryRequiresAuthentication(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "server.go"))
	if err != nil {
		t.Fatal(err)
	}

	protectedPattern := regexp.MustCompile(`s\.mux\.Handle\("([A-Z]+) ([^"]+)", s\.(authenticated|administrator)\(`)
	protected := protectedPattern.FindAllSubmatch(source, -1)
	if got, want := len(protected), 87; got != want {
		t.Fatalf("protected route inventory contains %d routes, want %d; review every route before changing this gate", got, want)
	}
	if got := bytes.Count(source, []byte("s.mux.Handle(")); got != len(protected) {
		t.Fatalf("found %d routes but only %d use reviewed authentication middleware", got, len(protected))
	}

	publicPattern := regexp.MustCompile(`s\.mux\.HandleFunc\("([^"]+)"`)
	public := publicPattern.FindAllSubmatch(source, -1)
	publicAllowlist := map[string]bool{
		"GET /health":              true,
		"GET /ready":               true,
		"GET /metrics":             true,
		"GET /api/v1/setup/status": true,
		"POST /api/v1/setup":       true,
		"POST /api/v1/auth/login":  true,
		"/":                        true,
	}
	if len(public) != len(publicAllowlist) {
		t.Fatalf("public route inventory contains %d routes, want %d", len(public), len(publicAllowlist))
	}
	for _, match := range public {
		if !publicAllowlist[string(match[1])] {
			t.Fatalf("unreviewed public route %q", match[1])
		}
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(nil, nil, nil, nil, nil, logger, false, "http://controller.example.test", time.Minute, t.TempDir(), &controlplane.Service{})
	pathValuePattern := regexp.MustCompile(`\{[^}]+\}`)
	for _, match := range protected {
		method := string(match[1])
		path := pathValuePattern.ReplaceAllString(string(match[2]), "test-id")
		t.Run(method+" "+path, func(t *testing.T) {
			request := httptest.NewRequest(method, path, nil)
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
			}
		})
	}
}

type boundaryAuthRepository struct {
	*apiAuthRepositoryFake
	tokens       *auth.TokenManager
	expected     string
	rejectExpiry bool
}

func (r *boundaryAuthRepository) AuthenticatedSession(_ context.Context, tokenHash []byte, now time.Time) (domain.Session, domain.User, error) {
	if !r.tokens.Equal(tokenHash, r.tokens.HashSessionToken(r.expected)) || r.rejectExpiry || !r.session.ExpiresAt.After(now) {
		return domain.Session{}, domain.User{}, domain.NewError(domain.ErrorAuthentication, "authentication is required")
	}
	return r.session, r.user, nil
}

func newBoundaryAuth(t *testing.T) (*auth.Service, *boundaryAuthRepository, string) {
	t.Helper()
	tokens, err := auth.NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	const csrf = "boundary-csrf-token"
	repository := &boundaryAuthRepository{
		apiAuthRepositoryFake: &apiAuthRepositoryFake{
			session: domain.Session{ID: "session-a", CSRFHash: tokens.HashCSRFToken(csrf), ExpiresAt: time.Now().Add(time.Hour)},
			user:    domain.User{ID: "33333333-3333-4333-8333-333333333333", Role: domain.RoleAdministrator, Enabled: true},
		},
		tokens:   tokens,
		expected: "valid-session-token",
	}
	service, err := auth.NewService(repository, tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return service, repository, csrf
}

func TestSessionBoundaryRejectsMissingTamperedAndExpiredTokens(t *testing.T) {
	authService, repository, _ := newBoundaryAuth(t)
	server := &Server{auth: authService, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), mux: http.NewServeMux()}
	server.routes()

	tests := []struct {
		name    string
		token   string
		expired bool
	}{
		{name: "missing"},
		{name: "tampered", token: "tampered-session-token"},
		{name: "expired", token: repository.expected, expired: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository.rejectExpiry = test.expired
			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			if test.token != "" {
				request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: test.token})
			}
			response := httptest.NewRecorder()
			server.mux.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"code":"AUTHENTICATION_REQUIRED"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestMutationBoundaryRejectsCSRFAndMassAssignment(t *testing.T) {
	authService, _, csrf := newBoundaryAuth(t)
	server := &Server{auth: authService, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), mux: http.NewServeMux()}
	server.routes()

	withoutCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", strings.NewReader(`{"name":"Cluster"}`))
	withoutCSRF.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid-session-token"})
	withoutResponse := httptest.NewRecorder()
	server.mux.ServeHTTP(withoutResponse, withoutCSRF)
	if withoutResponse.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d body=%s", withoutResponse.Code, withoutResponse.Body.String())
	}

	massAssignment := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", strings.NewReader(`{"name":"Cluster","role":"administrator","userId":"attacker"}`))
	massAssignment.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid-session-token"})
	massAssignment.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	massAssignment.Header.Set(csrfHeader, csrf)
	massAssignmentResponse := httptest.NewRecorder()
	server.mux.ServeHTTP(massAssignmentResponse, massAssignment)
	if massAssignmentResponse.Code != http.StatusBadRequest || !strings.Contains(massAssignmentResponse.Body.String(), `"field":"body"`) {
		t.Fatalf("mass-assignment status=%d body=%s", massAssignmentResponse.Code, massAssignmentResponse.Body.String())
	}
}

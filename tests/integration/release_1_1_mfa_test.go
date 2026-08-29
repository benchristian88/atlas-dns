package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/benchristian88/atlas-dns/internal/adguard"
	controllerapi "github.com/benchristian88/atlas-dns/internal/api"
	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

func TestRelease110MFAHTTPFlowRecoveryConcurrencyAndHostReset(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	cipher, err := auth.NewCredentialCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewTokenManager(bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(store, tokens, 12*time.Hour, cipher)
	if err != nil {
		t.Fatal(err)
	}
	probe := adguard.NewProbe(2 * time.Second)
	management := domain.NewManagementService(store, cipher, probe)
	inventoryService := inventory.NewService(store, cipher, adguard.NewConfigurationReader(probe))
	server := httptest.NewServer(controllerapi.NewServer(
		authService, management, inventoryService, store, store,
		slog.New(slog.NewTextHandler(io.Discard, nil)), false, "http://controller.example.test",
		30*time.Second, t.TempDir(),
	).Handler())
	t.Cleanup(server.Close)
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}

	const (
		email    = "mfa-admin@example.test"
		password = "correct horse battery staple"
	)
	setup := doJSON(t, client, http.MethodPost, server.URL+"/api/v1/setup",
		`{"email":"`+email+`","displayName":"MFA Administrator","password":"`+password+`"}`, "")
	if setup.StatusCode != http.StatusCreated {
		t.Fatalf("setup status=%d body=%s", setup.StatusCode, readBody(t, setup))
	}
	_ = readBody(t, setup)
	csrf := cookieValue(jar.Cookies(baseURL), "atlas_dns_csrf")

	start := doJSON(t, client, http.MethodPost, server.URL+"/api/v1/account/mfa/enroll/start",
		`{"currentPassword":"`+password+`"}`, csrf)
	if start.StatusCode != http.StatusOK {
		t.Fatalf("enrollment start status=%d body=%s", start.StatusCode, readBody(t, start))
	}
	var enrollment struct {
		Secret string `json:"secret"`
	}
	decodeBody(t, start, &enrollment)
	if enrollment.Secret == "" {
		t.Fatal("enrollment did not return the one-time secret")
	}
	code, err := totp.GenerateCode(enrollment.Secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	verifyEnrollment := doJSON(t, client, http.MethodPost, server.URL+"/api/v1/account/mfa/enroll/verify",
		`{"code":"`+code+`"}`, csrf)
	if verifyEnrollment.StatusCode != http.StatusOK {
		t.Fatalf("enrollment verify status=%d body=%s", verifyEnrollment.StatusCode, readBody(t, verifyEnrollment))
	}
	var enabled struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	decodeBody(t, verifyEnrollment, &enabled)
	if len(enabled.RecoveryCodes) != auth.MFARecoveryCodeCount {
		t.Fatalf("recovery code count=%d", len(enabled.RecoveryCodes))
	}

	var encryptedSecret []byte
	if err := store.Pool().QueryRow(ctx, `SELECT encrypted_secret FROM user_mfa`).Scan(&encryptedSecret); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encryptedSecret, []byte(enrollment.Secret)) {
		t.Fatal("database contains plaintext TOTP secret")
	}
	recoveryRows, err := store.Pool().Query(ctx, `SELECT code_hash FROM user_mfa_recovery_codes`)
	if err != nil {
		t.Fatal(err)
	}
	defer recoveryRows.Close()
	normalizedRecovery := []byte(strings.ReplaceAll(enabled.RecoveryCodes[0], "-", ""))
	storedRecoveryCount := 0
	for recoveryRows.Next() {
		var hash []byte
		if err := recoveryRows.Scan(&hash); err != nil {
			t.Fatal(err)
		}
		storedRecoveryCount++
		if bytes.Contains(hash, normalizedRecovery) || bytes.Contains(hash, []byte(enabled.RecoveryCodes[0])) {
			t.Fatal("database recovery hash contains plaintext recovery material")
		}
	}
	if err := recoveryRows.Err(); err != nil {
		t.Fatal(err)
	}
	if storedRecoveryCount != auth.MFARecoveryCodeCount {
		t.Fatalf("stored recovery hash count=%d", storedRecoveryCount)
	}

	logout := doJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/logout", `{}`, csrf)
	if logout.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", logout.StatusCode, readBody(t, logout))
	}
	_ = readBody(t, logout)
	passwordOnlyJar, _ := cookiejar.New(nil)
	passwordOnly := &http.Client{Jar: passwordOnlyJar, Timeout: 5 * time.Second}
	login := doJSON(t, passwordOnly, http.MethodPost, server.URL+"/api/v1/auth/login",
		`{"email":"`+email+`","password":"`+password+`"}`, "")
	if login.StatusCode != http.StatusAccepted {
		t.Fatalf("MFA login status=%d body=%s", login.StatusCode, readBody(t, login))
	}
	if len(login.Header.Values("Set-Cookie")) != 0 {
		t.Fatalf("password stage issued cookies: %v", login.Header.Values("Set-Cookie"))
	}
	var challenge struct {
		MFARequired  bool   `json:"mfaRequired"`
		MFAChallenge string `json:"mfaChallenge"`
	}
	decodeBody(t, login, &challenge)
	if !challenge.MFARequired || challenge.MFAChallenge == "" {
		t.Fatalf("password stage response=%#v", challenge)
	}
	var activeSessions int
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM sessions WHERE revoked_at IS NULL AND expires_at>now()`).Scan(&activeSessions); err != nil {
		t.Fatal(err)
	}
	if activeSessions != 0 {
		t.Fatalf("password stage created %d active sessions", activeSessions)
	}
	me := doJSON(t, passwordOnly, http.MethodGet, server.URL+"/api/v1/auth/me", "", "")
	if me.StatusCode != http.StatusUnauthorized {
		t.Fatalf("protected route before second factor status=%d body=%s", me.StatusCode, readBody(t, me))
	}
	_ = readBody(t, me)

	code, _ = totp.GenerateCode(enrollment.Secret, time.Now().UTC())
	complete := doJSON(t, passwordOnly, http.MethodPost, server.URL+"/api/v1/auth/mfa/verify",
		`{"challenge":"`+challenge.MFAChallenge+`","code":"`+code+`"}`, "")
	if complete.StatusCode != http.StatusOK {
		t.Fatalf("MFA completion status=%d body=%s", complete.StatusCode, readBody(t, complete))
	}
	_ = readBody(t, complete)
	if cookieValue(passwordOnlyJar.Cookies(baseURL), "atlas_dns_session") == "" {
		t.Fatal("second factor did not issue the canonical session cookie")
	}
	authorizedMe := doJSON(t, passwordOnly, http.MethodGet, server.URL+"/api/v1/auth/me", "", "")
	if authorizedMe.StatusCode != http.StatusOK {
		t.Fatalf("protected route after second factor status=%d body=%s", authorizedMe.StatusCode, readBody(t, authorizedMe))
	}
	_ = readBody(t, authorizedMe)
	replay := doJSON(t, &http.Client{Timeout: 5 * time.Second}, http.MethodPost, server.URL+"/api/v1/auth/mfa/verify",
		`{"challenge":"`+challenge.MFAChallenge+`","code":"`+code+`"}`, "")
	if replay.StatusCode == http.StatusOK {
		t.Fatal("consumed MFA challenge replayed")
	}
	_ = readBody(t, replay)

	// A single password challenge is raced deliberately. Exactly one transaction
	// may consume both the challenge and the one-time recovery code.
	recoveryLogin := doJSON(t, &http.Client{Timeout: 5 * time.Second}, http.MethodPost, server.URL+"/api/v1/auth/login",
		`{"email":"`+email+`","password":"`+password+`"}`, "")
	if recoveryLogin.StatusCode != http.StatusAccepted {
		t.Fatalf("recovery login status=%d body=%s", recoveryLogin.StatusCode, readBody(t, recoveryLogin))
	}
	var recoveryChallenge struct {
		MFAChallenge string `json:"mfaChallenge"`
	}
	decodeBody(t, recoveryLogin, &recoveryChallenge)
	type recoveryAttempt struct {
		status int
		err    error
	}
	attempts := make(chan recoveryAttempt, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			request, requestErr := http.NewRequest(http.MethodPost, server.URL+"/api/v1/auth/mfa/recovery", strings.NewReader(
				`{"challenge":"`+recoveryChallenge.MFAChallenge+`","recoveryCode":"`+enabled.RecoveryCodes[0]+`"}`))
			if requestErr != nil {
				attempts <- recoveryAttempt{err: requestErr}
				return
			}
			request.Header.Set("Content-Type", "application/json")
			response, requestErr := (&http.Client{Timeout: 5 * time.Second}).Do(request)
			if requestErr != nil {
				attempts <- recoveryAttempt{err: requestErr}
				return
			}
			_, copyErr := io.Copy(io.Discard, response.Body)
			closeErr := response.Body.Close()
			if copyErr != nil {
				attempts <- recoveryAttempt{err: copyErr}
				return
			}
			if closeErr != nil {
				attempts <- recoveryAttempt{err: closeErr}
				return
			}
			attempts <- recoveryAttempt{status: response.StatusCode}
		}()
	}
	wait.Wait()
	close(attempts)
	successes := 0
	for attempt := range attempts {
		if attempt.err != nil {
			t.Fatal(fmt.Errorf("concurrent recovery request: %w", attempt.err))
		}
		if attempt.status == http.StatusOK {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent recovery successes=%d, want 1", successes)
	}
	pendingReset := doJSON(t, &http.Client{Timeout: 5 * time.Second}, http.MethodPost, server.URL+"/api/v1/auth/login",
		`{"email":"`+email+`","password":"`+password+`"}`, "")
	if pendingReset.StatusCode != http.StatusAccepted {
		t.Fatalf("pre-reset challenge status=%d body=%s", pendingReset.StatusCode, readBody(t, pendingReset))
	}
	_ = readBody(t, pendingReset)

	user, revoked, err := authService.ResetMFAFromHost(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != email || revoked < 1 {
		t.Fatalf("host reset user=%q revoked=%d", user.Email, revoked)
	}
	status, err := authService.MFAStatus(ctx, user.ID)
	if err != nil || status.Enabled {
		t.Fatalf("MFA status after host reset=%#v err=%v", status, err)
	}
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM sessions WHERE revoked_at IS NULL`).Scan(&activeSessions); err != nil {
		t.Fatal(err)
	}
	if activeSessions != 0 {
		t.Fatalf("host reset left %d active sessions", activeSessions)
	}
	var factorRows int
	if err := store.Pool().QueryRow(ctx, `SELECT (SELECT count(*) FROM user_mfa)+(SELECT count(*) FROM user_mfa_recovery_codes)`).Scan(&factorRows); err != nil {
		t.Fatal(err)
	}
	if factorRows != 0 {
		t.Fatalf("host reset left %d MFA configuration/recovery rows", factorRows)
	}
	var openChallenges int
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM mfa_challenges WHERE consumed_at IS NULL`).Scan(&openChallenges); err != nil {
		t.Fatal(err)
	}
	if openChallenges != 0 {
		t.Fatalf("host reset left %d outstanding challenges", openChallenges)
	}
	recoveredLogin := doJSON(t, &http.Client{Timeout: 5 * time.Second}, http.MethodPost, server.URL+"/api/v1/auth/login",
		`{"email":"`+email+`","password":"`+password+`"}`, "")
	if recoveredLogin.StatusCode != http.StatusOK {
		t.Fatalf("normal password login after host reset status=%d body=%s", recoveredLogin.StatusCode, readBody(t, recoveredLogin))
	}
	_ = readBody(t, recoveredLogin)
	var resetAudits int
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='user.mfa.reset_from_host'`).Scan(&resetAudits); err != nil {
		t.Fatal(err)
	}
	if resetAudits != 1 {
		t.Fatalf("host reset audit count=%d", resetAudits)
	}
	var leakedAuditValues int
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE metadata_json::text LIKE $1 OR metadata_json::text LIKE $2`,
		"%"+enrollment.Secret+"%", "%"+enabled.RecoveryCodes[0]+"%").Scan(&leakedAuditValues); err != nil {
		t.Fatal(err)
	}
	if leakedAuditValues != 0 {
		t.Fatal("audit metadata contains MFA secret material")
	}
}

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

type mfaRepositoryFake struct {
	mu        sync.Mutex
	user      domain.User
	mfa       *domain.UserMFA
	challenge *domain.MFAChallenge
	codes     map[string]domain.MFARecoveryCode
	sessions  int
	audits    []domain.AuditEvent
	revoked   int
}

func (r *mfaRepositoryFake) HasUsers(context.Context) (bool, error) { return true, nil }
func (r *mfaRepositoryFake) CreateInitialUser(context.Context, domain.User, domain.Session, domain.AuditEvent, domain.AuditEvent) error {
	return nil
}
func (r *mfaRepositoryFake) UserByEmail(_ context.Context, email string) (domain.User, error) {
	if email != r.user.Email {
		return domain.User{}, domain.NewError(domain.ErrorNotFound, "user was not found")
	}
	return r.user, nil
}
func (r *mfaRepositoryFake) UserByID(_ context.Context, id string) (domain.User, error) {
	if id != r.user.ID {
		return domain.User{}, domain.NewError(domain.ErrorNotFound, "user was not found")
	}
	return r.user, nil
}
func (r *mfaRepositoryFake) CreateLoginSession(_ context.Context, _ domain.Session, event domain.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions++
	r.audits = append(r.audits, event)
	return nil
}
func (r *mfaRepositoryFake) AuthenticatedSession(context.Context, []byte, time.Time) (domain.Session, domain.User, error) {
	return domain.Session{}, domain.User{}, domain.NewError(domain.ErrorAuthentication, "authentication is required")
}
func (r *mfaRepositoryFake) TouchSession(context.Context, string, time.Time) error { return nil }
func (r *mfaRepositoryFake) RevokeSession(context.Context, string, time.Time, domain.AuditEvent) error {
	return nil
}
func (r *mfaRepositoryFake) RecordAuditEvent(_ context.Context, event domain.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audits = append(r.audits, event)
	return nil
}
func (r *mfaRepositoryFake) MFAEnabled(context.Context, string) (bool, error) {
	return r.mfa != nil && r.mfa.EnabledAt != nil, nil
}
func (r *mfaRepositoryFake) UserMFA(context.Context, string) (domain.UserMFA, error) {
	if r.mfa == nil {
		return domain.UserMFA{}, domain.NewError(domain.ErrorNotFound, "MFA configuration was not found")
	}
	value := *r.mfa
	value.RecoveryRemaining = 0
	for _, code := range r.codes {
		if code.UsedAt == nil {
			value.RecoveryRemaining++
		}
	}
	return value, nil
}
func (r *mfaRepositoryFake) UpsertPendingMFA(_ context.Context, value domain.UserMFA) error {
	r.mfa = &value
	return nil
}
func (r *mfaRepositoryFake) CreateMFAChallenge(_ context.Context, value domain.MFAChallenge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.challenge != nil {
		at := value.CreatedAt
		r.challenge.ConsumedAt = &at
	}
	r.challenge = &value
	return nil
}
func (r *mfaRepositoryFake) MFAChallengeByTokenHash(_ context.Context, hash []byte, now time.Time) (domain.MFAChallenge, domain.User, domain.UserMFA, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.challenge == nil || !bytes.Equal(r.challenge.TokenHash, hash) || r.challenge.ConsumedAt != nil || !r.challenge.ExpiresAt.After(now) || r.challenge.FailedAttempts >= 5 {
		return domain.MFAChallenge{}, domain.User{}, domain.UserMFA{}, domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
	}
	config, _ := r.UserMFA(context.Background(), r.user.ID)
	return *r.challenge, r.user, config, nil
}
func (r *mfaRepositoryFake) FailMFAChallenge(_ context.Context, hash []byte, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.challenge != nil && bytes.Equal(r.challenge.TokenHash, hash) && r.challenge.ConsumedAt == nil {
		r.challenge.FailedAttempts++
		if r.challenge.FailedAttempts >= 5 {
			r.challenge.ConsumedAt = &now
		}
	}
	return nil
}
func (r *mfaRepositoryFake) ConsumeMFAChallenge(_ context.Context, hash, recoveryHash []byte, now time.Time, event *domain.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.challenge == nil || !bytes.Equal(r.challenge.TokenHash, hash) || r.challenge.ConsumedAt != nil || !r.challenge.ExpiresAt.After(now) {
		return domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
	}
	if len(recoveryHash) > 0 {
		code, ok := r.codes[string(recoveryHash)]
		if !ok || code.UsedAt != nil {
			return domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
		}
		code.UsedAt = &now
		r.codes[string(recoveryHash)] = code
	}
	r.challenge.ConsumedAt = &now
	if event != nil {
		r.audits = append(r.audits, *event)
	}
	return nil
}
func (r *mfaRepositoryFake) CompleteMFAChallenge(ctx context.Context, hash, recoveryHash []byte, now time.Time, recoveryEvent *domain.AuditEvent, _ domain.Session, loginEvent domain.AuditEvent) error {
	if err := r.ConsumeMFAChallenge(ctx, hash, recoveryHash, now, recoveryEvent); err != nil {
		return err
	}
	return r.CreateLoginSession(ctx, domain.Session{}, loginEvent)
}
func (r *mfaRepositoryFake) EnableMFA(_ context.Context, _ string, _ string, codes []domain.MFARecoveryCode, now time.Time, event domain.AuditEvent) error {
	if r.mfa == nil || !r.mfa.EnrollmentExpires.After(now) {
		return domain.NewError(domain.ErrorConflict, "MFA enrollment is missing or expired")
	}
	r.mfa.EnabledAt = &now
	r.user.MFAEnabled = true
	r.codes = map[string]domain.MFARecoveryCode{}
	for _, code := range codes {
		r.codes[string(code.CodeHash)] = code
	}
	r.audits = append(r.audits, event)
	r.revoked++
	if r.challenge != nil && r.challenge.ConsumedAt == nil {
		r.challenge.ConsumedAt = &now
	}
	return nil
}
func (r *mfaRepositoryFake) ReplaceRecoveryCodes(_ context.Context, _ string, _ string, codes []domain.MFARecoveryCode, _ time.Time, event domain.AuditEvent) error {
	r.codes = map[string]domain.MFARecoveryCode{}
	for _, code := range codes {
		r.codes[string(code.CodeHash)] = code
	}
	r.audits = append(r.audits, event)
	r.revoked++
	return nil
}
func (r *mfaRepositoryFake) DisableMFA(_ context.Context, _ string, _ string, now time.Time, event domain.AuditEvent) error {
	r.mfa = nil
	r.codes = nil
	r.user.MFAEnabled = false
	r.audits = append(r.audits, event)
	r.revoked++
	if r.challenge != nil && r.challenge.ConsumedAt == nil {
		r.challenge.ConsumedAt = &now
	}
	return nil
}
func (r *mfaRepositoryFake) ResetMFAFromHost(_ context.Context, _ string, now time.Time, event domain.AuditEvent) (domain.User, int64, error) {
	revoked := r.sessions
	r.mfa = nil
	r.codes = nil
	r.user.MFAEnabled = false
	r.sessions = 0
	r.audits = append(r.audits, event)
	if r.challenge != nil && r.challenge.ConsumedAt == nil {
		r.challenge.ConsumedAt = &now
	}
	return r.user, int64(revoked), nil
}

func newMFAService(t *testing.T) (*Service, *mfaRepositoryFake, *CredentialCipher, time.Time) {
	t.Helper()
	hash, err := HashPassword("current secure password")
	if err != nil {
		t.Fatal(err)
	}
	repository := &mfaRepositoryFake{user: domain.User{
		ID: "11111111-1111-4111-8111-111111111111", Email: "operator@example.test",
		DisplayName: "Operator", PasswordHash: hash, Role: domain.RoleAdministrator, Enabled: true,
	}}
	tokens, err := NewTokenManager(bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := NewCredentialCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, tokens, time.Hour, cipher)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 26, 4, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	return service, repository, cipher, now
}

func TestMFAEnrollmentInterruptsLoginAndCompletesCanonicalSession(t *testing.T) {
	service, repository, cipher, now := newMFAService(t)
	ctx := context.Background()

	ordinary, err := service.Login(ctx, repository.user.Email, "current secure password", "request", "192.0.2.4", "test")
	if err != nil || ordinary.MFARequired || repository.sessions != 1 {
		t.Fatalf("ordinary login result=%#v sessions=%d err=%v", ordinary, repository.sessions, err)
	}
	repository.sessions = 0
	if _, err := service.StartMFAEnrollment(ctx, repository.user.ID, "wrong password"); err == nil || repository.mfa != nil {
		t.Fatal("enrollment started without current password")
	}
	enrollment, err := service.StartMFAEnrollment(ctx, repository.user.ID, "current secure password")
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.Secret == "" || enrollment.ProvisioningURI == "" || enrollment.QRCodeDataURL == "" {
		t.Fatalf("incomplete enrollment response: %#v", enrollment)
	}
	if bytes.Contains(repository.mfa.Secret.Ciphertext, []byte(enrollment.Secret)) {
		t.Fatal("TOTP secret persisted in plaintext")
	}
	decrypted, err := cipher.DecryptPayload(mfaSecretScope(repository.user.ID), repository.mfa.Secret)
	if err != nil || string(decrypted) != enrollment.Secret {
		t.Fatal("encrypted enrollment secret does not round trip")
	}
	if status, _ := service.MFAStatus(ctx, repository.user.ID); status.Enabled {
		t.Fatal("MFA enabled before TOTP verification")
	}
	code, err := totp.GenerateCode(enrollment.Secret, now)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := service.VerifyMFAEnrollment(ctx, repository.user.ID, "22222222-2222-4222-8222-222222222222", code, "request")
	if err != nil || len(recovery.RecoveryCodes) != MFARecoveryCodeCount || len(repository.codes) != MFARecoveryCodeCount {
		t.Fatalf("recovery=%#v stored=%d err=%v", recovery, len(repository.codes), err)
	}
	for _, plaintext := range recovery.RecoveryCodes {
		if _, exists := repository.codes[plaintext]; exists {
			t.Fatal("plaintext recovery code used as stored key")
		}
	}

	login, err := service.Login(ctx, repository.user.Email, "current secure password", "request", "192.0.2.4", "test")
	if err != nil || !login.MFARequired || login.MFAChallenge == "" || repository.sessions != 0 {
		t.Fatalf("MFA password stage created session: result=%#v sessions=%d err=%v", login, repository.sessions, err)
	}
	if _, err := service.CompleteMFAChallenge(ctx, login.MFAChallenge, differentTOTP(code), false, "request", "192.0.2.4", "test"); err == nil || repository.sessions != 0 {
		t.Fatal("invalid TOTP completed login")
	}
	completed, err := service.CompleteMFAChallenge(ctx, login.MFAChallenge, code, false, "request", "192.0.2.4", "test")
	if err != nil || completed.Token == "" || repository.sessions != 1 {
		t.Fatalf("valid TOTP did not create canonical session: %#v sessions=%d err=%v", completed, repository.sessions, err)
	}
	if _, err := service.CompleteMFAChallenge(ctx, login.MFAChallenge, code, false, "request", "192.0.2.4", "test"); err == nil {
		t.Fatal("consumed challenge replayed")
	}

	recoveryLogin, err := service.Login(ctx, repository.user.Email, "current secure password", "request", "192.0.2.4", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteMFAChallenge(ctx, recoveryLogin.MFAChallenge, recovery.RecoveryCodes[0], true, "request", "192.0.2.4", "test"); err != nil {
		t.Fatal(err)
	}
	reuseLogin, _ := service.Login(ctx, repository.user.Email, "current secure password", "request", "192.0.2.4", "test")
	if _, err := service.CompleteMFAChallenge(ctx, reuseLogin.MFAChallenge, recovery.RecoveryCodes[0], true, "request", "192.0.2.4", "test"); err == nil {
		t.Fatal("recovery code was reusable")
	}
}

func TestMFAChallengeExpirySkewAndPasswordFailureDisclosure(t *testing.T) {
	service, repository, _, now := newMFAService(t)
	ctx := context.Background()
	enrollment, err := service.StartMFAEnrollment(ctx, repository.user.ID, "current secure password")
	if err != nil {
		t.Fatal(err)
	}
	previousStep, _ := totp.GenerateCode(enrollment.Secret, now.Add(-30*time.Second))
	outsideSkew, _ := totp.GenerateCode(enrollment.Secret, now.Add(-60*time.Second))
	if err := service.verifyEncryptedTOTP(repository.user.ID, repository.mfa.Secret, previousStep, "skew-accepted"); err != nil {
		t.Fatalf("documented skew rejected: %v", err)
	}
	if err := service.verifyEncryptedTOTP(repository.user.ID, repository.mfa.Secret, outsideSkew, "skew-rejected"); err == nil {
		t.Fatal("outside-skew TOTP accepted")
	}
	code, _ := totp.GenerateCode(enrollment.Secret, now)
	if _, err := service.VerifyMFAEnrollment(ctx, repository.user.ID, "session", code, "request"); err != nil {
		t.Fatal(err)
	}
	login, _ := service.Login(ctx, repository.user.Email, "current secure password", "request", "192.0.2.4", "test")
	service.now = func() time.Time { return now.Add(MFAChallengeLifetime + time.Second) }
	if _, err := service.CompleteMFAChallenge(ctx, login.MFAChallenge, code, false, "request", "192.0.2.4", "test"); err == nil {
		t.Fatal("expired challenge accepted")
	}

	_, enabledFailure := service.Login(ctx, repository.user.Email, "wrong password", "request", "192.0.2.5", "test")
	_, missingFailure := service.Login(ctx, "missing@example.test", "wrong password", "request", "192.0.2.5", "test")
	var enabledDomain, missingDomain *domain.Error
	if !errors.As(enabledFailure, &enabledDomain) || !errors.As(missingFailure, &missingDomain) || enabledDomain.Kind != missingDomain.Kind || enabledDomain.Message != missingDomain.Message {
		t.Fatalf("password failure disclosed MFA status: enabled=%v missing=%v", enabledFailure, missingFailure)
	}
}

func TestMFAAttemptBoundRegenerationDisableAndHostReset(t *testing.T) {
	service, repository, _, now := newMFAService(t)
	ctx := context.Background()
	enrollment, err := service.StartMFAEnrollment(ctx, repository.user.ID, "current secure password")
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totp.GenerateCode(enrollment.Secret, now)
	recovery, err := service.VerifyMFAEnrollment(ctx, repository.user.ID, "current-session", code, "enable-request")
	if err != nil {
		t.Fatal(err)
	}
	if repository.revoked != 1 {
		t.Fatalf("enable other-session revocations=%d", repository.revoked)
	}

	bounded, err := service.Login(ctx, repository.user.Email, "current secure password", "request", "192.0.2.8", "test")
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err := service.CompleteMFAChallenge(ctx, bounded.MFAChallenge, differentTOTP(code), false, "request", "192.0.2.8", "test"); err == nil {
			t.Fatal("invalid TOTP completed bounded challenge")
		}
	}
	if repository.challenge.FailedAttempts != 5 || repository.challenge.ConsumedAt == nil || repository.sessions != 0 {
		t.Fatalf("attempt bound challenge=%#v sessions=%d", repository.challenge, repository.sessions)
	}
	if _, err := service.CompleteMFAChallenge(ctx, bounded.MFAChallenge, code, false, "request", "192.0.2.8", "test"); err == nil {
		t.Fatal("challenge succeeded after its attempt bound")
	}

	regenerated, err := service.RegenerateRecoveryCodes(ctx, repository.user.ID, "current-session", "current secure password", code, "regenerate-request")
	if err != nil || len(regenerated.RecoveryCodes) != MFARecoveryCodeCount {
		t.Fatalf("regenerated=%#v err=%v", regenerated, err)
	}
	if repository.revoked != 2 {
		t.Fatalf("regeneration other-session revocations=%d", repository.revoked)
	}
	oldLogin, _ := service.Login(ctx, repository.user.Email, "current secure password", "request", "192.0.2.9", "test")
	if _, err := service.CompleteMFAChallenge(ctx, oldLogin.MFAChallenge, recovery.RecoveryCodes[0], true, "request", "192.0.2.9", "test"); err == nil {
		t.Fatal("old recovery code remained valid after regeneration")
	}
	newLogin, _ := service.Login(ctx, repository.user.Email, "current secure password", "request", "192.0.2.10", "test")
	if _, err := service.CompleteMFAChallenge(ctx, newLogin.MFAChallenge, regenerated.RecoveryCodes[0], true, "request", "192.0.2.10", "test"); err != nil {
		t.Fatalf("regenerated recovery code failed: %v", err)
	}

	outstanding, _ := service.Login(ctx, repository.user.Email, "current secure password", "request", "192.0.2.11", "test")
	if err := service.DisableMFA(ctx, repository.user.ID, "current-session", "current secure password", differentTOTP(code), "disable-request"); err == nil {
		t.Fatal("MFA disabled without a valid TOTP")
	}
	if enabled, _ := repository.MFAEnabled(ctx, repository.user.ID); !enabled {
		t.Fatal("failed disable changed MFA state")
	}
	if err := service.DisableMFA(ctx, repository.user.ID, "current-session", "current secure password", code, "disable-request"); err != nil {
		t.Fatal(err)
	}
	if repository.revoked != 3 {
		t.Fatalf("disable other-session revocations=%d", repository.revoked)
	}
	if repository.mfa != nil || repository.codes != nil || repository.challenge.ConsumedAt == nil {
		t.Fatalf("disable did not clear factors/challenge: %#v", outstanding)
	}

	secondEnrollment, err := service.StartMFAEnrollment(ctx, repository.user.ID, "current secure password")
	if err != nil {
		t.Fatal(err)
	}
	secondCode, _ := totp.GenerateCode(secondEnrollment.Secret, now)
	if _, err := service.VerifyMFAEnrollment(ctx, repository.user.ID, "current-session", secondCode, "enable-again"); err != nil {
		t.Fatal(err)
	}
	repository.sessions = 0
	if _, err := service.createSession(ctx, repository.user, "request", "192.0.2.12", "test"); err != nil {
		t.Fatal(err)
	}
	pending, _ := service.Login(ctx, repository.user.Email, "current secure password", "request", "192.0.2.12", "test")
	user, revoked, err := service.ResetMFAFromHost(ctx, repository.user.Email)
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != repository.user.ID || revoked != 1 || repository.mfa != nil || repository.sessions != 0 || repository.challenge.ConsumedAt == nil {
		t.Fatalf("host reset user=%#v revoked=%d pending=%#v repository=%#v", user, revoked, pending, repository)
	}

	serialized, err := json.Marshal(repository.audits)
	if err != nil {
		t.Fatal(err)
	}
	secretEvidence := string(serialized)
	for _, forbidden := range append([]string{enrollment.Secret, enrollment.ProvisioningURI, secondEnrollment.Secret, secondEnrollment.ProvisioningURI}, append(recovery.RecoveryCodes, regenerated.RecoveryCodes...)...) {
		if strings.Contains(secretEvidence, forbidden) {
			t.Fatalf("audit events contain MFA secret material %q", forbidden)
		}
	}
	for _, action := range []string{"user.mfa.enabled", "user.mfa.recovery_codes_regenerated", "user.mfa.disabled", "user.mfa.reset_from_host"} {
		if !strings.Contains(secretEvidence, action) {
			t.Fatalf("missing audit action %q", action)
		}
	}
}

func TestMFARecoveryCodeConcurrentReuseHasOneWinner(t *testing.T) {
	service, repository, _, now := newMFAService(t)
	ctx := context.Background()
	enrollment, err := service.StartMFAEnrollment(ctx, repository.user.ID, "current secure password")
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totp.GenerateCode(enrollment.Secret, now)
	recovery, err := service.VerifyMFAEnrollment(ctx, repository.user.ID, "current-session", code, "request")
	if err != nil {
		t.Fatal(err)
	}
	login, err := service.Login(ctx, repository.user.Email, "current secure password", "request", "192.0.2.20", "test")
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, completeErr := service.CompleteMFAChallenge(ctx, login.MFAChallenge, recovery.RecoveryCodes[0], true, "request", "192.0.2.20", "test")
			results <- completeErr
		}()
	}
	wait.Wait()
	close(results)
	successes := 0
	for completeErr := range results {
		if completeErr == nil {
			successes++
		}
	}
	if successes != 1 || repository.sessions != 1 {
		t.Fatalf("concurrent successes=%d sessions=%d", successes, repository.sessions)
	}
}

func TestMFAEnrollmentVerificationIsRateLimitedPerUser(t *testing.T) {
	service, repository, _, now := newMFAService(t)
	ctx := context.Background()
	enrollment, err := service.StartMFAEnrollment(ctx, repository.user.ID, "current secure password")
	if err != nil {
		t.Fatal(err)
	}
	valid, _ := totp.GenerateCode(enrollment.Secret, now)
	for range 5 {
		if _, err := service.VerifyMFAEnrollment(ctx, repository.user.ID, "session", differentTOTP(valid), "request"); err == nil {
			t.Fatal("invalid enrollment code succeeded")
		}
	}
	if _, err := service.VerifyMFAEnrollment(ctx, repository.user.ID, "session", valid, "request"); err == nil {
		t.Fatal("enrollment verification bypassed its rate limit")
	} else {
		var rateLimit *domain.Error
		if !errors.As(err, &rateLimit) || rateLimit.Kind != domain.ErrorRateLimited {
			t.Fatalf("post-bound enrollment error=%v, want rate limit", err)
		}
	}
	if enabled, _ := repository.MFAEnabled(ctx, repository.user.ID); enabled {
		t.Fatal("rate-limited enrollment enabled MFA")
	}
}

func differentTOTP(code string) string {
	if code == "000000" {
		return "999999"
	}
	return "000000"
}

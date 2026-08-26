package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

const (
	MFATOTPIssuer           = "Atlas DNS"
	MFATOTPPeriod           = 30
	MFATOTPSkew             = 1
	MFAChallengeLifetime    = 5 * time.Minute
	MFAEnrollmentLifetime   = 10 * time.Minute
	MFARecoveryCodeCount    = 10
	recoveryCodeRandomBytes = 10
)

var mfaValidateOptions = totp.ValidateOpts{
	Period: MFATOTPPeriod, Skew: MFATOTPSkew, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
}

type MFAStatus struct {
	Enabled           bool `json:"enabled"`
	RecoveryRemaining int  `json:"recoveryCodesRemaining"`
}

type MFAEnrollment struct {
	Secret          string    `json:"secret"`
	ProvisioningURI string    `json:"provisioningUri"`
	QRCodeDataURL   string    `json:"qrCodeDataUrl"`
	Issuer          string    `json:"issuer"`
	AccountLabel    string    `json:"accountLabel"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

type MFARecoveryResult struct {
	RecoveryCodes     []string `json:"recoveryCodes"`
	RecoveryRemaining int      `json:"recoveryCodesRemaining"`
}

type MFARepository interface {
	UserByID(context.Context, string) (domain.User, error)
	MFAEnabled(context.Context, string) (bool, error)
	UserMFA(context.Context, string) (domain.UserMFA, error)
	UpsertPendingMFA(context.Context, domain.UserMFA) error
	CreateMFAChallenge(context.Context, domain.MFAChallenge) error
	MFAChallengeByTokenHash(context.Context, []byte, time.Time) (domain.MFAChallenge, domain.User, domain.UserMFA, error)
	FailMFAChallenge(context.Context, []byte, time.Time) error
	ConsumeMFAChallenge(context.Context, []byte, []byte, time.Time, *domain.AuditEvent) error
	CompleteMFAChallenge(context.Context, []byte, []byte, time.Time, *domain.AuditEvent, domain.Session, domain.AuditEvent) error
	EnableMFA(context.Context, string, string, []domain.MFARecoveryCode, time.Time, domain.AuditEvent) error
	ReplaceRecoveryCodes(context.Context, string, string, []domain.MFARecoveryCode, time.Time, domain.AuditEvent) error
	DisableMFA(context.Context, string, string, time.Time, domain.AuditEvent) error
	ResetMFAFromHost(context.Context, string, time.Time, domain.AuditEvent) (domain.User, int64, error)
}

func (s *Service) MFAStatus(ctx context.Context, userID string) (MFAStatus, error) {
	if s.mfaRepository == nil {
		return MFAStatus{}, fmt.Errorf("MFA repository is unavailable")
	}
	enabled, err := s.mfaRepository.MFAEnabled(ctx, userID)
	if err != nil || !enabled {
		return MFAStatus{Enabled: enabled}, err
	}
	config, err := s.mfaRepository.UserMFA(ctx, userID)
	if err != nil {
		return MFAStatus{}, err
	}
	return MFAStatus{Enabled: true, RecoveryRemaining: config.RecoveryRemaining}, nil
}

func (s *Service) StartMFAEnrollment(ctx context.Context, userID, currentPassword string) (MFAEnrollment, error) {
	if s.mfaRepository == nil {
		return MFAEnrollment{}, fmt.Errorf("MFA repository is unavailable")
	}
	if s.credentialCipher == nil {
		return MFAEnrollment{}, fmt.Errorf("MFA credential encryption is unavailable")
	}
	user, err := s.verifyCurrentPassword(ctx, userID, currentPassword, "mfa-enrollment-password")
	if err != nil {
		return MFAEnrollment{}, err
	}
	if user.MFAEnabled {
		return MFAEnrollment{}, domain.NewError(domain.ErrorConflict, "MFA is already enabled")
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer: MFATOTPIssuer, AccountName: user.Email, Period: MFATOTPPeriod,
		SecretSize: 20, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1, Rand: rand.Reader,
	})
	if err != nil {
		return MFAEnrollment{}, fmt.Errorf("generate TOTP enrollment: %w", err)
	}
	encrypted, err := s.credentialCipher.EncryptPayload(mfaSecretScope(user.ID), []byte(key.Secret()))
	if err != nil {
		return MFAEnrollment{}, fmt.Errorf("encrypt TOTP secret: %w", err)
	}
	now := s.now().UTC()
	expires := now.Add(MFAEnrollmentLifetime)
	if err := s.mfaRepository.UpsertPendingMFA(ctx, domain.UserMFA{
		UserID: user.ID, Secret: encrypted, EnrollmentStarted: now, EnrollmentExpires: expires,
	}); err != nil {
		return MFAEnrollment{}, err
	}
	image, err := key.Image(256, 256)
	if err != nil {
		return MFAEnrollment{}, fmt.Errorf("generate local enrollment QR code: %w", err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image); err != nil {
		return MFAEnrollment{}, fmt.Errorf("encode local enrollment QR code: %w", err)
	}
	return MFAEnrollment{
		Secret: key.Secret(), ProvisioningURI: key.URL(),
		QRCodeDataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()),
		Issuer:        MFATOTPIssuer, AccountLabel: user.Email, ExpiresAt: expires,
	}, nil
}

func (s *Service) VerifyMFAEnrollment(ctx context.Context, userID, currentSessionID, code, requestID string) (MFARecoveryResult, error) {
	if s.mfaRepository == nil {
		return MFARecoveryResult{}, fmt.Errorf("MFA repository is unavailable")
	}
	config, err := s.mfaRepository.UserMFA(ctx, userID)
	if err != nil {
		return MFARecoveryResult{}, domain.NewError(domain.ErrorConflict, "MFA enrollment is missing or expired")
	}
	if config.EnabledAt != nil {
		return MFARecoveryResult{}, domain.NewError(domain.ErrorConflict, "MFA is already enabled")
	}
	if !config.EnrollmentExpires.After(s.now().UTC()) {
		return MFARecoveryResult{}, domain.NewError(domain.ErrorConflict, "MFA enrollment is missing or expired")
	}
	enrollmentLimitKey := userID + "\x00mfa-enrollment-code"
	if err := s.verifyEncryptedTOTP(userID, config.Secret, code, enrollmentLimitKey); err != nil {
		return MFARecoveryResult{}, err
	}
	plain, stored, err := s.newRecoveryCodes(userID)
	if err != nil {
		return MFARecoveryResult{}, err
	}
	now := s.now().UTC()
	event, err := auditEvent("user", &userID, "user.mfa.enabled", "user", &userID, requestID, map[string]any{
		"recoveryCodesRemaining": MFARecoveryCodeCount, "otherSessionsRevoked": true,
	}, now)
	if err != nil {
		return MFARecoveryResult{}, err
	}
	if err := s.mfaRepository.EnableMFA(ctx, userID, currentSessionID, stored, now, event); err != nil {
		return MFARecoveryResult{}, err
	}
	s.factorLimiter.Success(enrollmentLimitKey)
	return MFARecoveryResult{RecoveryCodes: plain, RecoveryRemaining: len(plain)}, nil
}

func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID, currentSessionID, currentPassword, code, requestID string) (MFARecoveryResult, error) {
	if s.mfaRepository == nil {
		return MFARecoveryResult{}, fmt.Errorf("MFA repository is unavailable")
	}
	if _, _, err := s.verifyMFAStepUp(ctx, userID, currentPassword, code, "mfa-regenerate"); err != nil {
		return MFARecoveryResult{}, err
	}
	plain, stored, err := s.newRecoveryCodes(userID)
	if err != nil {
		return MFARecoveryResult{}, err
	}
	now := s.now().UTC()
	event, err := auditEvent("user", &userID, "user.mfa.recovery_codes_regenerated", "user", &userID, requestID, map[string]any{
		"recoveryCodesRemaining": MFARecoveryCodeCount, "otherSessionsRevoked": true,
	}, now)
	if err != nil {
		return MFARecoveryResult{}, err
	}
	if err := s.mfaRepository.ReplaceRecoveryCodes(ctx, userID, currentSessionID, stored, now, event); err != nil {
		return MFARecoveryResult{}, err
	}
	return MFARecoveryResult{RecoveryCodes: plain, RecoveryRemaining: len(plain)}, nil
}

func (s *Service) DisableMFA(ctx context.Context, userID, currentSessionID, currentPassword, code, requestID string) error {
	if s.mfaRepository == nil {
		return fmt.Errorf("MFA repository is unavailable")
	}
	if _, _, err := s.verifyMFAStepUp(ctx, userID, currentPassword, code, "mfa-disable"); err != nil {
		return err
	}
	now := s.now().UTC()
	event, err := auditEvent("user", &userID, "user.mfa.disabled", "user", &userID, requestID, map[string]any{
		"recoveryCodesInvalidated": true, "challengesInvalidated": true, "otherSessionsRevoked": true,
	}, now)
	if err != nil {
		return err
	}
	return s.mfaRepository.DisableMFA(ctx, userID, currentSessionID, now, event)
}

func (s *Service) VerifyTOTPForUser(ctx context.Context, userID, code, operation string) error {
	if s.mfaRepository == nil {
		return fmt.Errorf("MFA repository is unavailable")
	}
	config, err := s.mfaRepository.UserMFA(ctx, userID)
	if err != nil || config.EnabledAt == nil {
		return domain.NewError(domain.ErrorInvalidCredentials, "current password or authentication code is invalid")
	}
	return s.verifyEncryptedTOTP(userID, config.Secret, code, userID+"\x00"+operation)
}

func (s *Service) RequiresMFA(ctx context.Context, userID string) (bool, error) {
	if s.mfaRepository == nil {
		return false, nil
	}
	return s.mfaRepository.MFAEnabled(ctx, userID)
}

func (s *Service) CompleteMFAChallenge(ctx context.Context, challengeToken, code string, recovery bool, requestID, sourceIP, userAgent string) (SessionResult, error) {
	if s.mfaRepository == nil {
		return SessionResult{}, fmt.Errorf("MFA repository is unavailable")
	}
	tokenHash := s.tokens.HashMFAChallengeToken(strings.TrimSpace(challengeToken))
	challenge, user, config, err := s.mfaRepository.MFAChallengeByTokenHash(ctx, tokenHash, s.now().UTC())
	if err != nil {
		return SessionResult{}, err
	}
	limitKey := sourceIP + "\x00" + challenge.ID
	if !s.factorLimiter.Allow(limitKey) {
		return SessionResult{}, domain.NewError(domain.ErrorRateLimited, "too many authentication-code attempts; try again later")
	}
	var recoveryHash []byte
	var recoveryEvent *domain.AuditEvent
	if recovery {
		normalized, valid := normalizeRecoveryCode(code)
		if valid {
			recoveryHash = s.tokens.HashRecoveryCode(normalized)
			now := s.now().UTC()
			event, eventErr := auditEvent("user", &user.ID, "user.mfa.recovery_code_used", "user", &user.ID, requestID, map[string]any{
				"recoveryCodesRemaining": max(config.RecoveryRemaining-1, 0),
			}, now)
			if eventErr != nil {
				return SessionResult{}, eventErr
			}
			recoveryEvent = &event
		}
	} else if err := s.verifyEncryptedTOTP(user.ID, config.Secret, code, limitKey); err != nil {
		_ = s.mfaRepository.FailMFAChallenge(ctx, tokenHash, s.now().UTC())
		_ = s.recordFailure(ctx, requestID, sourceIP, user.Email, "invalid_second_factor")
		return SessionResult{}, domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
	}
	if recovery && len(recoveryHash) == 0 {
		_ = s.mfaRepository.FailMFAChallenge(ctx, tokenHash, s.now().UTC())
		s.factorLimiter.Failure(limitKey)
		_ = s.recordFailure(ctx, requestID, sourceIP, user.Email, "invalid_second_factor")
		return SessionResult{}, domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
	}
	result, loginEvent, err := s.buildSession(user, requestID, sourceIP, userAgent)
	if err != nil {
		return SessionResult{}, err
	}
	if err := s.mfaRepository.CompleteMFAChallenge(ctx, tokenHash, recoveryHash, s.now().UTC(), recoveryEvent, result.Session, loginEvent); err != nil {
		_ = s.mfaRepository.FailMFAChallenge(ctx, tokenHash, s.now().UTC())
		s.factorLimiter.Failure(limitKey)
		var domainError *domain.Error
		if !errors.As(err, &domainError) || (domainError.Kind != domain.ErrorInvalidCredentials && domainError.Kind != domain.ErrorAuthentication) {
			return SessionResult{}, err
		}
		_ = s.recordFailure(ctx, requestID, sourceIP, user.Email, "invalid_second_factor")
		return SessionResult{}, domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
	}
	s.factorLimiter.Success(limitKey)
	return result, nil
}

func (s *Service) CancelMFAChallenge(ctx context.Context, challengeToken string) error {
	if s.mfaRepository == nil {
		return fmt.Errorf("MFA repository is unavailable")
	}
	return s.mfaRepository.ConsumeMFAChallenge(ctx, s.tokens.HashMFAChallengeToken(strings.TrimSpace(challengeToken)), nil, s.now().UTC(), nil)
}

func (s *Service) ResetMFAFromHost(ctx context.Context, email string) (domain.User, int64, error) {
	if s.mfaRepository == nil {
		return domain.User{}, 0, fmt.Errorf("MFA repository is unavailable")
	}
	normalized, err := domain.NormaliseEmail(email)
	if err != nil {
		return domain.User{}, 0, err
	}
	now := s.now().UTC()
	event, err := auditEvent("system", nil, "user.mfa.reset_from_host", "user", nil, "admin-cli", map[string]any{
		"sessionsRevoked": true, "recoveryCodesInvalidated": true, "challengesInvalidated": true,
	}, now)
	if err != nil {
		return domain.User{}, 0, err
	}
	return s.mfaRepository.ResetMFAFromHost(ctx, normalized, now, event)
}

func (s *Service) createMFAChallenge(ctx context.Context, user domain.User, sourceIP, userAgent string) (SessionResult, error) {
	id, err := domain.NewID()
	if err != nil {
		return SessionResult{}, err
	}
	token, tokenHash, err := s.tokens.NewMFAChallengeToken()
	if err != nil {
		return SessionResult{}, err
	}
	now := s.now().UTC()
	expires := now.Add(MFAChallengeLifetime)
	if s.mfaRepository == nil {
		return SessionResult{}, fmt.Errorf("MFA repository is unavailable")
	}
	if err := s.mfaRepository.CreateMFAChallenge(ctx, domain.MFAChallenge{
		ID: id, UserID: user.ID, TokenHash: tokenHash, CreatedAt: now, ExpiresAt: expires,
		IPMetadata: truncate(sourceIP, 128), UserAgent: truncate(userAgent, 512),
	}); err != nil {
		return SessionResult{}, err
	}
	return SessionResult{MFARequired: true, MFAChallenge: token, ChallengeExpiresAt: expires}, nil
}

func (s *Service) verifyCurrentPassword(ctx context.Context, userID, password, operation string) (domain.User, error) {
	key := userID + "\x00" + operation
	if !s.factorLimiter.Allow(key) {
		return domain.User{}, domain.NewError(domain.ErrorRateLimited, "too many authentication attempts; try again later")
	}
	user, err := s.mfaRepository.UserByID(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	valid, err := VerifyPassword(user.PasswordHash, password)
	if err != nil {
		return domain.User{}, fmt.Errorf("verify current password: %w", err)
	}
	if !valid {
		s.factorLimiter.Failure(key)
		return domain.User{}, domain.NewError(domain.ErrorInvalidCredentials, "current password is incorrect")
	}
	s.factorLimiter.Success(key)
	return user, nil
}

func (s *Service) verifyMFAStepUp(ctx context.Context, userID, password, code, operation string) (domain.User, domain.UserMFA, error) {
	user, err := s.verifyCurrentPassword(ctx, userID, password, operation+"-password")
	if err != nil {
		return domain.User{}, domain.UserMFA{}, err
	}
	if !user.MFAEnabled {
		return domain.User{}, domain.UserMFA{}, domain.NewError(domain.ErrorConflict, "MFA is not enabled")
	}
	config, err := s.mfaRepository.UserMFA(ctx, userID)
	if err != nil {
		return domain.User{}, domain.UserMFA{}, err
	}
	if err := s.verifyEncryptedTOTP(userID, config.Secret, code, userID+"\x00"+operation+"-code"); err != nil {
		return domain.User{}, domain.UserMFA{}, err
	}
	return user, config, nil
}

func (s *Service) verifyEncryptedTOTP(userID string, encrypted domain.EncryptedPayload, code, limitKey string) error {
	if s.credentialCipher == nil {
		return fmt.Errorf("MFA credential encryption is unavailable")
	}
	if !s.factorLimiter.Allow(limitKey) {
		return domain.NewError(domain.ErrorRateLimited, "too many authentication-code attempts; try again later")
	}
	secret, err := s.credentialCipher.DecryptPayload(mfaSecretScope(userID), encrypted)
	if err != nil {
		return fmt.Errorf("decrypt TOTP secret: %w", err)
	}
	valid, err := totp.ValidateCustom(strings.TrimSpace(code), string(secret), s.now().UTC(), mfaValidateOptions)
	for index := range secret {
		secret[index] = 0
	}
	if err != nil {
		return fmt.Errorf("validate TOTP: %w", err)
	}
	if !valid {
		s.factorLimiter.Failure(limitKey)
		return domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid")
	}
	s.factorLimiter.Success(limitKey)
	return nil
}

func (s *Service) newRecoveryCodes(userID string) ([]string, []domain.MFARecoveryCode, error) {
	plain := make([]string, 0, MFARecoveryCodeCount)
	stored := make([]domain.MFARecoveryCode, 0, MFARecoveryCodeCount)
	now := s.now().UTC()
	for range MFARecoveryCodeCount {
		raw := make([]byte, recoveryCodeRandomBytes)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, fmt.Errorf("generate MFA recovery code: %w", err)
		}
		normalized := hex.EncodeToString(raw)
		id, err := domain.NewID()
		if err != nil {
			return nil, nil, err
		}
		plain = append(plain, formatRecoveryCode(normalized))
		stored = append(stored, domain.MFARecoveryCode{ID: id, UserID: userID, CodeHash: s.tokens.HashRecoveryCode(normalized), CreatedAt: now})
	}
	return plain, stored, nil
}

func normalizeRecoveryCode(value string) (string, bool) {
	value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
	if len(value) != recoveryCodeRandomBytes*2 {
		return "", false
	}
	_, err := hex.DecodeString(value)
	return value, err == nil
}

func formatRecoveryCode(value string) string {
	return value[0:5] + "-" + value[5:10] + "-" + value[10:15] + "-" + value[15:20]
}

func mfaSecretScope(userID string) string { return "user-mfa:" + userID }

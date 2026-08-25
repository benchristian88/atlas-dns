package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

type Repository interface {
	HasUsers(context.Context) (bool, error)
	CreateInitialUser(context.Context, domain.User, domain.Session, domain.AuditEvent, domain.AuditEvent) error
	UserByEmail(context.Context, string) (domain.User, error)
	CreateLoginSession(context.Context, domain.Session, domain.AuditEvent) error
	AuthenticatedSession(context.Context, []byte, time.Time) (domain.Session, domain.User, error)
	TouchSession(context.Context, string, time.Time) error
	RevokeSession(context.Context, string, time.Time, domain.AuditEvent) error
	RecordAuditEvent(context.Context, domain.AuditEvent) error
}

type Service struct {
	repository      Repository
	tokens          *TokenManager
	limiter         *LoginLimiter
	sessionDuration time.Duration
	runtime         interface {
		RuntimeSettings() systemsettings.RuntimeSettings
	}
	dummyHash string
	now       func() time.Time
}

func (s *Service) SetRuntimeSettings(provider interface {
	RuntimeSettings() systemsettings.RuntimeSettings
}) {
	s.runtime = provider
}

func (s *Service) currentSessionDuration() time.Duration {
	if s.runtime != nil {
		return s.runtime.RuntimeSettings().SessionDuration
	}
	return s.sessionDuration
}

type SessionResult struct {
	User      domain.User
	Session   domain.Session
	Token     string
	CSRFToken string
}

func NewService(repository Repository, tokens *TokenManager, sessionDuration time.Duration) (*Service, error) {
	dummyHash, err := HashPassword("not-a-real-password-value")
	if err != nil {
		return nil, fmt.Errorf("create authentication timing hash: %w", err)
	}
	return &Service{
		repository: repository, tokens: tokens, limiter: NewLoginLimiter(5, 15*time.Minute),
		sessionDuration: sessionDuration, dummyHash: dummyHash, now: time.Now,
	}, nil
}

func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	hasUsers, err := s.repository.HasUsers(ctx)
	return !hasUsers, err
}

func (s *Service) Setup(ctx context.Context, email, displayName, password, requestID, sourceIP, userAgent string) (SessionResult, error) {
	required, err := s.SetupRequired(ctx)
	if err != nil {
		return SessionResult{}, err
	}
	if !required {
		return SessionResult{}, domain.NewError(domain.ErrorConflict, "initial setup has already been completed")
	}
	normalisedEmail, err := domain.NormaliseEmail(email)
	if err != nil {
		return SessionResult{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if err := domain.ValidateDisplayName(displayName); err != nil {
		return SessionResult{}, err
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return SessionResult{}, err
	}
	userID, err := domain.NewID()
	if err != nil {
		return SessionResult{}, err
	}
	now := s.now().UTC()
	user := domain.User{
		ID: userID, Email: normalisedEmail, DisplayName: displayName, PasswordHash: passwordHash,
		Role: domain.RoleAdministrator, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	userEvent, err := auditEvent("user", &userID, "user.created", "user", &userID, requestID, map[string]any{"role": user.Role}, now)
	if err != nil {
		return SessionResult{}, err
	}
	result, loginEvent, err := s.buildSession(user, requestID, sourceIP, userAgent)
	if err != nil {
		return SessionResult{}, err
	}
	if err := s.repository.CreateInitialUser(ctx, user, result.Session, userEvent, loginEvent); err != nil {
		return SessionResult{}, err
	}
	return result, nil
}

func (s *Service) Login(ctx context.Context, email, password, requestID, sourceIP, userAgent string) (SessionResult, error) {
	normalisedEmail, normaliseErr := domain.NormaliseEmail(email)
	if normaliseErr != nil {
		normalisedEmail = strings.ToLower(strings.TrimSpace(email))
	}
	key := sourceIP + "\x00" + normalisedEmail
	if !s.limiter.Allow(key) {
		if err := s.recordFailure(ctx, requestID, sourceIP, normalisedEmail, "rate_limited"); err != nil {
			return SessionResult{}, err
		}
		return SessionResult{}, domain.NewError(domain.ErrorRateLimited, "too many login attempts; try again later")
	}
	user, err := s.repository.UserByEmail(ctx, normalisedEmail)
	encoded := s.dummyHash
	if err == nil {
		encoded = user.PasswordHash
	}
	valid, verifyErr := VerifyPassword(encoded, password)
	if verifyErr != nil {
		return SessionResult{}, fmt.Errorf("verify password: %w", verifyErr)
	}
	if err != nil || !valid || !user.Enabled {
		s.limiter.Failure(key)
		if auditErr := s.recordFailure(ctx, requestID, sourceIP, normalisedEmail, "invalid_credentials"); auditErr != nil {
			return SessionResult{}, auditErr
		}
		return SessionResult{}, domain.NewError(domain.ErrorInvalidCredentials, "email or password is incorrect")
	}
	s.limiter.Success(key)
	return s.createSession(ctx, user, requestID, sourceIP, userAgent)
}

func (s *Service) Authenticate(ctx context.Context, token string) (domain.Session, domain.User, error) {
	if token == "" {
		return domain.Session{}, domain.User{}, domain.NewError(domain.ErrorAuthentication, "authentication is required")
	}
	session, user, err := s.repository.AuthenticatedSession(ctx, s.tokens.HashSessionToken(token), s.now().UTC())
	if err != nil {
		return domain.Session{}, domain.User{}, err
	}
	_ = s.repository.TouchSession(ctx, session.ID, s.now().UTC())
	return session, user, nil
}

func (s *Service) ValidateCSRF(session domain.Session, token string) bool {
	if token == "" {
		return false
	}
	return s.tokens.Equal(session.CSRFHash, s.tokens.HashCSRFToken(token))
}

func (s *Service) Logout(ctx context.Context, session domain.Session, user domain.User, requestID string) error {
	now := s.now().UTC()
	event, err := auditEvent("user", &user.ID, "auth.logout", "session", &session.ID, requestID, map[string]any{}, now)
	if err != nil {
		return err
	}
	return s.repository.RevokeSession(ctx, session.ID, now, event)
}

func (s *Service) createSession(ctx context.Context, user domain.User, requestID, sourceIP, userAgent string) (SessionResult, error) {
	result, event, err := s.buildSession(user, requestID, sourceIP, userAgent)
	if err != nil {
		return SessionResult{}, err
	}
	if err := s.repository.CreateLoginSession(ctx, result.Session, event); err != nil {
		return SessionResult{}, err
	}
	return result, nil
}

func (s *Service) buildSession(user domain.User, requestID, sourceIP, userAgent string) (SessionResult, domain.AuditEvent, error) {
	sessionID, err := domain.NewID()
	if err != nil {
		return SessionResult{}, domain.AuditEvent{}, err
	}
	token, tokenHash, err := s.tokens.NewSessionToken()
	if err != nil {
		return SessionResult{}, domain.AuditEvent{}, err
	}
	csrfToken, csrfHash, err := s.tokens.NewCSRFToken()
	if err != nil {
		return SessionResult{}, domain.AuditEvent{}, err
	}
	now := s.now().UTC()
	session := domain.Session{
		ID: sessionID, UserID: user.ID, TokenHash: tokenHash, CSRFHash: csrfHash,
		CreatedAt: now, ExpiresAt: now.Add(s.currentSessionDuration()), LastSeenAt: now,
		IPMetadata: truncate(sourceIP, 128), UserAgent: truncate(userAgent, 512),
	}
	event, err := auditEvent("user", &user.ID, "auth.login.succeeded", "session", &sessionID, requestID, map[string]any{"sourceIp": session.IPMetadata}, now)
	if err != nil {
		return SessionResult{}, domain.AuditEvent{}, err
	}
	return SessionResult{User: user, Session: session, Token: token, CSRFToken: csrfToken}, event, nil
}

func (s *Service) recordFailure(ctx context.Context, requestID, sourceIP, email, outcome string) error {
	now := s.now().UTC()
	digest := sha256.Sum256([]byte(email))
	event, err := auditEvent("anonymous", nil, "auth.login.failed", "authentication", nil, requestID, map[string]any{
		"sourceIp": truncate(sourceIP, 128), "identifierHash": hex.EncodeToString(digest[:]), "outcome": outcome,
	}, now)
	if err != nil {
		return err
	}
	return s.repository.RecordAuditEvent(ctx, event)
}

func auditEvent(actorType string, actorUserID *string, action, resourceType string, resourceID *string, requestID string, metadata map[string]any, at time.Time) (domain.AuditEvent, error) {
	id, err := domain.NewID()
	if err != nil {
		return domain.AuditEvent{}, err
	}
	return domain.AuditEvent{
		ID: id, ActorType: actorType, ActorUserID: actorUserID, Action: action,
		ResourceType: resourceType, ResourceID: resourceID, RequestID: requestID,
		Metadata: metadata, CreatedAt: at,
	}, nil
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func IsAuthenticationError(err error) bool {
	var domainError *domain.Error
	return errors.As(err, &domainError) && domainError.Kind == domain.ErrorAuthentication
}

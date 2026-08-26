package useradmin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/domain"
)

type Repository interface {
	ListUsers(context.Context) ([]domain.User, error)
	UserByID(context.Context, string) (domain.User, error)
	CreateUser(context.Context, domain.User, domain.AuditEvent) error
	UpdateUser(context.Context, string, string, string, bool, time.Time, domain.AuditEvent) (domain.User, error)
	ResetUserPassword(context.Context, string, string, time.Time, domain.AuditEvent) error
	ChangeOwnPassword(context.Context, string, string, string, time.Time, domain.AuditEvent) error
}

type Service struct {
	repository      Repository
	passwordLimiter *auth.LoginLimiter
	mfa             SecondFactorVerifier
	now             func() time.Time
}

type SecondFactorVerifier interface {
	RequiresMFA(context.Context, string) (bool, error)
	VerifyTOTPForUser(context.Context, string, string, string) error
}

type CreateInput struct {
	Email       string
	DisplayName string
	Password    string
	Role        domain.UserRole
}

type UpdateInput struct {
	Email       string
	DisplayName string
	Role        domain.UserRole
	Enabled     bool
}

func NewService(repository Repository, verifiers ...SecondFactorVerifier) *Service {
	service := &Service{
		repository:      repository,
		passwordLimiter: auth.NewLoginLimiter(5, 15*time.Minute),
		now:             time.Now,
	}
	if len(verifiers) > 0 {
		service.mfa = verifiers[0]
	}
	return service
}

func (s *Service) List(ctx context.Context) ([]domain.User, error) {
	return s.repository.ListUsers(ctx)
}

func (s *Service) Create(ctx context.Context, actor domain.Actor, input CreateInput) (domain.User, error) {
	email, err := domain.NormaliseEmail(input.Email)
	if err != nil {
		return domain.User{}, err
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if err := domain.ValidateDisplayName(displayName); err != nil {
		return domain.User{}, err
	}
	if input.Role != domain.RoleAdministrator {
		return domain.User{}, domain.Validation("role", "must be administrator")
	}
	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		return domain.User{}, err
	}
	id, err := domain.NewID()
	if err != nil {
		return domain.User{}, err
	}
	now := s.now().UTC()
	user := domain.User{ID: id, Email: email, DisplayName: displayName, PasswordHash: passwordHash, Role: input.Role, Enabled: true, CreatedAt: now, UpdatedAt: now}
	event, err := event(actor, "user.created", id, map[string]any{"role": input.Role}, now)
	if err != nil {
		return domain.User{}, err
	}
	if err := s.repository.CreateUser(ctx, user, event); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Service) Update(ctx context.Context, actor domain.Actor, targetID string, input UpdateInput) (domain.User, error) {
	if !domain.ValidID(targetID) {
		return domain.User{}, domain.Validation("userId", "must be a valid identifier")
	}
	email, err := domain.NormaliseEmail(input.Email)
	if err != nil {
		return domain.User{}, err
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if err := domain.ValidateDisplayName(displayName); err != nil {
		return domain.User{}, err
	}
	if input.Role != domain.RoleAdministrator {
		return domain.User{}, domain.Validation("role", "must be administrator")
	}
	if actor.UserID == targetID && !input.Enabled {
		return domain.User{}, domain.NewError(domain.ErrorConflict, "you cannot disable your current account")
	}
	current, err := s.repository.UserByID(ctx, targetID)
	if err != nil {
		return domain.User{}, err
	}
	now := s.now().UTC()
	action := "user.updated"
	if current.Enabled != input.Enabled {
		if input.Enabled {
			action = "user.enabled"
		} else {
			action = "user.disabled"
		}
	} else if current.Email != email {
		action = "user.login_identifier_changed"
	}
	e, err := event(actor, action, targetID, map[string]any{
		"role": input.Role, "enabled": input.Enabled,
		"loginIdentifierChanged": current.Email != email,
		"displayNameChanged":     current.DisplayName != displayName,
	}, now)
	if err != nil {
		return domain.User{}, err
	}
	return s.repository.UpdateUser(ctx, targetID, email, displayName, input.Enabled, now, e)
}

func (s *Service) ResetPassword(ctx context.Context, actor domain.Actor, targetID, password string) error {
	if !domain.ValidID(targetID) {
		return domain.Validation("userId", "must be a valid identifier")
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	e, err := event(actor, "user.password_reset", targetID, map[string]any{"sessionsRevoked": true}, now)
	if err != nil {
		return err
	}
	return s.repository.ResetUserPassword(ctx, targetID, passwordHash, now, e)
}

func (s *Service) ChangeOwnPassword(ctx context.Context, actor domain.Actor, currentSessionID, currentPassword, newPassword, totpCode string) error {
	if !domain.ValidID(actor.UserID) {
		return domain.NewError(domain.ErrorAuthentication, "authentication is required")
	}
	if !domain.ValidID(currentSessionID) {
		return domain.NewError(domain.ErrorAuthentication, "authentication is required")
	}
	if !s.passwordLimiter.Allow(actor.UserID) {
		return domain.NewError(domain.ErrorRateLimited, "too many password attempts; try again later")
	}
	user, err := s.repository.UserByID(ctx, actor.UserID)
	if err != nil {
		return err
	}
	valid, err := auth.VerifyPassword(user.PasswordHash, currentPassword)
	if err != nil {
		return fmt.Errorf("verify current password: %w", err)
	}
	if !valid {
		s.passwordLimiter.Failure(actor.UserID)
		return domain.NewError(domain.ErrorInvalidCredentials, "current password is incorrect")
	}
	if s.mfa != nil {
		required, err := s.mfa.RequiresMFA(ctx, actor.UserID)
		if err != nil {
			return err
		}
		if required {
			if err := s.mfa.VerifyTOTPForUser(ctx, actor.UserID, totpCode, "password-change-code"); err != nil {
				return err
			}
		}
	}
	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	e, err := event(actor, "user.password_changed", actor.UserID, map[string]any{"otherSessionsRevoked": true}, now)
	if err != nil {
		return err
	}
	if err := s.repository.ChangeOwnPassword(ctx, actor.UserID, currentSessionID, passwordHash, now, e); err != nil {
		return err
	}
	s.passwordLimiter.Success(actor.UserID)
	return nil
}

func event(actor domain.Actor, action, targetID string, metadata map[string]any, now time.Time) (domain.AuditEvent, error) {
	id, err := domain.NewID()
	if err != nil {
		return domain.AuditEvent{}, fmt.Errorf("create user audit identifier: %w", err)
	}
	actorID := actor.UserID
	return domain.AuditEvent{ID: id, ActorType: "user", ActorUserID: &actorID, Action: action, ResourceType: "user", ResourceID: &targetID, RequestID: actor.RequestID, Metadata: metadata, CreatedAt: now}, nil
}

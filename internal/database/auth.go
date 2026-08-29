package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

func (s *Store) HasUsers(ctx context.Context) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users)").Scan(&exists); err != nil {
		return false, fmt.Errorf("check initial setup state: %w", err)
	}
	return exists, nil
}

func (s *Store) CreateInitialUser(ctx context.Context, user domain.User, session domain.Session, userEvent, loginEvent domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin initial user transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, "LOCK TABLE users IN EXCLUSIVE MODE"); err != nil {
		return fmt.Errorf("lock initial user setup: %w", err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users)").Scan(&exists); err != nil {
		return fmt.Errorf("check initial user: %w", err)
	}
	if exists {
		return domain.NewError(domain.ErrorConflict, "initial setup has already been completed")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO users
			(id, email, display_name, password_hash, role, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`,
		user.ID, user.Email, user.DisplayName, user.PasswordHash, user.Role, user.Enabled, user.CreatedAt)
	if err != nil {
		return fmt.Errorf("create initial administrator: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO sessions
			(id, user_id, token_hash, csrf_hash, created_at, expires_at, last_seen_at, ip_metadata, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $5, $7, $8)`,
		session.ID, session.UserID, session.TokenHash, session.CSRFHash, session.CreatedAt,
		session.ExpiresAt, session.IPMetadata, session.UserAgent)
	if err != nil {
		return fmt.Errorf("create initial administrator session: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE users SET last_login_at = $2 WHERE id = $1", user.ID, session.CreatedAt); err != nil {
		return fmt.Errorf("set initial administrator login time: %w", err)
	}
	if err := audit(ctx, tx, userEvent); err != nil {
		return err
	}
	if err := audit(ctx, tx, loginEvent); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit initial administrator: %w", err)
	}
	return nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.display_name, u.password_hash, u.role, u.enabled,
		       u.created_at, u.updated_at, u.last_login_at,
		       EXISTS(SELECT 1 FROM user_mfa m WHERE m.user_id=u.id AND m.enabled_at IS NOT NULL)
		FROM users u
		WHERE u.email = $1`, email).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Role, &user.Enabled,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt, &user.MFAEnabled)
	if err != nil {
		return domain.User{}, mapDatabaseError(err, "user")
	}
	return user, nil
}

func (s *Store) UserByID(ctx context.Context, id string) (domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.display_name, u.password_hash, u.role, u.enabled,
		       u.created_at, u.updated_at, u.last_login_at,
		       EXISTS(SELECT 1 FROM user_mfa m WHERE m.user_id=u.id AND m.enabled_at IS NOT NULL)
		FROM users u
		WHERE u.id = $1`, id).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Role, &user.Enabled,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt, &user.MFAEnabled)
	if err != nil {
		return domain.User{}, mapDatabaseError(err, "user")
	}
	return user, nil
}

func (s *Store) CreateLoginSession(ctx context.Context, session domain.Session, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin login transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := createLoginSession(ctx, tx, session, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit login: %w", err)
	}
	return nil
}

func createLoginSession(ctx context.Context, tx pgx.Tx, session domain.Session, event domain.AuditEvent) error {
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT enabled FROM users WHERE id=$1 FOR UPDATE`, session.UserID).Scan(&enabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NewError(domain.ErrorAuthentication, "authentication is required")
		}
		return fmt.Errorf("lock login user: %w", err)
	}
	if !enabled {
		return domain.NewError(domain.ErrorAuthentication, "authentication is required")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO sessions
			(id, user_id, token_hash, csrf_hash, created_at, expires_at, last_seen_at, ip_metadata, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $5, $7, $8)`,
		session.ID, session.UserID, session.TokenHash, session.CSRFHash, session.CreatedAt,
		session.ExpiresAt, session.IPMetadata, session.UserAgent)
	if err != nil {
		return fmt.Errorf("create login session: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE users SET last_login_at = $2, updated_at = $2 WHERE id = $1", session.UserID, session.CreatedAt); err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return nil
}

func (s *Store) AuthenticatedSession(ctx context.Context, tokenHash []byte, now time.Time) (domain.Session, domain.User, error) {
	var session domain.Session
	var user domain.User
	err := s.pool.QueryRow(ctx, `
		SELECT s.id, s.user_id, s.token_hash, s.csrf_hash, s.created_at, s.expires_at,
		       s.last_seen_at, s.revoked_at, s.ip_metadata, s.user_agent,
		       u.id, u.email, u.display_name, u.password_hash, u.role, u.enabled,
		       u.created_at, u.updated_at, u.last_login_at,
		       EXISTS(SELECT 1 FROM user_mfa m WHERE m.user_id=u.id AND m.enabled_at IS NOT NULL)
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > $2 AND u.enabled`,
		tokenHash, now).Scan(
		&session.ID, &session.UserID, &session.TokenHash, &session.CSRFHash, &session.CreatedAt,
		&session.ExpiresAt, &session.LastSeenAt, &session.RevokedAt, &session.IPMetadata, &session.UserAgent,
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Role, &user.Enabled,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt, &user.MFAEnabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Session{}, domain.User{}, domain.NewError(domain.ErrorAuthentication, "authentication is required")
		}
		return domain.Session{}, domain.User{}, fmt.Errorf("load authenticated session: %w", err)
	}
	return session, user, nil
}

func (s *Store) TouchSession(ctx context.Context, sessionID string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sessions SET last_seen_at = $2
		WHERE id = $1 AND last_seen_at < $2 - interval '5 minutes'`, sessionID, at)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

func (s *Store) RevokeSession(ctx context.Context, sessionID string, at time.Time, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin logout transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, "UPDATE sessions SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL", sessionID, at); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit logout: %w", err)
	}
	return nil
}

func (s *Store) RecordAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin audit transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audit event: %w", err)
	}
	return nil
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, "DELETE FROM sessions WHERE expires_at <= $1 OR revoked_at < $1 - interval '30 days'", now)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,email,display_name,password_hash,role,enabled,created_at,updated_at,last_login_at FROM users ORDER BY lower(email),id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := []domain.User{}
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Role, &user.Enabled, &user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, user domain.User, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin user creation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(ctx, `INSERT INTO users (id,email,display_name,password_hash,role,enabled,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$7)`, user.ID, user.Email, user.DisplayName, user.PasswordHash, user.Role, user.Enabled, user.CreatedAt)
	if isUniqueViolation(err) {
		return domain.NewError(domain.ErrorConflict, "a user with that email already exists")
	}
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) UpdateUser(ctx context.Context, id, email, displayName string, enabled bool, now time.Time, event domain.AuditEvent) (domain.User, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.User{}, fmt.Errorf("begin user update: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, "LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return domain.User{}, fmt.Errorf("lock users for update: %w", err)
	}
	var current domain.User
	err = tx.QueryRow(ctx, `SELECT id,email,display_name,password_hash,role,enabled,created_at,updated_at,last_login_at FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&current.ID, &current.Email, &current.DisplayName, &current.PasswordHash, &current.Role, &current.Enabled, &current.CreatedAt, &current.UpdatedAt, &current.LastLoginAt)
	if err != nil {
		return domain.User{}, mapDatabaseError(err, "user")
	}
	if current.Enabled && !enabled && current.Role == domain.RoleAdministrator {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE enabled AND role='administrator'`).Scan(&count); err != nil {
			return domain.User{}, fmt.Errorf("count enabled administrators: %w", err)
		}
		if count <= 1 {
			return domain.User{}, domain.NewError(domain.ErrorConflict, "the final enabled administrator cannot be disabled")
		}
	}
	err = tx.QueryRow(ctx, `UPDATE users SET email=$2,display_name=$3,enabled=$4,updated_at=$5 WHERE id=$1 RETURNING id,email,display_name,password_hash,role,enabled,created_at,updated_at,last_login_at`, id, email, displayName, enabled, now).Scan(&current.ID, &current.Email, &current.DisplayName, &current.PasswordHash, &current.Role, &current.Enabled, &current.CreatedAt, &current.UpdatedAt, &current.LastLoginAt)
	if isUniqueViolation(err) {
		return domain.User{}, domain.NewError(domain.ErrorConflict, "a user with that email already exists")
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("update user: %w", err)
	}
	if !enabled {
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$2 WHERE user_id=$1 AND revoked_at IS NULL`, id, now); err != nil {
			return domain.User{}, fmt.Errorf("revoke disabled user sessions: %w", err)
		}
	}
	if err := audit(ctx, tx, event); err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, fmt.Errorf("commit user update: %w", err)
	}
	return current, nil
}

func (s *Store) ResetUserPassword(ctx context.Context, id, passwordHash string, now time.Time, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin password reset: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `UPDATE users SET password_hash=$2,updated_at=$3 WHERE id=$1`, id, passwordHash, now)
	if err != nil {
		return fmt.Errorf("reset user password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NewError(domain.ErrorNotFound, "user was not found")
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$2 WHERE user_id=$1 AND revoked_at IS NULL`, id, now); err != nil {
		return fmt.Errorf("revoke password reset sessions: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ChangeOwnPassword(ctx context.Context, userID, currentSessionID, passwordHash string, now time.Time, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin own password change: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `UPDATE users SET password_hash=$2,updated_at=$3 WHERE id=$1 AND enabled`, userID, passwordHash, now)
	if err != nil {
		return fmt.Errorf("change own password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NewError(domain.ErrorAuthentication, "authentication is required")
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$3 WHERE user_id=$1 AND id<>$2 AND revoked_at IS NULL`, userID, currentSessionID, now); err != nil {
		return fmt.Errorf("revoke other sessions after password change: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit own password change: %w", err)
	}
	return nil
}

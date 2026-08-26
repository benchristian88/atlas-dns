package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

func (s *Store) MFAEnabled(ctx context.Context, userID string) (bool, error) {
	var enabled bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_mfa WHERE user_id=$1 AND enabled_at IS NOT NULL)`, userID).Scan(&enabled); err != nil {
		return false, fmt.Errorf("read MFA status: %w", err)
	}
	return enabled, nil
}

func (s *Store) UserMFA(ctx context.Context, userID string) (domain.UserMFA, error) {
	var value domain.UserMFA
	err := s.pool.QueryRow(ctx, `
		SELECT m.user_id,m.encrypted_secret,m.secret_nonce,m.secret_key_version,m.secret_algorithm,
		       m.enrollment_started_at,m.enrollment_expires_at,m.enabled_at,
		       count(c.id) FILTER (WHERE c.used_at IS NULL)
		FROM user_mfa m
		LEFT JOIN user_mfa_recovery_codes c ON c.user_id=m.user_id
		WHERE m.user_id=$1
		GROUP BY m.user_id`, userID).Scan(
		&value.UserID, &value.Secret.Ciphertext, &value.Secret.Nonce, &value.Secret.KeyVersion,
		&value.Secret.Algorithm, &value.EnrollmentStarted, &value.EnrollmentExpires,
		&value.EnabledAt, &value.RecoveryRemaining,
	)
	if err != nil {
		return domain.UserMFA{}, mapDatabaseError(err, "MFA configuration")
	}
	return value, nil
}

func (s *Store) UpsertPendingMFA(ctx context.Context, value domain.UserMFA) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_mfa
			(user_id,encrypted_secret,secret_nonce,secret_key_version,secret_algorithm,enrollment_started_at,enrollment_expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT(user_id) DO UPDATE SET
			encrypted_secret=EXCLUDED.encrypted_secret,
			secret_nonce=EXCLUDED.secret_nonce,
			secret_key_version=EXCLUDED.secret_key_version,
			secret_algorithm=EXCLUDED.secret_algorithm,
			enrollment_started_at=EXCLUDED.enrollment_started_at,
			enrollment_expires_at=EXCLUDED.enrollment_expires_at,
			enabled_at=NULL
		WHERE user_mfa.enabled_at IS NULL`,
		value.UserID, value.Secret.Ciphertext, value.Secret.Nonce, value.Secret.KeyVersion,
		value.Secret.Algorithm, value.EnrollmentStarted, value.EnrollmentExpires,
	)
	if err != nil {
		return fmt.Errorf("store pending MFA enrollment: %w", err)
	}
	return nil
}

func (s *Store) CreateMFAChallenge(ctx context.Context, challenge domain.MFAChallenge) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin MFA challenge: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := lockMFAUser(ctx, tx, challenge.UserID); err != nil {
		return err
	}
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_mfa WHERE user_id=$1 AND enabled_at IS NOT NULL)`, challenge.UserID).Scan(&enabled); err != nil {
		return fmt.Errorf("recheck MFA before challenge: %w", err)
	}
	if !enabled {
		return domain.NewError(domain.ErrorConflict, "MFA state changed; retry login")
	}
	if _, err := tx.Exec(ctx, `UPDATE mfa_challenges SET consumed_at=$2 WHERE user_id=$1 AND consumed_at IS NULL`, challenge.UserID, challenge.CreatedAt); err != nil {
		return fmt.Errorf("invalidate previous MFA challenges: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO mfa_challenges(id,user_id,token_hash,created_at,expires_at,ip_metadata,user_agent)
		VALUES($1,$2,$3,$4,$5,$6,$7)`, challenge.ID, challenge.UserID, challenge.TokenHash,
		challenge.CreatedAt, challenge.ExpiresAt, challenge.IPMetadata, challenge.UserAgent); err != nil {
		return fmt.Errorf("create MFA challenge: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Store) MFAChallengeByTokenHash(ctx context.Context, tokenHash []byte, now time.Time) (domain.MFAChallenge, domain.User, domain.UserMFA, error) {
	var challenge domain.MFAChallenge
	var user domain.User
	var config domain.UserMFA
	err := s.pool.QueryRow(ctx, `
		SELECT c.id,c.user_id,c.token_hash,c.created_at,c.expires_at,c.consumed_at,c.failed_attempts,c.ip_metadata,c.user_agent,
		       u.id,u.email,u.display_name,u.password_hash,u.role,u.enabled,u.created_at,u.updated_at,u.last_login_at,
		       m.user_id,m.encrypted_secret,m.secret_nonce,m.secret_key_version,m.secret_algorithm,
		       m.enrollment_started_at,m.enrollment_expires_at,m.enabled_at,
		       (SELECT count(*) FROM user_mfa_recovery_codes r WHERE r.user_id=m.user_id AND r.used_at IS NULL)
		FROM mfa_challenges c
		JOIN users u ON u.id=c.user_id
		JOIN user_mfa m ON m.user_id=c.user_id AND m.enabled_at IS NOT NULL
		WHERE c.token_hash=$1 AND c.consumed_at IS NULL AND c.expires_at>$2 AND c.failed_attempts<5 AND u.enabled`, tokenHash, now).Scan(
		&challenge.ID, &challenge.UserID, &challenge.TokenHash, &challenge.CreatedAt, &challenge.ExpiresAt,
		&challenge.ConsumedAt, &challenge.FailedAttempts, &challenge.IPMetadata, &challenge.UserAgent,
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Role, &user.Enabled,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
		&config.UserID, &config.Secret.Ciphertext, &config.Secret.Nonce, &config.Secret.KeyVersion,
		&config.Secret.Algorithm, &config.EnrollmentStarted, &config.EnrollmentExpires,
		&config.EnabledAt, &config.RecoveryRemaining,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.MFAChallenge{}, domain.User{}, domain.UserMFA{}, domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
		}
		return domain.MFAChallenge{}, domain.User{}, domain.UserMFA{}, fmt.Errorf("load MFA challenge: %w", err)
	}
	user.MFAEnabled = true
	return challenge, user, config, nil
}

func (s *Store) FailMFAChallenge(ctx context.Context, tokenHash []byte, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE mfa_challenges
		SET failed_attempts=failed_attempts+1,
		    consumed_at=CASE WHEN failed_attempts+1>=5 THEN $2 ELSE consumed_at END
		WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at>$2 AND failed_attempts<5`, tokenHash, now)
	if err != nil {
		return fmt.Errorf("record MFA challenge failure: %w", err)
	}
	return nil
}

func (s *Store) ConsumeMFAChallenge(ctx context.Context, tokenHash, recoveryHash []byte, now time.Time, event *domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin MFA challenge consumption: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var userID string
	err = tx.QueryRow(ctx, `
		UPDATE mfa_challenges SET consumed_at=$2
		WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at>$2 AND failed_attempts<5
		RETURNING user_id`, tokenHash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
	}
	if err != nil {
		return fmt.Errorf("consume MFA challenge: %w", err)
	}
	if len(recoveryHash) != 0 {
		tag, err := tx.Exec(ctx, `
			UPDATE user_mfa_recovery_codes SET used_at=$3
			WHERE user_id=$1 AND code_hash=$2 AND used_at IS NULL`, userID, recoveryHash, now)
		if err != nil {
			return fmt.Errorf("consume MFA recovery code: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
		}
	}
	if event != nil {
		if err := audit(ctx, tx, *event); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit MFA challenge consumption: %w", err)
	}
	return nil
}

// CompleteMFAChallenge consumes the pre-session factor and creates the normal
// session in one transaction. The per-user lock serializes host reset/disable
// so neither can finish before a newly completed session is visible to its
// revocation update.
func (s *Store) CompleteMFAChallenge(ctx context.Context, tokenHash, recoveryHash []byte, now time.Time, recoveryEvent *domain.AuditEvent, session domain.Session, loginEvent domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin MFA login completion: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var challengeUserID string
	if err := tx.QueryRow(ctx, `SELECT user_id FROM mfa_challenges WHERE token_hash=$1`, tokenHash).Scan(&challengeUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
		}
		return fmt.Errorf("resolve MFA challenge user: %w", err)
	}
	if challengeUserID != session.UserID {
		return domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
	}
	if err := lockMFAUser(ctx, tx, challengeUserID); err != nil {
		return err
	}
	var userID string
	err = tx.QueryRow(ctx, `
		UPDATE mfa_challenges SET consumed_at=$2
		WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at>$2 AND failed_attempts<5
		  AND EXISTS(SELECT 1 FROM user_mfa WHERE user_id=mfa_challenges.user_id AND enabled_at IS NOT NULL)
		RETURNING user_id`, tokenHash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
	}
	if err != nil {
		return fmt.Errorf("consume MFA challenge for login: %w", err)
	}
	if len(recoveryHash) != 0 {
		tag, err := tx.Exec(ctx, `
			UPDATE user_mfa_recovery_codes SET used_at=$3
			WHERE user_id=$1 AND code_hash=$2 AND used_at IS NULL`, userID, recoveryHash, now)
		if err != nil {
			return fmt.Errorf("consume MFA recovery code: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return domain.NewError(domain.ErrorInvalidCredentials, "authentication code is invalid or expired")
		}
	}
	if recoveryEvent != nil {
		if err := audit(ctx, tx, *recoveryEvent); err != nil {
			return err
		}
	}
	if err := createLoginSession(ctx, tx, session, loginEvent); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit MFA login completion: %w", err)
	}
	return nil
}

func (s *Store) EnableMFA(ctx context.Context, userID, currentSessionID string, codes []domain.MFARecoveryCode, now time.Time, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin MFA enable: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := lockMFAUser(ctx, tx, userID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE user_mfa SET enabled_at=$2 WHERE user_id=$1 AND enabled_at IS NULL AND enrollment_expires_at>$2`, userID, now)
	if err != nil {
		return fmt.Errorf("enable MFA: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return domain.NewError(domain.ErrorConflict, "MFA enrollment is missing or expired")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_mfa_recovery_codes WHERE user_id=$1`, userID); err != nil {
		return fmt.Errorf("clear MFA recovery codes: %w", err)
	}
	if err := insertRecoveryCodes(ctx, tx, codes); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$3 WHERE user_id=$1 AND id<>$2 AND revoked_at IS NULL`, userID, currentSessionID, now); err != nil {
		return fmt.Errorf("revoke pre-MFA sessions: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE mfa_challenges SET consumed_at=$2 WHERE user_id=$1 AND consumed_at IS NULL`, userID, now); err != nil {
		return fmt.Errorf("invalidate MFA challenges: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ReplaceRecoveryCodes(ctx context.Context, userID, currentSessionID string, codes []domain.MFARecoveryCode, now time.Time, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin recovery-code regeneration: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := lockMFAUser(ctx, tx, userID); err != nil {
		return err
	}
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT enabled_at IS NOT NULL FROM user_mfa WHERE user_id=$1 FOR UPDATE`, userID).Scan(&enabled); err != nil || !enabled {
		if errors.Is(err, pgx.ErrNoRows) || !enabled {
			return domain.NewError(domain.ErrorConflict, "MFA is not enabled")
		}
		return fmt.Errorf("lock MFA configuration: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_mfa_recovery_codes WHERE user_id=$1`, userID); err != nil {
		return fmt.Errorf("invalidate recovery codes: %w", err)
	}
	if err := insertRecoveryCodes(ctx, tx, codes); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$3 WHERE user_id=$1 AND id<>$2 AND revoked_at IS NULL`, userID, currentSessionID, now); err != nil {
		return fmt.Errorf("revoke other sessions: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DisableMFA(ctx context.Context, userID, currentSessionID string, now time.Time, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin MFA disable: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := lockMFAUser(ctx, tx, userID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM user_mfa WHERE user_id=$1 AND enabled_at IS NOT NULL`, userID)
	if err != nil {
		return fmt.Errorf("disable MFA: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return domain.NewError(domain.ErrorConflict, "MFA is not enabled")
	}
	if _, err := tx.Exec(ctx, `UPDATE mfa_challenges SET consumed_at=$2 WHERE user_id=$1 AND consumed_at IS NULL`, userID, now); err != nil {
		return fmt.Errorf("invalidate MFA challenges: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$3 WHERE user_id=$1 AND id<>$2 AND revoked_at IS NULL`, userID, currentSessionID, now); err != nil {
		return fmt.Errorf("revoke other sessions: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ResetMFAFromHost(ctx context.Context, email string, now time.Time, event domain.AuditEvent) (domain.User, int64, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.User{}, 0, fmt.Errorf("begin host MFA reset: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var user domain.User
	err = tx.QueryRow(ctx, `SELECT id,email,display_name,password_hash,role,enabled,created_at,updated_at,last_login_at FROM users WHERE email=$1`, email).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Role, &user.Enabled,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt)
	if err != nil {
		return domain.User{}, 0, mapDatabaseError(err, "user")
	}
	if err := lockMFAUser(ctx, tx, user.ID); err != nil {
		return domain.User{}, 0, err
	}
	if err := tx.QueryRow(ctx, `SELECT id,email,display_name,password_hash,role,enabled,created_at,updated_at,last_login_at FROM users WHERE id=$1 FOR UPDATE`, user.ID).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Role, &user.Enabled,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt); err != nil {
		return domain.User{}, 0, mapDatabaseError(err, "user")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_mfa WHERE user_id=$1`, user.ID); err != nil {
		return domain.User{}, 0, fmt.Errorf("delete MFA configuration: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE mfa_challenges SET consumed_at=$2 WHERE user_id=$1 AND consumed_at IS NULL`, user.ID, now); err != nil {
		return domain.User{}, 0, fmt.Errorf("invalidate MFA challenges: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$2 WHERE user_id=$1 AND revoked_at IS NULL`, user.ID, now)
	if err != nil {
		return domain.User{}, 0, fmt.Errorf("revoke sessions: %w", err)
	}
	resourceID := user.ID
	event.ResourceID = &resourceID
	if err := audit(ctx, tx, event); err != nil {
		return domain.User{}, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, 0, fmt.Errorf("commit host MFA reset: %w", err)
	}
	return user, tag.RowsAffected(), nil
}

func (s *Store) DeleteExpiredMFAChallenges(ctx context.Context, now time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM mfa_challenges WHERE expires_at<=$1 OR consumed_at<$1-interval '1 day'`, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired MFA challenges: %w", err)
	}
	return tag.RowsAffected(), nil
}

func insertRecoveryCodes(ctx context.Context, tx pgx.Tx, codes []domain.MFARecoveryCode) error {
	for _, code := range codes {
		if _, err := tx.Exec(ctx, `INSERT INTO user_mfa_recovery_codes(id,user_id,code_hash,created_at) VALUES($1,$2,$3,$4)`, code.ID, code.UserID, code.CodeHash, code.CreatedAt); err != nil {
			return fmt.Errorf("store MFA recovery code: %w", err)
		}
	}
	return nil
}

func lockMFAUser(ctx context.Context, tx pgx.Tx, userID string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, userID); err != nil {
		return fmt.Errorf("lock user MFA state: %w", err)
	}
	return nil
}

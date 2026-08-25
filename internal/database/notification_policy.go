package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
)

func (s *Store) NotificationPolicy(ctx context.Context) (haoperations.NotificationPolicy, error) {
	var value haoperations.NotificationPolicy
	if err := s.pool.QueryRow(ctx, `SELECT enabled_event_types,record_version FROM notification_policy WHERE singleton`).Scan(&value.EnabledEventTypes, &value.RecordVersion); err != nil {
		return value, fmt.Errorf("read notification policy: %w", err)
	}
	return value, nil
}

func (s *Store) UpdateNotificationPolicy(ctx context.Context, value haoperations.NotificationPolicy, expectedVersion int, now time.Time, event domain.AuditEvent) (haoperations.NotificationPolicy, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return value, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	err = tx.QueryRow(ctx, `UPDATE notification_policy SET enabled_event_types=$1,record_version=record_version+1,updated_at=$2,updated_by=$3 WHERE singleton AND record_version=$4 RETURNING record_version`,
		value.EnabledEventTypes, now, event.ActorUserID, expectedVersion).Scan(&value.RecordVersion)
	if err == pgx.ErrNoRows {
		return value, domain.NewError(domain.ErrorConflict, "notification policy changed; reload and try again")
	}
	if err != nil {
		return value, fmt.Errorf("update notification policy: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return value, err
	}
	if err := tx.Commit(ctx); err != nil {
		return value, err
	}
	return value, nil
}

package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

func (s *Store) PruneOperationalHistory(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit < 1 || limit > 10000 {
		limit = 10000
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM ha_operational_events WHERE id IN (
		SELECT id FROM ha_operational_events WHERE occurred_at < $1 ORDER BY occurred_at,id LIMIT $2
	)`, before.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("prune Operational History: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *Store) ClearOperationalHistory(ctx context.Context, event domain.AuditEvent) (systemsettings.ClearOperationalHistoryResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return systemsettings.ClearOperationalHistoryResult{}, fmt.Errorf("begin Operational History clear: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `LOCK TABLE ha_operational_events IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return systemsettings.ClearOperationalHistoryResult{}, err
	}
	var deliveries int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM notification_deliveries`).Scan(&deliveries); err != nil {
		return systemsettings.ClearOperationalHistoryResult{}, fmt.Errorf("count Operational History deliveries: %w", err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM ha_operational_events`)
	if err != nil {
		return systemsettings.ClearOperationalHistoryResult{}, fmt.Errorf("delete Operational History: %w", err)
	}
	result := systemsettings.ClearOperationalHistoryResult{EventsDeleted: tag.RowsAffected(), DeliveriesDeleted: deliveries}
	event.Metadata = map[string]any{"eventsDeleted": result.EventsDeleted, "deliveriesDeleted": result.DeliveriesDeleted, "result": "succeeded"}
	if err := audit(ctx, tx, event); err != nil {
		return systemsettings.ClearOperationalHistoryResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return systemsettings.ClearOperationalHistoryResult{}, fmt.Errorf("commit Operational History clear: %w", err)
	}
	return result, nil
}

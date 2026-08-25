package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/onboarding"
)

func (s *Store) OnboardingState(ctx context.Context, clusterID string) (onboarding.State, error) {
	var value onboarding.State
	err := s.pool.QueryRow(ctx, `SELECT cluster_id,redundancy_skipped_at,monitoring_reviewed_at,notifications_skipped_at,completed_at,completed_by,record_version,updated_at FROM cluster_onboarding_state WHERE cluster_id=$1`, clusterID).Scan(
		&value.ClusterID, &value.RedundancySkippedAt, &value.MonitoringReviewedAt, &value.NotificationsSkippedAt,
		&value.CompletedAt, &value.CompletedBy, &value.RecordVersion, &value.UpdatedAt,
	)
	if err != nil {
		return value, mapDatabaseError(err, "onboarding state")
	}
	return value, nil
}

func (s *Store) SaveOnboardingState(ctx context.Context, value onboarding.State, expectedVersion int, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin onboarding state update: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var affected int64
	if expectedVersion == 0 {
		result, execErr := tx.Exec(ctx, `INSERT INTO cluster_onboarding_state (cluster_id,redundancy_skipped_at,monitoring_reviewed_at,notifications_skipped_at,completed_at,completed_by,record_version,updated_at) VALUES($1,$2,$3,$4,$5,$6,1,$7) ON CONFLICT DO NOTHING`, value.ClusterID, value.RedundancySkippedAt, value.MonitoringReviewedAt, value.NotificationsSkippedAt, value.CompletedAt, value.CompletedBy, value.UpdatedAt)
		err, affected = execErr, result.RowsAffected()
	} else {
		result, execErr := tx.Exec(ctx, `UPDATE cluster_onboarding_state SET redundancy_skipped_at=$2,monitoring_reviewed_at=$3,notifications_skipped_at=$4,completed_at=$5,completed_by=$6,record_version=record_version+1,updated_at=$7 WHERE cluster_id=$1 AND record_version=$8`, value.ClusterID, value.RedundancySkippedAt, value.MonitoringReviewedAt, value.NotificationsSkippedAt, value.CompletedAt, value.CompletedBy, value.UpdatedAt, expectedVersion)
		err, affected = execErr, result.RowsAffected()
	}
	if err != nil {
		return fmt.Errorf("save onboarding state: %w", err)
	}
	if affected == 0 {
		return domain.NewError(domain.ErrorConflict, "onboarding state changed; reload and try again")
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

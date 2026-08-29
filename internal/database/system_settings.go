package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

func (s *Store) SystemSettings(ctx context.Context) (systemsettings.StoredSettings, error) {
	var value systemsettings.StoredSettings
	var session, health, requestTimeout, statistics, queryPoll, queryRetention *int64
	var logLevel *string
	var operationalHistoryDays int64
	var queryEnabled *bool
	err := s.pool.QueryRow(ctx, `SELECT update_checks_enabled,runtime_settings_initialized,session_duration_seconds,node_health_interval_seconds,node_request_timeout_seconds,statistics_poll_interval_seconds,query_log_collection_enabled,query_log_poll_interval_seconds,query_log_retention_seconds,log_level,operational_history_retention_days,record_version FROM system_settings WHERE singleton`).Scan(
		&value.UpdateChecksEnabled, &value.RuntimeInitialized, &session, &health, &requestTimeout, &statistics, &queryEnabled, &queryPoll, &queryRetention, &logLevel, &operationalHistoryDays, &value.RecordVersion,
	)
	if err != nil {
		return value, fmt.Errorf("read system settings: %w", err)
	}
	if session != nil {
		value.Runtime.SessionDuration = time.Duration(*session) * time.Second
	}
	if health != nil {
		value.Runtime.NodeHealthInterval = time.Duration(*health) * time.Second
	}
	if requestTimeout != nil {
		value.Runtime.NodeRequestTimeout = time.Duration(*requestTimeout) * time.Second
	}
	if statistics != nil {
		value.Runtime.StatisticsPollInterval = time.Duration(*statistics) * time.Second
	}
	if queryEnabled != nil {
		value.Runtime.QueryLogCollection = *queryEnabled
	}
	if queryPoll != nil {
		value.Runtime.QueryLogPollInterval = time.Duration(*queryPoll) * time.Second
	}
	if queryRetention != nil {
		value.Runtime.QueryLogRetention = time.Duration(*queryRetention) * time.Second
	}
	if logLevel != nil {
		value.Runtime.LogLevel = *logLevel
	}
	value.Runtime.OperationalHistoryRetention = time.Duration(operationalHistoryDays) * 24 * time.Hour
	return value, nil
}

func (s *Store) InitializeRuntimeSettings(ctx context.Context, fallback systemsettings.RuntimeSettings) (systemsettings.StoredSettings, error) {
	_, err := s.pool.Exec(ctx, `UPDATE system_settings SET runtime_settings_initialized=true,
		session_duration_seconds=COALESCE(session_duration_seconds,$1),node_health_interval_seconds=COALESCE(node_health_interval_seconds,$2),
		node_request_timeout_seconds=COALESCE(node_request_timeout_seconds,$3),statistics_poll_interval_seconds=COALESCE(statistics_poll_interval_seconds,$4),
		query_log_collection_enabled=COALESCE(query_log_collection_enabled,$5),query_log_poll_interval_seconds=COALESCE(query_log_poll_interval_seconds,$6),
		query_log_retention_seconds=COALESCE(query_log_retention_seconds,$7),log_level=COALESCE(log_level,$8)
		WHERE singleton AND (NOT runtime_settings_initialized OR session_duration_seconds IS NULL OR node_health_interval_seconds IS NULL
			OR node_request_timeout_seconds IS NULL OR statistics_poll_interval_seconds IS NULL OR query_log_collection_enabled IS NULL
			OR query_log_poll_interval_seconds IS NULL OR query_log_retention_seconds IS NULL OR log_level IS NULL)`,
		int64(fallback.SessionDuration/time.Second), int64(fallback.NodeHealthInterval/time.Second), int64(fallback.NodeRequestTimeout/time.Second),
		int64(fallback.StatisticsPollInterval/time.Second), fallback.QueryLogCollection,
		int64(fallback.QueryLogPollInterval/time.Second), int64(fallback.QueryLogRetention/time.Second), fallback.LogLevel)
	if err != nil {
		return systemsettings.StoredSettings{}, fmt.Errorf("initialize persisted runtime settings: %w", err)
	}
	return s.SystemSettings(ctx)
}

func (s *Store) UpdateChecksEnabled(ctx context.Context) (bool, error) {
	var enabled bool
	if err := s.pool.QueryRow(ctx, `SELECT update_checks_enabled FROM system_settings WHERE singleton`).Scan(&enabled); err != nil {
		return false, fmt.Errorf("read update-check setting: %w", err)
	}
	return enabled, nil
}

func (s *Store) UpdateSystemSettings(ctx context.Context, value systemsettings.StoredSettings, expectedVersion int, now time.Time, event domain.AuditEvent) (systemsettings.StoredSettings, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return systemsettings.StoredSettings{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	err = tx.QueryRow(ctx, `UPDATE system_settings SET update_checks_enabled=$1,runtime_settings_initialized=true,session_duration_seconds=$2,node_health_interval_seconds=$3,node_request_timeout_seconds=$4,statistics_poll_interval_seconds=$5,query_log_collection_enabled=$6,query_log_poll_interval_seconds=$7,query_log_retention_seconds=$8,log_level=$9,operational_history_retention_days=$10,record_version=record_version+1,updated_at=$11,updated_by=$12 WHERE singleton AND record_version=$13 RETURNING record_version`,
		value.UpdateChecksEnabled, int64(value.Runtime.SessionDuration/time.Second), int64(value.Runtime.NodeHealthInterval/time.Second), int64(value.Runtime.NodeRequestTimeout/time.Second), int64(value.Runtime.StatisticsPollInterval/time.Second), value.Runtime.QueryLogCollection,
		int64(value.Runtime.QueryLogPollInterval/time.Second), int64(value.Runtime.QueryLogRetention/time.Second), value.Runtime.LogLevel, int64(value.Runtime.OperationalHistoryRetention/(24*time.Hour)), now, event.ActorUserID, expectedVersion).Scan(&value.RecordVersion)
	if err == pgx.ErrNoRows {
		return systemsettings.StoredSettings{}, domain.NewError(domain.ErrorConflict, "system settings changed; reload and try again")
	}
	if err != nil {
		return systemsettings.StoredSettings{}, fmt.Errorf("update system settings: %w", err)
	}
	value.RuntimeInitialized = true
	if err := audit(ctx, tx, event); err != nil {
		return systemsettings.StoredSettings{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return systemsettings.StoredSettings{}, err
	}
	return value, nil
}

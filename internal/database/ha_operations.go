package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
)

func (s *Store) NodeLifecycleSettings(ctx context.Context, nodeID string) (haoperations.NodeSettings, error) {
	var value haoperations.NodeSettings
	err := s.pool.QueryRow(ctx, `SELECT node_id,dns_probe_host,dns_probe_port,dns_probe_name,dns_probe_type,
		expected_rcode,probe_udp,probe_tcp,installation_type,record_version,created_at,updated_at
		FROM node_lifecycle_settings WHERE node_id=$1`, nodeID).Scan(&value.NodeID, &value.DNSProbeHost,
		&value.DNSProbePort, &value.DNSProbeName, &value.DNSProbeType, &value.ExpectedRCode,
		&value.ProbeUDP, &value.ProbeTCP, &value.InstallationType, &value.RecordVersion,
		&value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		return haoperations.NodeSettings{}, mapDatabaseError(err, "node lifecycle settings")
	}
	return value, nil
}

func (s *Store) SaveNodeLifecycleSettings(ctx context.Context, value haoperations.NodeSettings, expectedVersion int, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin lifecycle settings update: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var rowsAffected int64
	if expectedVersion == 0 {
		tag, execErr := tx.Exec(ctx, `INSERT INTO node_lifecycle_settings
			(node_id,dns_probe_host,dns_probe_port,dns_probe_name,dns_probe_type,expected_rcode,probe_udp,probe_tcp,installation_type,record_version,created_at,updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,1,$10,$10) ON CONFLICT DO NOTHING`, value.NodeID,
			value.DNSProbeHost, value.DNSProbePort, value.DNSProbeName, value.DNSProbeType,
			value.ExpectedRCode, value.ProbeUDP, value.ProbeTCP, value.InstallationType, value.UpdatedAt)
		err, rowsAffected = execErr, tag.RowsAffected()
	} else {
		tag, execErr := tx.Exec(ctx, `UPDATE node_lifecycle_settings SET dns_probe_host=$2,dns_probe_port=$3,
			dns_probe_name=$4,dns_probe_type=$5,expected_rcode=$6,probe_udp=$7,probe_tcp=$8,
			installation_type=$9,record_version=record_version+1,updated_at=$10
			WHERE node_id=$1 AND record_version=$11`, value.NodeID, value.DNSProbeHost, value.DNSProbePort,
			value.DNSProbeName, value.DNSProbeType, value.ExpectedRCode, value.ProbeUDP, value.ProbeTCP,
			value.InstallationType, value.UpdatedAt, expectedVersion)
		err, rowsAffected = execErr, tag.RowsAffected()
	}
	if err != nil {
		return fmt.Errorf("save lifecycle settings: %w", err)
	}
	if rowsAffected == 0 {
		return domain.NewError(domain.ErrorConflict, "node lifecycle settings were changed by another request")
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit lifecycle settings: %w", err)
	}
	return nil
}

func (s *Store) LatestDNSProbe(ctx context.Context, nodeID string) (haoperations.DNSProbeResult, error) {
	return scanDNSProbe(s.pool.QueryRow(ctx, dnsProbeSelect+` WHERE node_id=$1 ORDER BY probed_at DESC,id DESC LIMIT 1`, nodeID))
}

func (s *Store) LatestDNSProbes(ctx context.Context, clusterID string) ([]haoperations.DNSProbeResult, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (node_id) id,cluster_id,node_id,status,udp_status,tcp_status,response_code,latency_ms,address_family,error_code,probed_at
		FROM dns_probe_results WHERE cluster_id=$1 ORDER BY node_id,probed_at DESC,id DESC`, clusterID)
	if err != nil {
		return nil, fmt.Errorf("list latest DNS probes: %w", err)
	}
	defer rows.Close()
	result := []haoperations.DNSProbeResult{}
	for rows.Next() {
		value, scanErr := scanDNSProbe(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate DNS probes: %w", err)
	}
	return result, nil
}

const dnsProbeSelect = `SELECT id,cluster_id,node_id,status,udp_status,tcp_status,response_code,latency_ms,address_family,error_code,probed_at FROM dns_probe_results`

func scanDNSProbe(row rowScanner) (haoperations.DNSProbeResult, error) {
	var value haoperations.DNSProbeResult
	if err := row.Scan(&value.ID, &value.ClusterID, &value.NodeID, &value.Status, &value.UDPStatus,
		&value.TCPStatus, &value.ResponseCode, &value.LatencyMS, &value.AddressFamily, &value.ErrorCode,
		&value.ProbedAt); err != nil {
		return value, mapDatabaseError(err, "DNS probe")
	}
	return value, nil
}

func (s *Store) SaveDNSProbe(ctx context.Context, value haoperations.DNSProbeResult, transition *haoperations.Event) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin DNS probe result: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(ctx, `INSERT INTO dns_probe_results
		(id,cluster_id,node_id,status,udp_status,tcp_status,response_code,latency_ms,address_family,error_code,probed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, value.ID, value.ClusterID, value.NodeID,
		value.Status, value.UDPStatus, value.TCPStatus, value.ResponseCode, value.LatencyMS,
		value.AddressFamily, value.ErrorCode, value.ProbedAt)
	if err != nil {
		return fmt.Errorf("insert DNS probe: %w", err)
	}
	if transition != nil {
		if err := insertHAEvent(ctx, tx, *transition); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dns_probe_results WHERE id IN
		(SELECT id FROM dns_probe_results WHERE probed_at < $1 ORDER BY probed_at,id LIMIT 10000)`, value.ProbedAt.Add(-30*24*time.Hour)); err != nil {
		return fmt.Errorf("clean DNS probes: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit DNS probe: %w", err)
	}
	return nil
}

func (s *Store) RecordHAEvent(ctx context.Context, value haoperations.Event) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin HA event: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := insertHAEvent(ctx, tx, value); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ha_operational_events WHERE id IN
		(SELECT id FROM ha_operational_events WHERE occurred_at < $1 ORDER BY occurred_at,id LIMIT 10000)`, value.OccurredAt.Add(-365*24*time.Hour)); err != nil {
		return fmt.Errorf("clean HA operational events: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordHAEventAndAudit(ctx context.Context, value haoperations.Event, auditEvent domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin audited HA event: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := insertHAEvent(ctx, tx, value); err != nil {
		return err
	}
	if err := audit(ctx, tx, auditEvent); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audited HA event: %w", err)
	}
	return nil
}

func insertHAEvent(ctx context.Context, tx pgx.Tx, value haoperations.Event) error {
	details, err := json.Marshal(value.Details)
	if err != nil {
		return fmt.Errorf("encode HA event: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO ha_operational_events (id,cluster_id,node_id,event_type,severity,summary,details_json,occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, value.ID, value.ClusterID, value.NodeID, value.EventType,
		value.Severity, value.Summary, details, value.OccurredAt)
	if err != nil {
		return fmt.Errorf("insert HA event: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT id FROM notification_channels WHERE cluster_id=$1 AND enabled`, value.ClusterID)
	if err != nil {
		return fmt.Errorf("list event notification channels: %w", err)
	}
	channelIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		channelIDs = append(channelIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, channelID := range channelIDs {
		deliveryID, idErr := domain.NewID()
		if idErr != nil {
			return idErr
		}
		status := "pending"
		var next *time.Time
		at := value.OccurredAt
		if value.EventType == "dns.failed" && value.NodeID != nil {
			var maintenance bool
			if err := tx.QueryRow(ctx, `SELECT maintenance_mode FROM nodes WHERE id=$1`, *value.NodeID).Scan(&maintenance); err != nil {
				return err
			}
			if maintenance {
				status = "suppressed"
			} else {
				next = &at
			}
		} else {
			next = &at
		}
		_, err = tx.Exec(ctx, `INSERT INTO notification_deliveries
			(id,channel_id,event_id,status,attempt_count,next_attempt_at,created_at,completed_at,channel_name)
			SELECT $1,c.id,$3,$4,0,$5,$6::timestamptz,
				CASE WHEN $4='suppressed' THEN $6::timestamptz ELSE NULL::timestamptz END,c.name
			FROM notification_channels c WHERE c.id=$2
			ON CONFLICT(channel_id,event_id) DO NOTHING`, deliveryID, channelID, value.ID, status, next, value.OccurredAt)
		if err != nil {
			return fmt.Errorf("queue notification: %w", err)
		}
	}
	return nil
}

func (s *Store) ListHAEvents(ctx context.Context, clusterID, nodeID string, limit int) ([]haoperations.Event, error) {
	query := `SELECT id,cluster_id,node_id,event_type,severity,summary,details_json,occurred_at
		FROM ha_operational_events WHERE cluster_id=$1`
	args := []any{clusterID}
	if nodeID != "" {
		query += ` AND node_id=$2`
		args = append(args, nodeID)
	}
	query += fmt.Sprintf(` ORDER BY occurred_at DESC,id DESC LIMIT $%d`, len(args)+1)
	args = append(args, limit)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list HA events: %w", err)
	}
	defer rows.Close()
	result := []haoperations.Event{}
	for rows.Next() {
		var value haoperations.Event
		var details []byte
		if err := rows.Scan(&value.ID, &value.ClusterID, &value.NodeID, &value.EventType, &value.Severity, &value.Summary, &details, &value.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan HA event: %w", err)
		}
		if err := json.Unmarshal(details, &value.Details); err != nil {
			return nil, fmt.Errorf("decode HA event: %w", err)
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) ListHAHistory(ctx context.Context, request haoperations.HistoryQuery) ([]haoperations.HistoryItem, error) {
	args := []any{request.ClusterID}
	eventWhere := "e.cluster_id=$1 AND e.event_type<>'notification.test'"
	deliveryWhere := "e.cluster_id=$1"
	if request.NodeID != "" {
		args = append(args, request.NodeID)
		parameter := fmt.Sprintf("$%d", len(args))
		eventWhere += " AND e.node_id=" + parameter
		deliveryWhere += " AND e.node_id=" + parameter
	}
	if request.BeforeAt != nil {
		args = append(args, *request.BeforeAt, request.BeforeID)
		atParameter := fmt.Sprintf("$%d", len(args)-1)
		idParameter := fmt.Sprintf("$%d", len(args))
		eventWhere += " AND (e.occurred_at,e.id)<(" + atParameter + "," + idParameter + ")"
		deliveryWhere += " AND (d.created_at,d.id)<(" + atParameter + "," + idParameter + ")"
	}
	args = append(args, request.Limit)
	limitParameter := fmt.Sprintf("$%d", len(args))
	query := `WITH history AS (
		SELECT e.id,e.cluster_id,e.node_id,e.event_type,e.severity,e.summary,e.details_json,e.occurred_at,
			'event'::text AS kind,NULL::uuid AS channel_id,''::text AS channel_name,''::text AS delivery_status,
			0::integer AS attempt_count,''::text AS error_code,''::text AS error_summary,NULL::integer AS http_status,
			NULL::timestamptz AS completed_at,false AS is_test
		FROM ha_operational_events e WHERE ` + eventWhere + `
		UNION ALL
		SELECT d.id,e.cluster_id,e.node_id,e.event_type,e.severity,e.summary,'{}'::jsonb,d.created_at,
			'notification',d.channel_id,d.channel_name,
			CASE WHEN d.status='succeeded' THEN 'delivered'
				 WHEN d.status='failed' AND d.completed_at IS NULL THEN 'pending'
				 ELSE d.status END,
			d.attempt_count,d.error_code,d.error_summary,d.http_status,d.completed_at,e.event_type='notification.test'
		FROM notification_deliveries d JOIN ha_operational_events e ON e.id=d.event_id WHERE ` + deliveryWhere + `
	)
	SELECT id,cluster_id,node_id,event_type,severity,summary,details_json,occurred_at,kind,channel_id,channel_name,
		delivery_status,attempt_count,error_code,error_summary,http_status,completed_at,is_test
	FROM history ORDER BY occurred_at DESC,id DESC LIMIT ` + limitParameter
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list HA operational history: %w", err)
	}
	defer rows.Close()
	result := []haoperations.HistoryItem{}
	for rows.Next() {
		var value haoperations.HistoryItem
		var details []byte
		var channelID *string
		var channelName, status, errorCode, errorSummary string
		var attemptCount int
		var httpStatus *int
		var completedAt *time.Time
		var test bool
		if err := rows.Scan(&value.ID, &value.ClusterID, &value.NodeID, &value.EventType, &value.Severity, &value.Summary,
			&details, &value.OccurredAt, &value.Kind, &channelID, &channelName, &status, &attemptCount, &errorCode,
			&errorSummary, &httpStatus, &completedAt, &test); err != nil {
			return nil, fmt.Errorf("scan HA operational history: %w", err)
		}
		if err := json.Unmarshal(details, &value.Details); err != nil {
			return nil, fmt.Errorf("decode HA operational history: %w", err)
		}
		if value.Kind == "notification" {
			value.Notification = &haoperations.NotificationHistoryOutcome{ChannelID: channelID, ChannelName: channelName, Status: status,
				AttemptCount: attemptCount, ErrorCode: errorCode, ErrorSummary: errorSummary, HTTPStatus: httpStatus, Test: test, CompletedAt: completedAt}
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate HA operational history: %w", err)
	}
	return result, nil
}

func (s *Store) ActiveDeploymentExists(ctx context.Context, clusterID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE cluster_id=$1 AND status IN ('queued','validating','running','cancelling'))`, clusterID).Scan(&exists)
	return exists, err
}

func (s *Store) OpenDriftExists(ctx context.Context, nodeID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drift_events WHERE node_id=$1 AND status='open')`, nodeID).Scan(&exists)
	return exists, err
}

func (s *Store) CollectorChecks(ctx context.Context, nodeID string) ([]haoperations.Check, error) {
	checks := []haoperations.Check{}
	var statistics string
	statisticsErr := s.pool.QueryRow(ctx, `SELECT status FROM statistics_poll_attempts WHERE node_id=$1 ORDER BY completed_at DESC,id DESC LIMIT 1`, nodeID).Scan(&statistics)
	statisticsUnconfigured := errors.Is(statisticsErr, pgx.ErrNoRows)
	if statisticsErr != nil && !statisticsUnconfigured {
		return nil, fmt.Errorf("read statistics collector state: %w", statisticsErr)
	}
	statisticsOK := statisticsUnconfigured || (statisticsErr == nil && (statistics == "succeeded" || statistics == "partial" || statistics == "unsupported" || statistics == "maintenance"))
	statisticsStatus := map[bool]string{true: "pass", false: "fail"}[statisticsOK]
	if statisticsUnconfigured {
		statisticsStatus = "not_applicable"
	}
	checks = append(checks, haoperations.Check{Name: "statistics_collector", Status: statisticsStatus, Required: !statisticsUnconfigured, ErrorCode: map[bool]string{true: "", false: "STATISTICS_COLLECTOR_NOT_READY"}[statisticsOK], Message: map[bool]string{true: "Collector is not configured for this node", false: "Statistics collector has a known resumable state"}[statisticsUnconfigured]})
	var queryLog string
	queryErr := s.pool.QueryRow(ctx, `SELECT last_status FROM query_ingestion_checkpoints WHERE node_id=$1`, nodeID).Scan(&queryLog)
	queryUnconfigured := errors.Is(queryErr, pgx.ErrNoRows)
	if queryErr != nil && !queryUnconfigured {
		return nil, fmt.Errorf("read Query Log collector state: %w", queryErr)
	}
	queryOK := queryUnconfigured || (queryErr == nil && (queryLog == "succeeded" || queryLog == "partial" || queryLog == "unsupported" || queryLog == "logging_disabled" || queryLog == "maintenance"))
	queryStatus := map[bool]string{true: "pass", false: "fail"}[queryOK]
	if queryUnconfigured {
		queryStatus = "not_applicable"
	}
	checks = append(checks, haoperations.Check{Name: "query_log_collector", Status: queryStatus, Required: !queryUnconfigured, ErrorCode: map[bool]string{true: "", false: "QUERY_LOG_COLLECTOR_NOT_READY"}[queryOK], Message: map[bool]string{true: "Collector is not configured for this node", false: "Query Log collector has a known resumable state"}[queryUnconfigured]})
	return checks, nil
}

func (s *Store) CreateUpgrade(ctx context.Context, value haoperations.Upgrade, auditEvent domain.AuditEvent, event haoperations.Event) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	preflight, _ := json.Marshal(value.Preflight)
	validation, _ := json.Marshal(value.Validation)
	_, err = tx.Exec(ctx, `INSERT INTO upgrade_operations (id,cluster_id,node_id,from_version,target_version,installation_type,mode,status,requested_by,request_id,preflight_json,validation_json,error_code,error_summary,started_at,completed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, value.ID, value.ClusterID, value.NodeID, value.FromVersion, value.TargetVersion, value.InstallationType, value.Mode, value.Status, value.RequestedBy, value.RequestID, preflight, validation, value.ErrorCode, value.ErrorSummary, value.StartedAt, value.CompletedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewError(domain.ErrorConflict, "the node already has an active upgrade")
		}
		return fmt.Errorf("create upgrade: %w", err)
	}
	if err := audit(ctx, tx, auditEvent); err != nil {
		return err
	}
	if err := insertHAEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) UpdateUpgrade(ctx context.Context, value haoperations.Upgrade, auditEvent domain.AuditEvent, event haoperations.Event) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	validation, _ := json.Marshal(value.Validation)
	tag, err := tx.Exec(ctx, `UPDATE upgrade_operations SET status=$2,validation_json=$3,error_code=$4,error_summary=$5,completed_at=$6 WHERE id=$1`, value.ID, value.Status, validation, value.ErrorCode, value.ErrorSummary, value.CompletedAt)
	if err != nil {
		return fmt.Errorf("update upgrade: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NewError(domain.ErrorNotFound, "upgrade was not found")
	}
	if err := audit(ctx, tx, auditEvent); err != nil {
		return err
	}
	if err := insertHAEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) UpgradeByID(ctx context.Context, id string) (haoperations.Upgrade, error) {
	return scanUpgrade(s.pool.QueryRow(ctx, upgradeSelect+` WHERE id=$1`, id))
}
func (s *Store) ListUpgrades(ctx context.Context, clusterID string, limit int) ([]haoperations.Upgrade, error) {
	rows, err := s.pool.Query(ctx, upgradeSelect+` WHERE cluster_id=$1 ORDER BY started_at DESC,id DESC LIMIT $2`, clusterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []haoperations.Upgrade{}
	for rows.Next() {
		value, scanErr := scanUpgrade(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

const upgradeSelect = `SELECT id,cluster_id,node_id,from_version,target_version,installation_type,mode,status,requested_by,request_id,preflight_json,validation_json,error_code,error_summary,started_at,completed_at FROM upgrade_operations`

func scanUpgrade(row rowScanner) (haoperations.Upgrade, error) {
	var value haoperations.Upgrade
	var preflight, validation []byte
	err := row.Scan(&value.ID, &value.ClusterID, &value.NodeID, &value.FromVersion, &value.TargetVersion, &value.InstallationType, &value.Mode, &value.Status, &value.RequestedBy, &value.RequestID, &preflight, &validation, &value.ErrorCode, &value.ErrorSummary, &value.StartedAt, &value.CompletedAt)
	if err != nil {
		return value, mapDatabaseError(err, "upgrade")
	}
	if err := json.Unmarshal(preflight, &value.Preflight); err != nil {
		return value, err
	}
	if err := json.Unmarshal(validation, &value.Validation); err != nil {
		return value, err
	}
	return value, nil
}

func (s *Store) ReleaseCache(ctx context.Context) (haoperations.ReleaseCache, error) {
	var value haoperations.ReleaseCache
	err := s.pool.QueryRow(ctx, `SELECT version,release_url,compatibility,checked_at,expires_at,error_code FROM upstream_release_cache WHERE product='adguard_home'`).Scan(&value.Version, &value.ReleaseURL, &value.Compatibility, &value.CheckedAt, &value.ExpiresAt, &value.ErrorCode)
	if err != nil {
		return value, mapDatabaseError(err, "upstream release cache")
	}
	return value, nil
}

func (s *Store) SaveReleaseCache(ctx context.Context, value haoperations.ReleaseCache) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO upstream_release_cache(product,version,release_url,compatibility,checked_at,expires_at,error_code) VALUES('adguard_home',$1,$2,$3,$4,$5,$6) ON CONFLICT(product) DO UPDATE SET version=EXCLUDED.version,release_url=EXCLUDED.release_url,compatibility=EXCLUDED.compatibility,checked_at=EXCLUDED.checked_at,expires_at=EXCLUDED.expires_at,error_code=EXCLUDED.error_code`, value.Version, value.ReleaseURL, value.Compatibility, value.CheckedAt, value.ExpiresAt, value.ErrorCode)
	return err
}

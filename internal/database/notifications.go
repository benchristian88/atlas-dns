package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
)

const notificationChannelSelect = `SELECT id,cluster_id,name,channel_type,enabled,destination_summary,record_version,
	created_at,updated_at,encrypted_destination,destination_nonce,destination_key_version,destination_algorithm
	FROM notification_channels`

func (s *Store) ListNotificationChannels(ctx context.Context, clusterID string) ([]haoperations.NotificationChannel, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,cluster_id,name,channel_type,enabled,destination_summary,record_version,created_at,updated_at
		FROM notification_channels WHERE cluster_id=$1 ORDER BY lower(name),id`, clusterID)
	if err != nil {
		return nil, fmt.Errorf("list notification channels: %w", err)
	}
	defer rows.Close()
	result := []haoperations.NotificationChannel{}
	for rows.Next() {
		var value haoperations.NotificationChannel
		if err := rows.Scan(&value.ID, &value.ClusterID, &value.Name, &value.ChannelType, &value.Enabled, &value.DestinationSummary, &value.RecordVersion, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		value.DestinationSet = true
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) NotificationChannelRecord(ctx context.Context, id string) (haoperations.NotificationChannelRecord, error) {
	var value haoperations.NotificationChannelRecord
	err := s.pool.QueryRow(ctx, notificationChannelSelect+` WHERE id=$1`, id).Scan(
		&value.Channel.ID, &value.Channel.ClusterID, &value.Channel.Name, &value.Channel.ChannelType,
		&value.Channel.Enabled, &value.Channel.DestinationSummary, &value.Channel.RecordVersion, &value.Channel.CreatedAt, &value.Channel.UpdatedAt,
		&value.Destination.Ciphertext, &value.Destination.Nonce, &value.Destination.KeyVersion, &value.Destination.Algorithm)
	if err != nil {
		return value, mapDatabaseError(err, "notification channel")
	}
	value.Channel.DestinationSet = true
	return value, nil
}

func (s *Store) SaveNotificationChannel(ctx context.Context, value haoperations.NotificationChannelRecord, expectedVersion int, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var affected int64
	if expectedVersion == 0 {
		tag, execErr := tx.Exec(ctx, `INSERT INTO notification_channels
			(id,cluster_id,name,channel_type,enabled,destination_summary,encrypted_destination,destination_nonce,destination_key_version,destination_algorithm,record_version,created_at,updated_at)
			VALUES($1,$2,$3,'webhook',$4,$5,$6,$7,$8,$9,1,$10,$10) ON CONFLICT DO NOTHING`, value.Channel.ID, value.Channel.ClusterID, value.Channel.Name, value.Channel.Enabled, value.Channel.DestinationSummary, value.Destination.Ciphertext, value.Destination.Nonce, value.Destination.KeyVersion, value.Destination.Algorithm, value.Channel.UpdatedAt)
		err, affected = execErr, tag.RowsAffected()
	} else {
		tag, execErr := tx.Exec(ctx, `UPDATE notification_channels SET name=$2,enabled=$3,destination_summary=$4,encrypted_destination=$5,
			destination_nonce=$6,destination_key_version=$7,destination_algorithm=$8,record_version=record_version+1,updated_at=$9
			WHERE id=$1 AND record_version=$10`, value.Channel.ID, value.Channel.Name, value.Channel.Enabled, value.Channel.DestinationSummary, value.Destination.Ciphertext, value.Destination.Nonce, value.Destination.KeyVersion, value.Destination.Algorithm, value.Channel.UpdatedAt, expectedVersion)
		err, affected = execErr, tag.RowsAffected()
	}
	if err != nil {
		return fmt.Errorf("save notification channel: %w", err)
	}
	if affected == 0 {
		return domain.NewError(domain.ErrorConflict, "notification channel was changed by another request")
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteNotificationChannel(ctx context.Context, id string, expectedVersion int, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `DELETE FROM notification_channels WHERE id=$1 AND record_version=$2`, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("delete notification channel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NewError(domain.ErrorConflict, "notification channel was changed by another request")
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ClaimNotificationDelivery(ctx context.Context, now time.Time) (haoperations.NotificationDelivery, haoperations.NotificationChannelRecord, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return haoperations.NotificationDelivery{}, haoperations.NotificationChannelRecord{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var delivery haoperations.NotificationDelivery
	var details []byte
	err = tx.QueryRow(ctx, `SELECT d.id,d.channel_id,d.event_id,d.status,d.attempt_count,d.error_code,d.error_summary,d.http_status,d.next_attempt_at,d.created_at,d.completed_at,
		e.id,e.cluster_id,e.node_id,e.event_type,e.severity,e.summary,e.details_json,e.occurred_at
		FROM notification_deliveries d JOIN ha_operational_events e ON e.id=d.event_id
		JOIN notification_channels c ON c.id=d.channel_id
		WHERE c.enabled AND d.status IN('pending','failed') AND d.attempt_count<5 AND COALESCE(d.next_attempt_at,d.created_at)<=$1
		ORDER BY COALESCE(d.next_attempt_at,d.created_at),d.id FOR UPDATE OF d SKIP LOCKED LIMIT 1`, now).Scan(
		&delivery.ID, &delivery.ChannelID, &delivery.EventID, &delivery.Status, &delivery.AttemptCount, &delivery.ErrorCode, &delivery.ErrorSummary, &delivery.HTTPStatus,
		&delivery.NextAttemptAt, &delivery.CreatedAt, &delivery.CompletedAt, &delivery.Event.ID, &delivery.Event.ClusterID,
		&delivery.Event.NodeID, &delivery.Event.EventType, &delivery.Event.Severity, &delivery.Event.Summary, &details, &delivery.Event.OccurredAt)
	if err != nil {
		return delivery, haoperations.NotificationChannelRecord{}, mapDatabaseError(err, "notification delivery")
	}
	if err := json.Unmarshal(details, &delivery.Event.Details); err != nil {
		return delivery, haoperations.NotificationChannelRecord{}, err
	}
	delivery.AttemptCount++
	delivery.Status = "pending"
	if _, err := tx.Exec(ctx, `UPDATE notification_deliveries SET status='pending',attempt_count=$2,next_attempt_at=NULL WHERE id=$1`, delivery.ID, delivery.AttemptCount); err != nil {
		return delivery, haoperations.NotificationChannelRecord{}, err
	}
	var channel haoperations.NotificationChannelRecord
	err = tx.QueryRow(ctx, notificationChannelSelect+` WHERE id=$1`, delivery.ChannelID).Scan(
		&channel.Channel.ID, &channel.Channel.ClusterID, &channel.Channel.Name, &channel.Channel.ChannelType, &channel.Channel.Enabled,
		&channel.Channel.DestinationSummary, &channel.Channel.RecordVersion, &channel.Channel.CreatedAt, &channel.Channel.UpdatedAt, &channel.Destination.Ciphertext,
		&channel.Destination.Nonce, &channel.Destination.KeyVersion, &channel.Destination.Algorithm)
	if err != nil {
		return delivery, channel, err
	}
	channel.Channel.DestinationSet = true
	if err := tx.Commit(ctx); err != nil {
		return delivery, channel, err
	}
	return delivery, channel, nil
}

func (s *Store) FinishNotificationDelivery(ctx context.Context, value haoperations.NotificationDelivery) error {
	_, err := s.pool.Exec(ctx, `UPDATE notification_deliveries SET status=$2,attempt_count=$3,error_code=$4,error_summary=$5,http_status=$6,next_attempt_at=$7,completed_at=$8 WHERE id=$1`, value.ID, value.Status, value.AttemptCount, value.ErrorCode, value.ErrorSummary, value.HTTPStatus, value.NextAttemptAt, value.CompletedAt)
	return err
}

func (s *Store) RecordNotificationTest(ctx context.Context, event haoperations.Event, delivery haoperations.NotificationDelivery, auditEvent domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin notification test history: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	details, err := json.Marshal(event.Details)
	if err != nil {
		return fmt.Errorf("encode notification test event: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ha_operational_events
		(id,cluster_id,node_id,event_type,severity,summary,details_json,occurred_at)
		VALUES($1,$2,NULL,'notification.test','info',$3,$4,$5)`, event.ID, event.ClusterID, event.Summary, details, event.OccurredAt); err != nil {
		return fmt.Errorf("insert notification test event: %w", err)
	}
	tag, err := tx.Exec(ctx, `INSERT INTO notification_deliveries
		(id,channel_id,event_id,status,attempt_count,error_code,error_summary,http_status,created_at,completed_at,channel_name)
		SELECT $1,c.id,$3,$4,$5,$6,$7,$8,$9,$10,c.name FROM notification_channels c WHERE c.id=$2`,
		delivery.ID, delivery.ChannelID, delivery.EventID, delivery.Status, delivery.AttemptCount, delivery.ErrorCode,
		delivery.ErrorSummary, delivery.HTTPStatus, delivery.CreatedAt, delivery.CompletedAt)
	if err != nil {
		return fmt.Errorf("insert notification test delivery: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NewError(domain.ErrorNotFound, "notification channel was not found")
	}
	if err := audit(ctx, tx, auditEvent); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ha_operational_events WHERE id IN
		(SELECT id FROM ha_operational_events WHERE occurred_at < $1 ORDER BY occurred_at,id LIMIT 10000)`, event.OccurredAt.Add(-365*24*time.Hour)); err != nil {
		return fmt.Errorf("clean notification test history: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit notification test history: %w", err)
	}
	return nil
}

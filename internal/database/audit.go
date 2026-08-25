package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	auditservice "github.com/benchristian88/atlas-dns/internal/audit"
	"github.com/benchristian88/atlas-dns/internal/domain"
)

const controllerAuditPredicate = `(a.action LIKE 'auth.%'
	OR a.action LIKE 'user.%'
	OR a.action LIKE 'system_settings.%'
	OR a.action LIKE 'operational_history.%'
	OR a.action LIKE 'backup.%'
	OR a.action LIKE 'controller.%'
	OR a.action = 'notification.policy_changed')`

const auditSelect = `SELECT a.id,a.actor_type,a.actor_user_id,COALESCE(u.display_name,''),a.action,
	a.resource_type,a.resource_id,a.request_id,a.metadata_json,a.created_at,
	CASE WHEN ` + controllerAuditPredicate + ` THEN 'controller' ELSE '' END
	FROM audit_events a LEFT JOIN users u ON u.id=a.actor_user_id`

func (s *Store) ListAuditEvents(ctx context.Context, query auditservice.Query) ([]domain.AuditEvent, error) {
	arguments := []any{}
	conditions := []string{}
	if query.ClusterID != "" {
		arguments = append(arguments, query.ClusterID)
		cluster := len(arguments)
		clusterPredicate := fmt.Sprintf(`(
			a.metadata_json->>'clusterId'=$%[1]d::text
			OR (a.resource_type='cluster' AND a.resource_id=$%[1]d::uuid)
			OR (a.resource_type='node' AND EXISTS(SELECT 1 FROM nodes n WHERE n.id=a.resource_id AND n.cluster_id=$%[1]d::uuid))
			OR (a.resource_type='configuration_draft' AND EXISTS(SELECT 1 FROM configuration_drafts d WHERE d.id=a.resource_id AND d.cluster_id=$%[1]d::uuid))
			OR (a.resource_type='configuration_revision' AND EXISTS(SELECT 1 FROM configuration_revisions r WHERE r.id=a.resource_id AND r.cluster_id=$%[1]d::uuid))
			OR (a.resource_type='deployment' AND EXISTS(SELECT 1 FROM deployments d WHERE d.id=a.resource_id AND d.cluster_id=$%[1]d::uuid))
			OR (a.resource_type='drift_event' AND EXISTS(SELECT 1 FROM drift_events d WHERE d.id=a.resource_id AND d.cluster_id=$%[1]d::uuid))
			OR (a.resource_type='notification_channel' AND EXISTS(SELECT 1 FROM notification_channels c WHERE c.id=a.resource_id AND c.cluster_id=$%[1]d::uuid))
			OR (a.resource_type='upgrade' AND EXISTS(SELECT 1 FROM upgrade_operations u WHERE u.id=a.resource_id AND u.cluster_id=$%[1]d::uuid))
			OR (a.resource_type='operational_command' AND EXISTS(SELECT 1 FROM operational_commands o WHERE o.id=a.resource_id AND o.cluster_id=$%[1]d::uuid))
		)`, cluster)
		if query.IncludeController {
			conditions = append(conditions, "("+clusterPredicate+" OR "+controllerAuditPredicate+")")
		} else {
			conditions = append(conditions, clusterPredicate)
		}
	}
	if query.BeforeAt != nil {
		arguments = append(arguments, *query.BeforeAt, query.BeforeID)
		conditions = append(conditions, fmt.Sprintf("(a.created_at,a.id) < ($%d,$%d::uuid)", len(arguments)-1, len(arguments)))
	}
	arguments = append(arguments, query.Limit)
	statement := auditSelect
	if len(conditions) > 0 {
		statement += " WHERE " + strings.Join(conditions, " AND ")
	}
	statement += fmt.Sprintf(" ORDER BY a.created_at DESC,a.id DESC LIMIT $%d", len(arguments))
	rows, err := s.pool.Query(ctx, statement, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()
	events := make([]domain.AuditEvent, 0)
	for rows.Next() {
		event, err := scanAuditEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, nil
}

func (s *Store) AuditEventByID(ctx context.Context, id string) (domain.AuditEvent, error) {
	event, err := scanAuditEvent(s.pool.QueryRow(ctx, auditSelect+` WHERE a.id=$1`, id))
	if err != nil {
		return domain.AuditEvent{}, mapDatabaseError(err, "audit event")
	}
	return event, nil
}

func scanAuditEvent(row rowScanner) (domain.AuditEvent, error) {
	var event domain.AuditEvent
	var metadata []byte
	if err := row.Scan(
		&event.ID, &event.ActorType, &event.ActorUserID, &event.ActorDisplayName,
		&event.Action, &event.ResourceType, &event.ResourceID, &event.RequestID,
		&metadata, &event.CreatedAt, &event.Scope,
	); err != nil {
		return domain.AuditEvent{}, fmt.Errorf("scan audit event: %w", err)
	}
	if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
		return domain.AuditEvent{}, fmt.Errorf("decode audit metadata: %w", err)
	}
	return event, nil
}

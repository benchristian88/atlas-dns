package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/controlplane"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

func (s *Store) UpdateConfigurationDraft(ctx context.Context, draft inventory.Draft, expectedVersion int, event domain.AuditEvent) error {
	document, err := json.Marshal(draft.Document)
	if err != nil {
		return fmt.Errorf("encode configuration draft: %w", err)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin draft update: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `UPDATE configuration_drafts SET document_json=$1,canonical_hash=$2,version=version+1,updated_by=$3,updated_at=$4 WHERE cluster_id=$5 AND version=$6`, document, draft.CanonicalHash, draft.UpdatedBy, draft.UpdatedAt, draft.ClusterID, expectedVersion)
	if err != nil {
		return fmt.Errorf("update configuration draft: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return domain.NewError(domain.ErrorConflict, "the configuration draft was changed by another request")
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) PublishRevision(ctx context.Context, draft inventory.Draft, revision *controlplane.Revision, expectedVersion int, event domain.AuditEvent) error {
	document, err := json.Marshal(revision.Document)
	if err != nil {
		return fmt.Errorf("encode configuration revision: %w", err)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin revision publication: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM clusters WHERE id=$1 FOR UPDATE`, revision.ClusterID); err != nil {
		return fmt.Errorf("lock cluster revision sequence: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(revision_number),0)+1 FROM configuration_revisions WHERE cluster_id=$1`, revision.ClusterID).Scan(&revision.RevisionNumber); err != nil {
		return fmt.Errorf("select revision number: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO configuration_revisions (id,cluster_id,revision_number,schema_version,document_json,canonical_hash,summary,created_by,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, revision.ID, revision.ClusterID, revision.RevisionNumber, revision.SchemaVersion, document, revision.CanonicalHash, revision.Summary, revision.CreatedBy, revision.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert configuration revision: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE configuration_drafts SET base_revision_id=$1,version=version+1,updated_by=$2,updated_at=$3 WHERE cluster_id=$4 AND version=$5`, revision.ID, revision.CreatedBy, revision.CreatedAt, revision.ClusterID, expectedVersion)
	if err != nil {
		return fmt.Errorf("base draft on revision: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return domain.NewError(domain.ErrorConflict, "the configuration draft was changed by another request")
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ListRevisions(ctx context.Context, clusterID string, includeArchived bool) ([]controlplane.Revision, error) {
	rows, err := s.pool.Query(ctx, revisionSelect+` WHERE r.cluster_id=$1 AND ($2 OR r.archived_at IS NULL) ORDER BY r.revision_number DESC`, clusterID, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("list revisions: %w", err)
	}
	defer rows.Close()
	items := []controlplane.Revision{}
	for rows.Next() {
		item, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) RevisionByID(ctx context.Context, id string) (controlplane.Revision, error) {
	item, err := scanRevision(s.pool.QueryRow(ctx, revisionSelect+` WHERE r.id=$1`, id))
	return item, mapDatabaseError(err, "configuration revision")
}

const revisionSelect = `SELECT r.id,r.cluster_id,r.revision_number,r.schema_version,
	r.document_json,r.canonical_hash,r.summary,r.created_by,r.created_at,
	COALESCE(c.active_revision_id=r.id,false),r.archived_at,r.archived_by,
	(c.active_revision_id IS DISTINCT FROM r.id
		AND NOT EXISTS(SELECT 1 FROM nodes n WHERE n.applied_revision_id=r.id)
		AND NOT EXISTS(SELECT 1 FROM configuration_drafts d WHERE d.base_revision_id=r.id)
		AND NOT EXISTS(SELECT 1 FROM deployments p WHERE p.revision_id=r.id OR p.rollback_of_revision_id=r.id)
		AND NOT EXISTS(SELECT 1 FROM drift_events e WHERE e.desired_revision_id=r.id))
	FROM configuration_revisions r JOIN clusters c ON c.id=r.cluster_id`

func scanRevision(row rowScanner) (controlplane.Revision, error) {
	var item controlplane.Revision
	var document []byte
	if err := row.Scan(&item.ID, &item.ClusterID, &item.RevisionNumber, &item.SchemaVersion, &document, &item.CanonicalHash, &item.Summary, &item.CreatedBy, &item.CreatedAt, &item.Active, &item.ArchivedAt, &item.ArchivedBy, &item.Lifecycle.CanDelete); err != nil {
		return item, err
	}
	if err := json.Unmarshal(document, &item.Document); err != nil {
		return item, fmt.Errorf("decode configuration revision: %w", err)
	}
	if item.SchemaVersion == configuration.LegacySchemaVersion || item.Document.SchemaVersion == configuration.LegacySchemaVersion {
		item.Document = configuration.ConvertLegacyDesired(item.Document)
		item.SchemaVersion = configuration.SchemaVersion
		if _, hash, err := configuration.MarshalDesired(item.Document); err == nil {
			item.CanonicalHash = hash
		}
	}
	item.Lifecycle.CanArchive = !item.Active && item.ArchivedAt == nil
	item.Lifecycle.CanRestore = item.ArchivedAt != nil
	return item, nil
}

func (s *Store) SetRevisionArchived(ctx context.Context, id, actorID string, archived bool, at time.Time, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var active bool
	var archivedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT c.active_revision_id=r.id,r.archived_at FROM configuration_revisions r JOIN clusters c ON c.id=r.cluster_id WHERE r.id=$1 FOR UPDATE OF r,c`, id).Scan(&active, &archivedAt)
	if err != nil {
		return mapDatabaseError(err, "configuration revision")
	}
	if archived {
		if active {
			return domain.NewError(domain.ErrorConflict, "the active revision cannot be archived")
		}
		if archivedAt != nil {
			return domain.NewError(domain.ErrorConflict, "the revision is already archived")
		}
		_, err = tx.Exec(ctx, `UPDATE configuration_revisions SET archived_at=$2,archived_by=$3 WHERE id=$1`, id, at, actorID)
	} else {
		if archivedAt == nil {
			return domain.NewError(domain.ErrorConflict, "the revision is not archived")
		}
		_, err = tx.Exec(ctx, `UPDATE configuration_revisions SET archived_at=NULL,archived_by=NULL WHERE id=$1`, id)
	}
	if err != nil {
		return fmt.Errorf("update revision archive state: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteUnusedRevision(ctx context.Context, id string, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var referenced bool
	err = tx.QueryRow(ctx, `SELECT
		(c.active_revision_id=r.id
		 OR EXISTS(SELECT 1 FROM nodes n WHERE n.applied_revision_id=r.id)
		 OR EXISTS(SELECT 1 FROM configuration_drafts d WHERE d.base_revision_id=r.id)
		 OR EXISTS(SELECT 1 FROM deployments p WHERE p.revision_id=r.id OR p.rollback_of_revision_id=r.id)
		 OR EXISTS(SELECT 1 FROM drift_events e WHERE e.desired_revision_id=r.id))
		FROM configuration_revisions r JOIN clusters c ON c.id=r.cluster_id
		WHERE r.id=$1 FOR UPDATE OF r,c`, id).Scan(&referenced)
	if err != nil {
		return mapDatabaseError(err, "configuration revision")
	}
	if referenced {
		return domain.NewError(domain.ErrorConflict, "the revision is active or retained by configuration, deployment, node, or drift history")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM configuration_revisions WHERE id=$1`, id); err != nil {
		return fmt.Errorf("delete unused revision: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CreateDeployment(ctx context.Context, deployment controlplane.Deployment, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin deployment creation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(ctx, `INSERT INTO deployments (id,cluster_id,revision_id,status,strategy,failure_policy,origin,rollback_of_revision_id,requested_by,request_id,requested_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, deployment.ID, deployment.ClusterID, deployment.RevisionID, deployment.Status, deployment.Strategy, deployment.FailurePolicy, deployment.Origin, deployment.RollbackOfRevisionID, deployment.RequestedBy, deployment.RequestID, deployment.RequestedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewError(domain.ErrorConflict, "the cluster already has an active deployment")
		}
		return fmt.Errorf("insert deployment: %w", err)
	}
	for _, node := range deployment.Nodes {
		if _, err := tx.Exec(ctx, `INSERT INTO deployment_nodes (id,deployment_id,node_id,position,effective_hash,status) VALUES ($1,$2,$3,$4,$5,$6)`, node.ID, deployment.ID, node.NodeID, node.Position, node.EffectiveHash, node.Status); err != nil {
			return fmt.Errorf("insert deployment node: %w", err)
		}
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ClusterHasActiveDeployment(ctx context.Context, clusterID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM deployments WHERE cluster_id=$1 AND status IN ('queued','validating','running','cancelling'))`, clusterID).Scan(&exists)
	return exists, err
}

func (s *Store) ListDeployments(ctx context.Context, clusterID string, includeArchived bool) ([]controlplane.Deployment, error) {
	rows, err := s.pool.Query(ctx, deploymentSelect+` WHERE d.cluster_id=$1 AND ($2 OR d.archived_at IS NULL) ORDER BY d.requested_at DESC`, clusterID, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	defer rows.Close()
	items := []controlplane.Deployment{}
	for rows.Next() {
		item, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) DeploymentByID(ctx context.Context, id string) (controlplane.Deployment, error) {
	item, err := scanDeployment(s.pool.QueryRow(ctx, deploymentSelect+` WHERE d.id=$1`, id))
	if err != nil {
		return item, mapDatabaseError(err, "deployment")
	}
	rows, err := s.pool.Query(ctx, `SELECT id,deployment_id,node_id,position,effective_hash,status,attempt_count,started_at,completed_at,error_code,error_message,verification_snapshot_id FROM deployment_nodes WHERE deployment_id=$1 ORDER BY position`, id)
	if err != nil {
		return item, fmt.Errorf("list deployment nodes: %w", err)
	}
	defer rows.Close()
	item.Nodes = []controlplane.DeploymentNode{}
	for rows.Next() {
		var node controlplane.DeploymentNode
		if err := rows.Scan(&node.ID, &node.DeploymentID, &node.NodeID, &node.Position, &node.EffectiveHash, &node.Status, &node.AttemptCount, &node.StartedAt, &node.CompletedAt, &node.ErrorCode, &node.ErrorMessage, &node.VerificationSnapshotID); err != nil {
			return item, fmt.Errorf("scan deployment node: %w", err)
		}
		item.Nodes = append(item.Nodes, node)
	}
	return item, rows.Err()
}

const deploymentSelect = `SELECT d.id,d.cluster_id,d.revision_id,d.status,d.strategy,d.failure_policy,d.origin,d.rollback_of_revision_id,d.requested_by,d.request_id,d.cancel_requested,d.error_code,d.requested_at,d.started_at,d.completed_at,d.archived_at,d.archived_by,
	(d.status='queued' AND d.started_at IS NULL
		AND NOT EXISTS(SELECT 1 FROM deployment_nodes n WHERE n.deployment_id=d.id AND (n.status<>'pending' OR n.attempt_count<>0 OR n.started_at IS NOT NULL))
		AND NOT EXISTS(SELECT 1 FROM drift_events e WHERE e.related_deployment_id=d.id))
	FROM deployments d`

func scanDeployment(row rowScanner) (controlplane.Deployment, error) {
	var item controlplane.Deployment
	err := row.Scan(&item.ID, &item.ClusterID, &item.RevisionID, &item.Status, &item.Strategy, &item.FailurePolicy, &item.Origin, &item.RollbackOfRevisionID, &item.RequestedBy, &item.RequestID, &item.CancelRequested, &item.ErrorCode, &item.RequestedAt, &item.StartedAt, &item.CompletedAt, &item.ArchivedAt, &item.ArchivedBy, &item.Lifecycle.CanDelete)
	terminal := item.Status == "partially_succeeded" || item.Status == "succeeded" || item.Status == "failed" || item.Status == "cancelled" || item.Status == "interrupted"
	item.Lifecycle.CanArchive = terminal && item.ArchivedAt == nil
	item.Lifecycle.CanRestore = item.ArchivedAt != nil
	return item, err
}

func (s *Store) SetDeploymentArchived(ctx context.Context, id, actorID string, archived bool, at time.Time, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var status string
	var archivedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT status,archived_at FROM deployments WHERE id=$1 FOR UPDATE`, id).Scan(&status, &archivedAt)
	if err != nil {
		return mapDatabaseError(err, "deployment")
	}
	if archived {
		terminal := status == "partially_succeeded" || status == "succeeded" || status == "failed" || status == "cancelled" || status == "interrupted"
		if !terminal {
			return domain.NewError(domain.ErrorConflict, "only terminal deployment history can be archived")
		}
		if archivedAt != nil {
			return domain.NewError(domain.ErrorConflict, "the deployment is already archived")
		}
		_, err = tx.Exec(ctx, `UPDATE deployments SET archived_at=$2,archived_by=$3 WHERE id=$1`, id, at, actorID)
	} else {
		if archivedAt == nil {
			return domain.NewError(domain.ErrorConflict, "the deployment is not archived")
		}
		_, err = tx.Exec(ctx, `UPDATE deployments SET archived_at=NULL,archived_by=NULL WHERE id=$1`, id)
	}
	if err != nil {
		return fmt.Errorf("update deployment archive state: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteUnstartedDeployment(ctx context.Context, id string, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT
		(d.status='queued' AND d.started_at IS NULL
		 AND NOT EXISTS(SELECT 1 FROM deployment_nodes n WHERE n.deployment_id=d.id AND (n.status<>'pending' OR n.attempt_count<>0 OR n.started_at IS NOT NULL))
		 AND NOT EXISTS(SELECT 1 FROM drift_events e WHERE e.related_deployment_id=d.id))
		FROM deployments d WHERE d.id=$1 FOR UPDATE`, id).Scan(&eligible)
	if err != nil {
		return mapDatabaseError(err, "deployment")
	}
	if !eligible {
		return domain.NewError(domain.ErrorConflict, "the deployment has started or is retained by operational history")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM deployment_nodes WHERE deployment_id=$1`, id); err != nil {
		return fmt.Errorf("delete unstarted deployment tasks: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM deployments WHERE id=$1`, id); err != nil {
		return fmt.Errorf("delete unstarted deployment: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RequestDeploymentCancel(ctx context.Context, id string, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `UPDATE deployments SET cancel_requested=true,status=CASE WHEN status='queued' THEN 'cancelling' ELSE status END WHERE id=$1 AND status IN ('queued','validating','running')`, id)
	if err != nil {
		return fmt.Errorf("request deployment cancellation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return domain.NewError(domain.ErrorConflict, "deployment is no longer cancellable")
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ClaimDeployment(ctx context.Context, at time.Time) (string, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM deployments WHERE status IN ('queued','cancelling') ORDER BY requested_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	_, err = tx.Exec(ctx, `UPDATE deployments SET status=CASE WHEN cancel_requested THEN 'cancelling' ELSE 'validating' END,started_at=COALESCE(started_at,$2) WHERE id=$1`, id, at)
	if err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

func (s *Store) UpdateDeploymentNode(ctx context.Context, node controlplane.DeploymentNode) error {
	_, err := s.pool.Exec(ctx, `UPDATE deployment_nodes SET status=$2,attempt_count=$3,started_at=$4,completed_at=$5,error_code=$6,error_message=$7,verification_snapshot_id=$8 WHERE id=$1`, node.ID, node.Status, node.AttemptCount, node.StartedAt, node.CompletedAt, node.ErrorCode, node.ErrorMessage, node.VerificationSnapshotID)
	return err
}

func (s *Store) SetDeploymentRunning(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE deployments SET status='running' WHERE id=$1 AND status='validating'`, id)
	return err
}

func (s *Store) MarkNodeApplied(ctx context.Context, nodeID, revisionID, hash string, snapshotID string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET applied_revision_id=$2,applied_hash=$3,convergence_status='converged',last_reconciled_at=$4,updated_at=$4 WHERE id=$1`, nodeID, revisionID, hash, at)
	return err
}

func (s *Store) FinishDeployment(ctx context.Context, deployment controlplane.Deployment, activate bool, event domain.AuditEvent) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(ctx, `UPDATE deployments SET status=$2,error_code=$3,completed_at=$4 WHERE id=$1`, deployment.ID, deployment.Status, deployment.ErrorCode, deployment.CompletedAt)
	if err != nil {
		return fmt.Errorf("finish deployment: %w", err)
	}
	if activate {
		if _, err := tx.Exec(ctx, `UPDATE clusters SET active_revision_id=$2,updated_at=$3 WHERE id=$1`, deployment.ClusterID, deployment.RevisionID, deployment.CompletedAt); err != nil {
			return fmt.Errorf("activate revision: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE drift_events SET reconciliation_status=CASE WHEN $2='succeeded' THEN 'enforcing' ELSE 'failed' END WHERE related_deployment_id=$1 AND status='open'`, deployment.ID, deployment.Status); err != nil {
		return fmt.Errorf("update deployment drift state: %w", err)
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) InterruptDeployments(ctx context.Context, at time.Time) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := tx.Query(ctx, `SELECT id,request_id,status FROM deployments WHERE status IN ('validating','running','cancelling') FOR UPDATE`)
	if err != nil {
		return err
	}
	type interruptedDeployment struct{ id, requestID, previousStatus string }
	interrupted := []interruptedDeployment{}
	for rows.Next() {
		var item interruptedDeployment
		if err := rows.Scan(&item.id, &item.requestID, &item.previousStatus); err != nil {
			rows.Close()
			return err
		}
		interrupted = append(interrupted, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if _, err := tx.Exec(ctx, `UPDATE deployment_nodes SET status='interrupted',completed_at=$1,error_code='CONTROLLER_RESTARTED',error_message='Controller restarted before this task completed.' WHERE status IN ('validating','applying','verifying')`, at); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE deployment_nodes SET status='skipped',completed_at=$1,error_code='DEPLOYMENT_INTERRUPTED' WHERE status='pending' AND deployment_id IN (SELECT id FROM deployments WHERE status IN ('validating','running','cancelling'))`, at); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE deployments SET status='interrupted',error_code='CONTROLLER_RESTARTED',completed_at=$1 WHERE status IN ('validating','running','cancelling')`, at)
	if err != nil {
		return err
	}
	for _, item := range interrupted {
		eventID, err := domain.NewID()
		if err != nil {
			return err
		}
		resourceID := item.id
		event := domain.AuditEvent{ID: eventID, ActorType: "system", Action: "deployment.interrupted", ResourceType: "deployment", ResourceID: &resourceID, RequestID: item.requestID, Metadata: map[string]any{"previousStatus": item.previousStatus, "reason": "controller_restart"}, CreatedAt: at}
		if err := audit(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListDriftEvents(ctx context.Context, clusterID string) ([]controlplane.DriftEvent, error) {
	rows, err := s.pool.Query(ctx, driftSelect+` WHERE cluster_id=$1 ORDER BY detected_at DESC`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []controlplane.DriftEvent{}
	for rows.Next() {
		item, err := scanDrift(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) DriftEventByID(ctx context.Context, id string) (controlplane.DriftEvent, error) {
	item, err := scanDrift(s.pool.QueryRow(ctx, driftSelect+` WHERE id=$1`, id))
	return item, mapDatabaseError(err, "drift event")
}

const driftSelect = `SELECT id,cluster_id,node_id,desired_revision_id,desired_hash,observed_snapshot_id,observed_hash,fingerprint,status,policy,reconciliation_status,diff_json,detected_at,last_seen_at,resolved_at,resolution,related_deployment_id FROM drift_events`

func scanDrift(row rowScanner) (controlplane.DriftEvent, error) {
	var item controlplane.DriftEvent
	var differences []byte
	if err := row.Scan(&item.ID, &item.ClusterID, &item.NodeID, &item.DesiredRevisionID, &item.DesiredHash, &item.ObservedSnapshotID, &item.ObservedHash, &item.Fingerprint, &item.Status, &item.Policy, &item.ReconciliationStatus, &differences, &item.DetectedAt, &item.LastSeenAt, &item.ResolvedAt, &item.Resolution, &item.RelatedDeploymentID); err != nil {
		return item, err
	}
	if err := json.Unmarshal(differences, &item.Differences); err != nil {
		return item, err
	}
	return item, nil
}

func (s *Store) UpsertDriftEvent(ctx context.Context, item controlplane.DriftEvent, event domain.AuditEvent) (controlplane.DriftEvent, bool, error) {
	differences, err := json.Marshal(item.Differences)
	if err != nil {
		return item, false, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return item, false, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var insertedID string
	err = tx.QueryRow(ctx, `INSERT INTO drift_events (id,cluster_id,node_id,desired_revision_id,desired_hash,observed_snapshot_id,observed_hash,fingerprint,status,policy,reconciliation_status,diff_json,detected_at,last_seen_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'open',$9,$10,$11,$12,$12) ON CONFLICT (node_id,fingerprint) WHERE status='open' DO NOTHING RETURNING id`, item.ID, item.ClusterID, item.NodeID, item.DesiredRevisionID, item.DesiredHash, item.ObservedSnapshotID, item.ObservedHash, item.Fingerprint, item.Policy, item.ReconciliationStatus, differences, item.DetectedAt).Scan(&insertedID)
	created := err == nil
	if err != nil && err != pgx.ErrNoRows {
		return item, false, err
	}
	if !created {
		if err := tx.QueryRow(ctx, `UPDATE drift_events SET observed_snapshot_id=$3,observed_hash=$4,last_seen_at=$5,policy=$6,diff_json=$7,reconciliation_status=CASE WHEN reconciliation_status='enforcing' THEN 'enforcing' WHEN $6='alert' THEN 'alerted' ELSE 'pending' END WHERE node_id=$1 AND fingerprint=$2 AND status='open' RETURNING id,cluster_id,node_id,desired_revision_id,desired_hash,observed_snapshot_id,observed_hash,fingerprint,status,policy,reconciliation_status,diff_json,detected_at,last_seen_at,resolved_at,resolution,related_deployment_id`, item.NodeID, item.Fingerprint, item.ObservedSnapshotID, item.ObservedHash, item.LastSeenAt, item.Policy, differences).Scan(&item.ID, &item.ClusterID, &item.NodeID, &item.DesiredRevisionID, &item.DesiredHash, &item.ObservedSnapshotID, &item.ObservedHash, &item.Fingerprint, &item.Status, &item.Policy, &item.ReconciliationStatus, &differences, &item.DetectedAt, &item.LastSeenAt, &item.ResolvedAt, &item.Resolution, &item.RelatedDeploymentID); err != nil {
			return item, false, err
		}
		if err := json.Unmarshal(differences, &item.Differences); err != nil {
			return item, false, err
		}
	} else if err := audit(ctx, tx, event); err != nil {
		return item, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return item, false, err
	}
	return item, created, nil
}

func (s *Store) ResolveNodeDrift(ctx context.Context, nodeID, resolution string, at time.Time, event domain.AuditEvent) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `UPDATE drift_events SET status='resolved',reconciliation_status='resolved',resolved_at=$3,resolution=$2 WHERE node_id=$1 AND status='open'`, nodeID, resolution, at)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, tx.Commit(ctx)
	}
	if err := audit(ctx, tx, event); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (s *Store) UpdateDriftReconciliation(ctx context.Context, id, status string, deploymentID *string) error {
	_, err := s.pool.Exec(ctx, `UPDATE drift_events SET reconciliation_status=$2,related_deployment_id=$3 WHERE id=$1`, id, status, deploymentID)
	return err
}

func (s *Store) UpdateNodeConvergence(ctx context.Context, nodeID, status string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET convergence_status=$2,last_reconciled_at=$3,updated_at=$3 WHERE id=$1`, nodeID, status, at)
	return err
}

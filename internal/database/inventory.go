package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

func (s *Store) SaveObservation(ctx context.Context, snapshot inventory.Snapshot, profile inventory.CapabilityProfile) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin observation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	features, err := json.Marshal(profile.Features)
	if err != nil {
		return err
	}
	warnings, err := json.Marshal(profile.Warnings)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO node_capability_profiles (node_id, product_version, api_compatibility, schema_version, features_json, warnings_json, refreshed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (node_id) DO UPDATE SET product_version=EXCLUDED.product_version, api_compatibility=EXCLUDED.api_compatibility, schema_version=EXCLUDED.schema_version, features_json=EXCLUDED.features_json, warnings_json=EXCLUDED.warnings_json, refreshed_at=EXCLUDED.refreshed_at`, profile.NodeID, profile.ProductVersion, profile.Compatibility, profile.SchemaVersion, features, warnings, profile.RefreshedAt)
	if err != nil {
		return fmt.Errorf("store capability profile: %w", err)
	}
	var document []byte
	if snapshot.Document != nil {
		document, err = json.Marshal(snapshot.Document)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO observed_snapshots (id,node_id,observed_at,schema_version,document_json,canonical_hash,node_version,collection_status,error_code) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, snapshot.ID, snapshot.NodeID, snapshot.ObservedAt, snapshot.SchemaVersion, document, nullableString(snapshot.CanonicalHash), snapshot.NodeVersion, snapshot.CollectionStatus, snapshot.ErrorCode)
	if err != nil {
		return fmt.Errorf("store observed snapshot: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit observation: %w", err)
	}
	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) LatestSnapshots(ctx context.Context, clusterID string) ([]inventory.Snapshot, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (o.node_id) o.id,o.node_id,o.observed_at,o.schema_version,o.document_json,o.canonical_hash,o.node_version,o.collection_status,o.error_code FROM observed_snapshots o JOIN nodes n ON n.id=o.node_id WHERE n.cluster_id=$1 AND n.deleted_at IS NULL ORDER BY o.node_id,o.observed_at DESC`, clusterID)
	if err != nil {
		return nil, fmt.Errorf("list latest snapshots: %w", err)
	}
	defer rows.Close()
	items := []inventory.Snapshot{}
	for rows.Next() {
		item, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) LatestSuccessfulSnapshots(ctx context.Context, clusterID string) ([]inventory.Snapshot, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (o.node_id) o.id,o.node_id,o.observed_at,o.schema_version,o.document_json,o.canonical_hash,o.node_version,o.collection_status,o.error_code FROM observed_snapshots o JOIN nodes n ON n.id=o.node_id WHERE n.cluster_id=$1 AND n.deleted_at IS NULL AND o.collection_status='succeeded' ORDER BY o.node_id,o.observed_at DESC`, clusterID)
	if err != nil {
		return nil, fmt.Errorf("list latest successful snapshots: %w", err)
	}
	defer rows.Close()
	items := []inventory.Snapshot{}
	for rows.Next() {
		item, scanErr := scanSnapshot(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanSnapshot(row rowScanner) (inventory.Snapshot, error) {
	var item inventory.Snapshot
	var document []byte
	var hash *string
	if err := row.Scan(&item.ID, &item.NodeID, &item.ObservedAt, &item.SchemaVersion, &document, &hash, &item.NodeVersion, &item.CollectionStatus, &item.ErrorCode); err != nil {
		return item, fmt.Errorf("scan snapshot: %w", err)
	}
	if hash != nil {
		item.CanonicalHash = *hash
	}
	if len(document) > 0 {
		var decoded configuration.Document
		if err := json.Unmarshal(document, &decoded); err != nil {
			return item, fmt.Errorf("decode snapshot: %w", err)
		}
		if item.SchemaVersion == configuration.LegacySchemaVersion || decoded.SchemaVersion == configuration.LegacySchemaVersion {
			decoded = configuration.ConvertLegacyDocument(decoded)
			item.SchemaVersion = configuration.SchemaVersion
		}
		item.Document = &decoded
	}
	return item, nil
}

func (s *Store) SnapshotByID(ctx context.Context, id string) (inventory.Snapshot, error) {
	item, err := scanSnapshot(s.pool.QueryRow(ctx, `SELECT id,node_id,observed_at,schema_version,document_json,canonical_hash,node_version,collection_status,error_code FROM observed_snapshots WHERE id=$1`, id))
	return item, mapDatabaseError(err, "snapshot")
}

func (s *Store) CapabilityProfiles(ctx context.Context, clusterID string) ([]inventory.CapabilityProfile, error) {
	rows, err := s.pool.Query(ctx, `SELECT c.node_id,c.product_version,c.api_compatibility,c.schema_version,c.features_json,c.warnings_json,c.refreshed_at FROM node_capability_profiles c JOIN nodes n ON n.id=c.node_id WHERE n.cluster_id=$1 AND n.deleted_at IS NULL ORDER BY lower(n.name)`, clusterID)
	if err != nil {
		return nil, fmt.Errorf("list capability profiles: %w", err)
	}
	defer rows.Close()
	items := []inventory.CapabilityProfile{}
	for rows.Next() {
		var item inventory.CapabilityProfile
		var features, warnings []byte
		if err := rows.Scan(&item.NodeID, &item.ProductVersion, &item.Compatibility, &item.SchemaVersion, &features, &warnings, &item.RefreshedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(features, &item.Features); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(warnings, &item.Warnings); err != nil {
			return nil, err
		}
		if item.SchemaVersion == configuration.LegacySchemaVersion {
			item.SchemaVersion = configuration.SchemaVersion
			item.Compatibility = string(domain.CompatibilityUnsupported)
			item.Warnings = append(item.Warnings, "This retained v1.0.x capability record requires a fresh schema-2 observation.")
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) DraftByCluster(ctx context.Context, clusterID string) (inventory.Draft, error) {
	var item inventory.Draft
	var document []byte
	err := s.pool.QueryRow(ctx, `SELECT id,cluster_id,source_snapshot_id,base_revision_id,schema_version,document_json,canonical_hash,version,updated_by,updated_at FROM configuration_drafts WHERE cluster_id=$1`, clusterID).Scan(&item.ID, &item.ClusterID, &item.SourceSnapshotID, &item.BaseRevisionID, &item.SchemaVersion, &document, &item.CanonicalHash, &item.Version, &item.UpdatedBy, &item.UpdatedAt)
	if err != nil {
		return item, mapDatabaseError(err, "configuration draft")
	}
	if err := json.Unmarshal(document, &item.Document); err != nil {
		return item, fmt.Errorf("decode configuration draft: %w", err)
	}
	if item.SchemaVersion == configuration.LegacySchemaVersion || item.Document.SchemaVersion == configuration.LegacySchemaVersion {
		item.Document = configuration.ConvertLegacyDesired(item.Document)
		item.SchemaVersion = configuration.SchemaVersion
		item.CanonicalHash = ""
	}
	if item.CanonicalHash == "" {
		_, item.CanonicalHash, err = configuration.MarshalDesired(item.Document)
		if err != nil {
			return item, fmt.Errorf("hash migrated configuration draft: %w", err)
		}
	}
	return item, nil
}

func (s *Store) ImportDraft(ctx context.Context, draft inventory.Draft, expectedVersion int, event domain.AuditEvent) error {
	document, err := json.Marshal(draft.Document)
	if err != nil {
		return err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var rowsAffected int64
	if expectedVersion == 0 {
		result, execErr := tx.Exec(ctx, `INSERT INTO configuration_drafts (id,cluster_id,source_snapshot_id,base_revision_id,schema_version,document_json,canonical_hash,version,updated_by,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,1,$8,$9) ON CONFLICT DO NOTHING`, draft.ID, draft.ClusterID, draft.SourceSnapshotID, draft.BaseRevisionID, draft.SchemaVersion, document, draft.CanonicalHash, draft.UpdatedBy, draft.UpdatedAt)
		err, rowsAffected = execErr, result.RowsAffected()
	} else {
		result, execErr := tx.Exec(ctx, `UPDATE configuration_drafts SET source_snapshot_id=$1,base_revision_id=$2,schema_version=$3,document_json=$4,canonical_hash=$5,version=version+1,updated_by=$6,updated_at=$7 WHERE cluster_id=$8 AND version=$9`, draft.SourceSnapshotID, draft.BaseRevisionID, draft.SchemaVersion, document, draft.CanonicalHash, draft.UpdatedBy, draft.UpdatedAt, draft.ClusterID, expectedVersion)
		err, rowsAffected = execErr, result.RowsAffected()
	}
	if err != nil {
		return fmt.Errorf("import configuration draft: %w", err)
	}
	if rowsAffected != 1 {
		return domain.NewError(domain.ErrorConflict, "the configuration draft was changed by another request")
	}
	if err := audit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/database"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

func TestRelease102ReappliesCurrentMigrationChainWithoutChangingLedger(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()

	var beforeCount, beforeVersion int
	if err := store.Pool().QueryRow(ctx, `SELECT count(*),max(version) FROM schema_migrations`).Scan(&beforeCount, &beforeVersion); err != nil {
		t.Fatal(err)
	}
	if beforeCount != 15 || beforeVersion != 15 {
		t.Fatalf("baseline ledger count/version = %d/%d, want 15/15", beforeCount, beforeVersion)
	}

	if err := database.ApplyMigrations(ctx, store.Pool()); err != nil {
		t.Fatalf("reapply unchanged v1.0.0 baseline: %v", err)
	}
	var afterCount, afterVersion int
	if err := store.Pool().QueryRow(ctx, `SELECT count(*),max(version) FROM schema_migrations`).Scan(&afterCount, &afterVersion); err != nil {
		t.Fatal(err)
	}
	if afterCount != beforeCount || afterVersion != beforeVersion {
		t.Fatalf("ledger changed from %d/%d to %d/%d", beforeCount, beforeVersion, afterCount, afterVersion)
	}
}

func TestRelease101NodeDeletionPrunesOnlyMutableOverride(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 16, 2, 0, 0, 0, time.UTC)
	const (
		userID                = "10100000-0000-4000-8000-000000000001"
		clusterID             = "10100000-0000-4000-8000-000000000002"
		nodeID                = "10100000-0000-4000-8000-000000000003"
		snapshotID            = "10100000-0000-4000-8000-000000000004"
		draftID               = "10100000-0000-4000-8000-000000000005"
		revisionID            = "10100000-0000-4000-8000-000000000006"
		auditID               = "10100000-0000-4000-8000-000000000007"
		replacementID         = "10100000-0000-4000-8000-000000000008"
		replacementSnapshotID = "10100000-0000-4000-8000-000000000009"
	)
	if _, err := store.Pool().Exec(ctx, `INSERT INTO users(id,email,display_name,password_hash,role,enabled,created_at,updated_at) VALUES($1,'release-101@example.test','Administrator','hash','administrator',true,$2,$2)`, userID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO clusters(id,name,created_at,updated_at) VALUES($1,'Replacement lifecycle',$2,$2)`, clusterID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO nodes(id,cluster_id,name,base_url,encrypted_credentials,credential_nonce,credential_key_version,credential_algorithm,certificate_policy,enabled,record_version,created_at,updated_at) VALUES($1,$2,'Old DNS','https://old-dns.example.test','ciphertext','nonce',1,'AES-256-GCM','system',true,1,$3,$3)`, nodeID, clusterID, now); err != nil {
		t.Fatal(err)
	}
	observed := `{"schemaVersion":2,"shared":{},"nodeSpecific":{"bindHosts":["192.0.2.10"],"dnsPort":53},"observedOnly":{},"unsupported":[]}`
	if _, err := store.Pool().Exec(ctx, `INSERT INTO observed_snapshots(id,node_id,observed_at,schema_version,document_json,canonical_hash,node_version,collection_status,error_code) VALUES($1,$2,$3,2,$4,'observed-hash','v0.107.78','succeeded','')`, snapshotID, nodeID, now, observed); err != nil {
		t.Fatal(err)
	}
	desired := `{"schemaVersion":2,"shared":{},"nodeOverrides":{"10100000-0000-4000-8000-000000000003":{"bindHosts":["192.0.2.10"],"dnsPort":53}},"unsupported":[]}`
	if _, err := store.Pool().Exec(ctx, `INSERT INTO configuration_drafts(id,cluster_id,source_snapshot_id,schema_version,document_json,canonical_hash,version,updated_by,updated_at) VALUES($1,$2,$3,2,$4,'draft-hash',4,$5,$6)`, draftID, clusterID, snapshotID, desired, userID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO configuration_revisions(id,cluster_id,revision_number,schema_version,document_json,canonical_hash,summary,created_by,created_at) VALUES($1,$2,1,2,$3,'revision-hash','Historical node identity',$4,$5)`, revisionID, clusterID, desired, userID, now); err != nil {
		t.Fatal(err)
	}
	resourceID := nodeID
	actorID := userID
	event := domain.AuditEvent{ID: auditID, ActorType: "user", ActorUserID: &actorID, Action: "node.removed", ResourceType: "node", ResourceID: &resourceID, RequestID: "release-1.0.1-delete", Metadata: map[string]any{"clusterId": clusterID}, CreatedAt: now.Add(time.Minute)}
	if err := store.SoftDeleteNode(ctx, nodeID, 1, now.Add(time.Minute), event); err != nil {
		t.Fatal(err)
	}

	var draftJSON, revisionJSON []byte
	var draftVersion int
	if err := store.Pool().QueryRow(ctx, `SELECT document_json,version FROM configuration_drafts WHERE id=$1`, draftID).Scan(&draftJSON, &draftVersion); err != nil {
		t.Fatal(err)
	}
	if err := store.Pool().QueryRow(ctx, `SELECT document_json FROM configuration_revisions WHERE id=$1`, revisionID).Scan(&revisionJSON); err != nil {
		t.Fatal(err)
	}
	var draftDocument, revisionDocument struct {
		NodeOverrides map[string]json.RawMessage `json:"nodeOverrides"`
	}
	if err := json.Unmarshal(draftJSON, &draftDocument); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(revisionJSON, &revisionDocument); err != nil {
		t.Fatal(err)
	}
	if _, stale := draftDocument.NodeOverrides[nodeID]; stale || draftVersion != 5 {
		t.Fatalf("mutable override stale=%v version=%d document=%s", stale, draftVersion, draftJSON)
	}
	if _, historical := revisionDocument.NodeOverrides[nodeID]; !historical {
		t.Fatalf("immutable revision lost historical node identity: %s", revisionJSON)
	}

	if _, err := store.Pool().Exec(ctx, `INSERT INTO nodes(id,cluster_id,name,base_url,encrypted_credentials,credential_nonce,credential_key_version,credential_algorithm,certificate_policy,enabled,record_version,created_at,updated_at) VALUES($1,$2,'Replacement DNS','https://old-dns.example.test','ciphertext','nonce',1,'AES-256-GCM','system',true,1,$3,$3)`, replacementID, clusterID, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	replacementObserved := `{"schemaVersion":2,"shared":{},"nodeSpecific":{"bindHosts":["192.0.2.20"],"dnsPort":53},"observedOnly":{},"unsupported":[]}`
	if _, err := store.Pool().Exec(ctx, `INSERT INTO observed_snapshots(id,node_id,observed_at,schema_version,document_json,canonical_hash,node_version,collection_status,error_code) VALUES($1,$2,$3,2,$4,'replacement-observed-hash','v0.107.78','succeeded','')`, replacementSnapshotID, replacementID, now.Add(2*time.Minute), replacementObserved); err != nil {
		t.Fatal(err)
	}
	importService := inventory.NewService(store, nil, nil)
	imported, err := importService.Import(ctx, domain.Actor{UserID: userID, RequestID: "10100000-0000-4000-8000-000000000010"}, clusterID, replacementSnapshotID, 5, true)
	if err != nil {
		t.Fatal(err)
	}
	if replacementID == nodeID || imported.Version != 6 || imported.Document.NodeOverrides[replacementID].DNSPort != 53 {
		t.Fatalf("replacement identity/draft not initialized: %#v", imported)
	}
	if _, stale := imported.Document.NodeOverrides[nodeID]; stale {
		t.Fatalf("deleted identity returned to mutable draft: %#v", imported.Document.NodeOverrides)
	}
}

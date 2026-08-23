# Database Design

## Database

PostgreSQL is the initial primary database.

The supported database compatibility baseline is Atlas DNS Controller v1.0.0.
Physically, a clean database reaches that baseline by applying the immutable
embedded `000001`–`000014` chain shipped in v1.0.0. The release-era names are
historical labels, not support promises for pre-1.0 databases. Release 1.0.1 is
schema-neutral. Release 1.0.2 appends migration `000015` without changing the
released baseline files.

Release 0.7 adds no parallel health tables. Durable node, observation,
Statistics, Query Log, deployment, and drift records remain authoritative;
short-lived worker-running state stays in process and becomes unknown after a
restart. Migration 000011 adds retained-time indexes so Operational Status can
read PostgreSQL relation estimates/sizes, pool counters, and indexed min/max
timestamps rather than exact full-table counts.

## Design principles

- UTC timestamps.
- UUID primary identifiers for exposed resources.
- Append-only immutable revisions and audit records.
- Explicit state transitions.
- Encrypted secret payloads.
- Normalised control-plane data.
- Partitionable high-volume event tables.
- Foreign-key integrity.
- Migration-based schema changes.

## Core tables

### Release 0.1 implemented schema

Migration `000001_release_0_1` creates only the foundation tables needed by the release:

- `users`
- `sessions`
- `clusters`
- `nodes`
- `audit_events`

The migration runner owns `schema_migrations`, including the version, name, SHA-256 checksum, and application time. Tables described later in this document are planned for their corresponding roadmap releases and do not exist in the 0.1 schema.

Migration `000002_release_0_2` adds `node_capability_profiles`, `observed_snapshots`, and `configuration_drafts`. Capability profiles are mutable current discovery results. Observation rows are immutable success or failure attempts. Drafts are mutable, optimistic, one-per-cluster inventory workspaces and are not revisions or desired state.

Migration `000003_release_0_3` makes drafts authoritative workspaces and adds immutable `configuration_revisions`, `deployments`, ordered `deployment_nodes`, and `drift_events`. Cluster rows hold `reconciliation_policy` and the active-revision relationship. Node rows hold maintenance, applied revision/hash, convergence, and last reconciliation state. Historical deployment, revision, snapshot, and drift relationships use restrictive foreign keys.

Migration `000004_release_0_4` widens the existing schema-version checks on capability profiles, observations, drafts, and revisions from exactly 1 to 1 or 2. It adds no parallel settings tables and does not rewrite immutable JSON or hashes. The development down migration refuses to restore the schema-v1-only checks while any schema-v2 record exists.

Release 0.1 mutable cluster and node records use integer optimistic versions. Node health updates do not increment the operator-facing `record_version`, so polling cannot create false edit conflicts.

### users

- id
- email
- display_name
- password_hash
- role
- enabled
- created_at
- updated_at
- last_login_at

### sessions

- id
- user_id
- token_hash
- created_at
- expires_at
- last_seen_at
- revoked_at
- ip_metadata
- user_agent

### clusters

- id
- name
- description
- reconciliation_policy
- active_revision_id
- created_at
- updated_at

### nodes

- id
- cluster_id
- name
- base_url
- encrypted_credentials
- certificate_policy
- enabled
- maintenance_mode
- applied_revision_id
- applied_hash
- convergence_status
- last_reconciled_at
- last_seen_at
- health_status
- version
- capabilities_json
- created_at
- updated_at

The implemented node credential envelope uses separate `encrypted_credentials`, `credential_nonce`, `credential_key_version`, and `credential_algorithm` columns. `custom_ca_pem` is write-only through the API. `deleted_at` preserves node identity after removal while credential and CA material are destroyed.

### configuration_drafts

- id
- cluster_id
- base_revision_id
- document_json
- version
- updated_by
- updated_at

### configuration_revisions

- id
- cluster_id
- revision_number
- schema_version
- document_json
- canonical_hash
- summary
- created_by
- created_at

### node_overrides

Node overrides may be embedded in the revision document initially. If independent querying or ownership becomes important, extract them into a separate revisioned table.

### observed_snapshots

- id
- node_id
- observed_at
- schema_version
- document_json
- canonical_hash
- node_version
- collection_status
- error_code

### deployments

- id
- cluster_id
- revision_id
- status
- strategy
- failure_policy
- origin
- rollback_of_revision_id
- requested_by
- request_id
- cancel_requested
- error_code
- requested_at
- started_at
- completed_at

### deployment_nodes

- id
- deployment_id
- node_id
- effective_hash
- position
- status
- attempt_count
- started_at
- completed_at
- error_code
- error_message
- verification_snapshot_id

### drift_events

- id
- cluster_id
- node_id
- desired_revision_id
- desired_hash
- observed_snapshot_id
- observed_hash
- status
- policy
- fingerprint
- reconciliation_status
- diff_json
- detected_at
- last_seen_at
- resolved_at
- resolution
- related_deployment_id

### audit_events

- id
- actor_type
- actor_user_id
- action
- resource_type
- resource_id
- request_id
- metadata_json
- created_at

### statistics_snapshots

- id
- node_id
- period_start
- period_end
- source_payload_json
- normalised_metrics_json
- collected_at

### query_events

- id
- cluster_id
- node_id
- source_timestamp
- ingested_at
- source_fingerprint and source_occurrence
- query_name
- query_type
- client_identifier, display name, and protocol
- response status and code
- processing milliseconds
- upstream
- filtering reason and service
- bounded rule and answer JSON arrays
- cache and answer-DNSSEC flags

### query_ingestion_checkpoints and attempts

- one checkpoint per node with cluster, high-water/source bounds, last
  attempt/success, safe state/error/gap, logging state, and node version;
- immutable UUID attempts with start/completion, status/error, bounded counts,
  page count, and gap evidence.

## Indexing

Initial indexes:

- users: unique lower(email).
- sessions: token_hash, expires_at.
- nodes: cluster_id, health_status.
- revisions: unique(cluster_id, revision_number).
- observations: node_id plus observed_at descending.
- deployments: cluster_id plus requested_at descending.
- deployment_nodes: deployment_id, node_id.
- drift: node_id plus detected_at descending, unresolved status.
- audit: created_at descending, resource lookup.
- query_events: node_id and source_timestamp.
- query_events: domain and source_timestamp.
- query_events: client address and source timestamp.

## High-volume strategy

Release 0.6 deliberately starts with one indexed PostgreSQL table rather than
premature partitions. Retention is short (seven days by default, 90-day hard
maximum), cleanup is bounded, list queries use keyset pagination, and source
inserts use one PostgreSQL batch per node poll. Time partitioning should be
introduced only with measured sustained volume and an append-only migration;
query-derived rollups and ClickHouse remain later scope.

## Secrets

Store node credentials as encrypted envelopes.

The database stores:

- Ciphertext.
- Nonce.
- Key version.
- Encryption metadata.

The encryption key is supplied through controller runtime configuration and is not stored in the database.

Release 0.1 uses AES-256-GCM. The node UUID is authenticated additional data, preventing an envelope copied between node records from decrypting successfully. Key version `1` is recorded for future explicit rotation support.

## Optimistic concurrency

Drafts and mutable settings should have a version integer or updated-at precondition.

An update with a stale version returns a conflict and does not overwrite another operator's work.

Release 0.8 deliberately adds parallel *DNS service* health evidence because a
successful management API request does not prove that the node answers DNS.
`node_lifecycle_settings` is mutable and optimistic;
`dns_probe_results`, `ha_operational_events`, and `upgrade_operations` are
durable operational history. `upstream_release_cache` bounds external lookups.
`notification_channels` stores only encrypted destinations and
`notification_deliveries` stores bounded state/error codes, safe HTTP status and
failure summaries, and an event/history lookup index. Node/history
foreign keys are restrictive; internal deliveries cascade when their channel is
removed or their one-year parent event expires.

Release 0.9 migration `000013_release_0_9_productisation` adds two singleton
tables. `controller_release_cache` contains bounded, non-authoritative stable
release metadata and may be excluded from Standard Backup. `system_settings`
contains the optimistic `update_checks_enabled` setting, record version, and
safe updater attribution; it is required control-plane recovery state. Existing
users remain the only authentication principals, and sessions remain transient
and are excluded from both Standard and Full portable recovery.

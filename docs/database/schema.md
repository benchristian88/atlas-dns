# Database Schema Reference

PostgreSQL 17 is the initial system of record. Identifiers exposed outside the
database are UUIDs, timestamps use `timestamptz`/UTC, and migrations are embedded
and applied under a PostgreSQL advisory lock. Applied migration checksums are
immutable.

## Record families

### Identity and audit

- `users` — local administrator identity, password hash, enabled state, version.
- `sessions` — server-side browser sessions; excluded from portable restore.
- `audit_events` — append-only actor/action/safe metadata/request evidence.

User records are retained for audit attribution. Passwords and raw sessions are
never audit metadata.

### Cluster, node, and capability

- `clusters` — desired-state/reconciliation scope and active revision pointer.
- `nodes` — node identity, encrypted credentials, maintenance, observation,
  applied revision, and convergence state.
- `node_capability_profiles` — version/schema-aware supported feature evidence.
- `observed_snapshots` — immutable normalized node configuration observations.

Encrypted credential envelopes and public endpoint identity remain distinct from
canonical configuration documents.

### Configuration and deployment

- `configuration_drafts` — one mutable optimistic desired-state document per
  cluster with optional base revision.
- `configuration_revisions` — immutable published desired state, origin/summary,
  archive time/actor, and monotonically increasing cluster revision number.
- `deployments` — durable requested strategy/status/revision/rollback and archive
  metadata.
- `deployment_nodes` — ordered per-node execution, safe error, observation, and
  verification evidence.
- `drift_events` — desired-versus-observed differences and optional related
  deployment.
- `operational_commands` and `operational_command_node_results` — non-revision
  commands such as guarded DHCP reset/filter refresh, with per-node evidence.

Desired configuration, observed configuration, applied revision, deployment
results, drift, and audit are never collapsed into one record.

Revision hard-delete eligibility rechecks every active/applied/base/deployment/
rollback/drift reference in one transaction. Deployment hard-delete eligibility
requires queued/no-start state, untouched node tasks, and no drift reference.
Archive preserves all foreign keys and immutable content.

### Telemetry and operational status

- `statistics_poll_attempts`, `statistics_snapshots`, `statistics_buckets` —
  bounded node-attributed collection evidence, exact results, and overlap-safe
  hourly/daily aggregates.
- `query_events`, `query_ingestion_checkpoints`, `query_ingestion_attempts` —
  normalized node-attributed events, restart cursor/coverage, and attempts.

Query events use node-scoped source fingerprint/occurrence identity and bounded
keyset/search indexes. Raw source payloads, credentials, and node URLs are not
stored. Operational-history retention is independent from configuration history.

### HA lifecycle and notifications

- `node_lifecycle_settings` and `dns_probe_results` — configurable active-DNS
  checks and immutable results.
- `ha_operational_events` — deduplicated transition evidence.
- `upgrade_operations` — durable operator-guided upgrade lifecycle.
- `upstream_release_cache` — bounded AdGuard Home release awareness.
- `notification_channels` — encrypted HTTPS destination, safe
  `destination_summary`, name, and enabled state.
- `notification_deliveries` — HA event delivery attempts with nullable channel
  reference, durable safe `channel_name` snapshot, bounded HTTP/failure
  diagnostics, and per-destination Test/real evidence.

Deleting a notification channel sets the delivery foreign key to null rather
than cascading operational history. HA event and audit records are unaffected.

### Product operations

- `controller_release_cache` — bounded controller release-awareness cache;
  excluded from portable restore.
- `system_settings` — singleton optimistic persistent product settings.

Portable backup metadata is produced outside persistent product tables. Standard
backup retains control-plane rows, including archive status; Full also retains
bounded operational-history data.

## JSON and indexing

JSONB is appropriate for versioned canonical configuration, capability
documents, structured diffs, bounded statistics vectors, and safe audit metadata.
It must not replace core relationships. Canonical documents carry explicit
schema versions and stable hashes; released schema-v1 records are not rewritten
when schema-v2 support is added.

Indexes support cluster/time listing, revision numbering/archive filters,
deployment status/archive filters, drift deduplication, retained-time cleanup,
Query Log keyset/trigram search, and worker claims. High-volume cleanup is
bounded to avoid long uninterruptible transactions.

## Migration ledger

The files below are development-era milestones that were all shipped unchanged
in v1.0.0. Every up file is embedded in the controller, applied during clean
bootstrap, and recorded by version/name/SHA-256 in `schema_migrations`.

| Version and filename | Current schema purpose | v1.0 decision |
|---|---|---|
| `000001_release_0_1` | Users, sessions, audit, clusters/nodes, encrypted credentials, health. | Retain; clean-bootstrap root and released checksum. |
| `000002_release_0_2` | Immutable observations, capabilities, and configuration inventory/draft. | Retain; bootstrap dependency and released checksum. |
| `000003_release_0_3` | Immutable revisions, deployments/results, drift, reconciliation state. | Retain; bootstrap dependency and released checksum. |
| `000004_release_0_4` | Canonical configuration schema-v2 capability. | Retain; bootstrap dependency and released checksum. |
| `000005_release_0_4_1_dhcp_operations` | Durable DHCP commands and per-node results. | Retain; bootstrap dependency and released checksum. |
| `000006_release_0_4_1_dns_operations` | Durable encrypted-input DNS command lifecycle. | Retain; bootstrap dependency and released checksum. |
| `000007_release_0_4_1_host_filter_operation` | Host-filter test command capability. | Retain; bootstrap dependency and released checksum. |
| `000008_release_0_4_1_policy_operations` | Query Log clear and Statistics reset commands. | Retain; bootstrap dependency and released checksum. |
| `000009_release_0_5_statistics` | Statistics attempts, snapshots, and buckets. | Retain; bootstrap dependency and released checksum. |
| `000010_release_0_6_query_log` | Query Log events, checkpoints, attempts, extension, and search indexes. | Retain; bootstrap dependency and released checksum. |
| `000011_release_0_7_operational_health` | Operational-health retained-time indexes. | Retain; bootstrap dependency and released checksum. |
| `000012_release_0_8_ha_operations` | DNS probes, HA events, lifecycle settings, guided upgrades, webhooks/deliveries. | Retain; bootstrap dependency and released checksum. |
| `000013_release_0_9_productisation` | Product settings and controller release cache. | Retain; bootstrap dependency and released checksum. |
| `000014_release_0_9_2_lifecycle_polish` | Revision/deployment archive metadata and retained webhook delivery identity. | Retain; final v1.0.0 schema and released checksum. |
| `000015_release_1_0_2_notification_history` | Bounded webhook HTTP/failure diagnostics and delivery-history query index. | Append-only v1.0.2 upgrade. |

The complete chain is the physical v1.0.0 baseline. Pre-1.0 databases are not
supported for in-place upgrade, but removing or squashing the chain would break
empty-database creation and v1.0.0 checksum recognition. Release 1.0.1 uses the
same schema and adds no migration. Release 1.0.2 appends `000015`. Future
schema-changing 1.x releases append new immutable, never-renumbered forward
migrations after the current highest version; schema-neutral patches do not add
placeholders.

Down migrations exist for development and controlled rollback analysis. They may
be destructive—especially where newer history cannot fit the older model—and
must not be run against retained production data without a documented,
preflighted recovery procedure.

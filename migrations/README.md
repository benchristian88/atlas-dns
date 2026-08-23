# Database Migrations

Ordered PostgreSQL migrations and their embedded Go filesystem live in this directory.

The controller applies pending migrations on startup when `AUTO_MIGRATE=true`. Operators can also run:

```bash
make migrate
make migrate-down # development only; rolls back one migration
```

Applied versions and SHA-256 checksums are stored in `schema_migrations`. A changed checksum causes startup to fail rather than silently accepting an edited released migration.

Do not edit a migration after it has been included in a released version.

## Stable baseline

Atlas DNS Controller 1.0.0 was released with, and clean databases are created
by, the complete embedded `000001` through `000014` up-migration chain. That
immutable chain is the physical v1.0.0 database baseline even though filenames
describe the pre-1.0 development milestones in which each table was introduced.
Existing 1.0.0 databases record every version and checksum, so these files must
not be removed, squashed, renumbered, or rewritten.

Pre-1.0 databases are not supported in-place upgrade sources. This does not make
the baseline files disposable: they are required for both clean 1.0.x bootstrap
and 1.0.0 ledger recognition. Release 1.0.1 changes no schema and therefore adds
no migration.

For future 1.x schema changes:

- add the next ordered forward migration after `000014`;
- never edit or renumber a migration shipped in a stable release;
- do not add empty migrations for schema-neutral releases; and
- keep downgrade and restore decisions explicit in release notes.

Released schema milestones:

- `000001_release_0_1`: authentication, clusters, nodes, encrypted credentials, and audit.
- `000002_release_0_2`: capabilities, immutable observations, and optimistic inventory drafts.
- `000003_release_0_3`: desired documents, immutable revisions, durable deployments, applied state, reconciliation, maintenance, and drift history.
- `000004_release_0_4`: schema-v2 configuration records while preserving immutable schema-v1 history.
- `000005_release_0_4_1_dhcp_operations`: durable, idempotent DHCP operational commands and per-node results.
- `000006_release_0_4_1_dns_operations`: encrypted-input, durable fleet DNS operational commands and per-node results.
- `000007_release_0_4_1_host_filter_operation`: encrypted-input, durable host-filtering tests and bounded per-node rule results.
- `000008_release_0_4_1_policy_operations`: confirmed Query Log clear and Statistics reset commands with durable per-node results.
- `000009_release_0_5_statistics`: normalized Statistics attempts, snapshots, and buckets.
- `000010_release_0_6_query_log`: normalized Query Log events, checkpoints, attempts, and search indexes.
- `000011_release_0_7_operational_health`: bounded retained-time metadata indexes for Operational Status and cleanup.
- `000012_release_0_8_ha_operations`: DNS probes, HA events, node lifecycle settings, guided upgrades, release awareness, and encrypted webhook delivery.
- `000013_release_0_9_productisation`: controller release cache and optimistic system settings.
- `000014_release_0_9_2_lifecycle_polish`: revision/deployment archive state and retained webhook delivery identity.
- `000015_release_1_0_2_notification_history`: bounded webhook HTTP/failure diagnostics and delivery-history query indexing.

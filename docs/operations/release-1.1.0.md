# Release 1.1.0 upgrade notes

Back up Atlas before upgrading. Release 1.1.0 applies forward-only migrations
`000016` and `000017`; no down migration is a supported production rollback.

On first startup, operational values from a v1.0.x environment are copied into
PostgreSQL if the corresponding database columns are not initialized. Preserve
the old environment for that first startup. Verify the values in **System →
System Settings**, then remove the deprecated variables from normal `.env`
management. Subsequent environment edits do not override database values.

All v1.0.x persisted data is retained. Historical schema-1 JSON and hashes stay
immutable in PostgreSQL, but the 1.1 API reads them through a one-way schema-2
migration representation marked non-deployable. Import a fresh observation and
publish a new schema-2 revision before deployment. AdGuard Home older than
0.107.78 is unsupported; upgrade each node before Atlas can observe or mutate
managed configuration.

Existing webhook channels, encrypted destinations, and category subscriptions
are preserved. The new global notification policy starts with conservative
failure/recovery/redundancy defaults and does not replay historical events.

Standard Backup includes System Settings and notification policy because both
are required control-plane state. It continues to exclude Operational History
events/deliveries, Statistics, Query Log events, DNS probes, and sessions. Full
Backup includes retained operational data. Audit Log, revisions, deployments,
drift, and upgrade records are unaffected by Operational History retention or
the clear action.

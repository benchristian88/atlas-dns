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

Certificate expiry monitoring now treats a successful AdGuard observation with
TLS encryption disabled as `not_applicable`. AdGuard's zero/default certificate
timestamps are no longer classified as expired, so intentionally non-TLS nodes
do not add Dashboard Attention, HA certificate alerts, operational events,
webhook deliveries, or false recovery transitions. Enabled TLS remains
fail-closed for missing, invalid, malformed, or expired certificate evidence.

Standard Backup includes System Settings and notification policy because both
are required control-plane state. It continues to exclude Operational History
events/deliveries, Statistics, Query Log events, DNS probes, and sessions. Full
Backup includes retained operational data. Audit Log, revisions, deployments,
drift, and upgrade records are unaffected by Operational History retention or
the clear action.

The v1.1 administration UI now enforces canonical action ownership. Notifications
is the only webhook/event-policy management page; HA Operations retains delivery
and webhook-test evidence with partial-source warnings. Node Detail owns
existing-node connection tests and maintenance entry/return, while Nodes keeps
inventory/create/edit/delete/candidate validation and Drift keeps
restore/adopt/reconciliation. Query Log and Drift now link to exact Node Detail
routes.

Operational Status is labelled Monitoring and uses compact accessible section
headers plus a Dashboard-style responsive health summary. HA Operations, Nodes,
and Node Detail use the same compact health-card anatomy for their top-level
evidence. System Settings removes non-setting General, Backup & Restore,
Operations, and Security cards while retaining every runtime setting and the
release-check control. About Atlas Project now precedes technical build data.

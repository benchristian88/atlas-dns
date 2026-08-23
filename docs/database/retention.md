# Data Retention

## Control-plane records

Retain indefinitely by default:

- Revisions.
- Deployments.
- Drift events.
- Audit events.

Allow explicit administrative cleanup only in a later release.

## Observed snapshots

Suggested default:

- Full snapshots: 30 days.
- Daily convergence summaries: 1 year.

## Statistics

Release 0.5 enforced default:

- Normalized node snapshots and poll attempts: 32 days.
- Hourly node-attributed buckets: 32 days.
- Daily node-attributed rollups: 400 days.

Cleanup rolls completed hourly days into daily buckets before expiry. Raw node
responses are never stored. Custom operator retention controls remain later
work; changing the node's own statistics retention may reduce which exact
windows it can supply.

Release 0.7 bounds each Statistics delete to 10,000 rows per table/pass. The
Operational Status page reports estimated relation rows/total bytes and oldest
and newest snapshots from PostgreSQL metadata and bounded aggregate queries.

## Query events

Release 0.6 enforces a separate central retention window. Collection defaults
to enabled and normalized raw events default to seven days. Operators may set
`QUERY_LOG_COLLECTION_ENABLED=false` without deleting retained data and may set
`QUERY_LOG_RETENTION` from one hour through 90 days. These settings never alter
schema-v2 node-local query-log policy.

Each poll deletes at most 10,000 expired events and 10,000 expired ingestion
attempts; attempt evidence uses a fixed 32-day window. Cleanup failure is logged
and does not block ingestion. Query-derived rollups are intentionally not part
of Release 0.6.

Release 0.7 exposes cleanup worker success/failure and the configured central
retention without changing node-local Query Log policy. The environment value
remains the supported control because controller runtime settings are not yet
stored as mutable database resources; a UI setting would introduce a second
configuration authority. Use autovacuum and regular `ANALYZE`; do not schedule
manual `VACUUM FULL` in normal operation. Monitor table and index growth,
autovacuum lag, and backup duration as event volume grows.

## HA lifecycle evidence

DNS probe samples are retained for 30 days and transition-oriented HA events
and their delivery attempts for 365 days. Cleanup deletes at most 10,000 events
in a pass and cascades only their internal deliveries. Upgrade operations and
audits are not automatically shortened in Release 0.8 because they are
operator/action evidence; capacity must be reviewed before introducing a
destructive policy. Probe rows contain no query answers or
client activity—the configured test name, credentials, and raw packets are not
persisted.

Webhook Test results use the same parent-event retention and one delivery row
per selected destination. Delivery rows add bounded scalar diagnostics only;
remote response bodies and destination URLs are never retained. Operational
History keyset pages and the event lookup index avoid unbounded browser or
database result sets as this append-heavy history grows.

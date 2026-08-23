# Operational Health

Operational health uses one presentation-ready, authenticated cluster endpoint:

```text
GET /api/v1/clusters/{clusterId}/operational-status
```

The endpoint validates cluster scope and combines existing durable evidence:
node status, latest immutable observations, Statistics poll attempts, Query Log
checkpoints/gaps, PostgreSQL metadata, and bounded process-local worker state.
It does not store or return credentials, URLs, query contents, raw errors, or
stack traces.

## State model and aggregation

The shared states are `healthy`, `degraded`, `stale`, `failed`, `paused`,
`unsupported`, `maintenance`, and `unknown`. Existing Statistics and Query Log
coverage vocabularies remain unchanged on their feature APIs; operational
health maps them into this common presentation vocabulary.

PostgreSQL unavailability makes readiness and overall operational state fail.
A stale/failed node or collector degrades the overall controller but does not
fail liveness and does not imply DNS failure. Paused, unsupported, and
maintenance subsystems remain explicit and do not alone declare the whole
controller failed. Three consecutive process worker failures mark that worker
failed; a successful run clears its streak and safe error code.

Statistics staleness reuses `max(2 * STATISTICS_POLL_INTERVAL +
NODE_REQUEST_TIMEOUT, 3h)`. Query Log staleness reuses
`max(3 * QUERY_LOG_POLL_INTERVAL, 2m)`. Observation freshness uses
`max(3 * NODE_HEALTH_INTERVAL + NODE_REQUEST_TIMEOUT, 2m)` and remains distinct
from the node connectivity timestamp.

Statistics health is based on ranges eligible under each node's current
retention. A successful 24-hour read on a node configured for 24 hours is
healthy even though 7d and 30d reports identify that node with
`STATISTICS_RANGE_EXCEEDS_NODE_RETENTION`. Query Log root (`.`) questions are
valid records and do not create malformed-record gaps. A genuine skipped
record remains a degraded data-quality state even when the rest of the poll
succeeds.

A maintenance Statistics attempt is an intentional skipped poll. After the
node returns to service, operational health continues using its most recent
successful attempt while that success remains inside the normal Statistics
staleness window. With no prior success, or once that success becomes stale,
the node remains non-current until a scheduled collection succeeds. A partial
node attempt counts toward coverage but propagates `degraded` to the collection
and overall controller summary.

`Unsupported` requires evidence: a version below the capability floor or an
endpoint that returns its documented not-found/not-implemented response.
Unknown API generations and reachability, authentication, TLS, timeout, or
response-validation failures remain unknown/failed. A newer compatible v0.107
patch is not made unsupported solely by its version number.

## Worker and retry behavior

Workers are context-cancellable, run one pass at a time, and isolate nodes
behind a four-request concurrency bound. Node requests keep their configured
timeout. Collectors retry failed nodes on the next bounded scheduled pass;
deployment and operational-command claim failures use cancellable exponential
backoff from one to 30 seconds. Success resets worker degradation. Retention
failure does not stop its collector.

The worker tracker is intentionally process-local. Durable collector attempts,
checkpoints, deployments, drift, and observations remain the restart source of
truth. Immediately after restart a process worker is `unknown` until it runs.

## Liveness, readiness, detailed status, and metrics

- `GET /health`: public liveness; the HTTP process can respond.
- `GET /ready`: public readiness; PostgreSQL responds within two seconds.
- detailed operational status: authenticated cluster diagnostics.
- `GET /metrics`: disabled unless `METRICS_BEARER_TOKEN` is configured; when
  enabled it requires that bearer token and exposes Prometheus worker counters
  and gauges with only a bounded worker-name label.

Metrics never label or contain domains, clients, users, request IDs, node IDs,
error text, credentials, or tokens.

Routine polls, worker transitions, and cleanup passes are operational evidence
and logs, not audit events. Audit remains reserved for authenticated security,
configuration, deployment, rollback, credential, and node-management actions.

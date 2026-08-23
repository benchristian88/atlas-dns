# ADR-0028: Aggregate exact node statistics in the controller

## Status

Accepted.

## Context

Each AdGuard Home node owns its live DNS counters and serves DNS independently.
Operators need honest cluster totals and node attribution without placing the
controller in the DNS path or deriving traffic metrics from query logs.

AdGuard Home statistics are cumulative over a requested recent window. The
`recent` request parameter needed for exact 24-hour, 7-day, and 30-day windows
is present in the tested v0.107.72–v0.107.78 API contract. Earlier supported
configuration contracts expose statistics but cannot guarantee those exact
windows. Additive counters can be summed; averages and percentages cannot.

v1.0.2 extends explicit test evidence through v0.107.79 and treats later
v0.107 patches as provisionally capable when the same typed configuration and
statistics response contracts validate. It does not infer support across a
different API generation.

## Decision

The controller will:

- poll each enabled node immediately at startup and every hour by default;
- request exact 24-hour, 7-day, and 30-day windows directly from the node;
- cap concurrent node collection at four, apply the configured node request
  timeout, wait for a pass to finish before starting another, and skip
  maintenance nodes;
- record every node poll as a small durable attempt with a stable status and
  safe error code;
- persist immutable normalized snapshots and node-attributed time buckets, not
  raw node response bodies;
- upsert buckets by node, resolution, and source start so overlapping windows
  do not double-count durable series data;
- sum additive counters, derive percentages from aggregate numerators and
  denominators, weight processing time by DNS query count, and weight upstream
  latency by upstream response count;
- expose coverage, freshness, missing, stale, maintenance, and unsupported-node
  state beside every aggregated result;
- exclude contracts without exact-range support instead of presenting an
  approximate historical window as exact; and
- keep all query-log ingestion outside this pipeline.

Domains are normalized by trimming whitespace, lower-casing, and removing one
terminal dot. IP clients use canonical IP spelling; other client identifiers
retain case and are merged only when their trimmed identifiers match exactly.
Rankings sort by value descending and normalized key ascending.

Periodic collection evidence is not duplicated into the human audit-event
stream. `statistics_poll_attempts` is the operational record; structured logs
report storage and cleanup failures without credentials, URLs, or response
bodies.

## Consequences

- DNS service remains available when the controller or PostgreSQL is offline.
- An outage pauses collection. Restart performs an immediate poll and can
  recover whatever history remains inside each node's statistics retention,
  but cannot recreate history already discarded by the node.
- Older nodes remain configuration-manageable while showing an explicit
  statistics capability exclusion.
- The snapshot API is presentation-ready and does not require the browser to
  reproduce aggregation mathematics.
- Custom ranges require a later API, storage, and UX decision.
- PostgreSQL capacity now includes bounded telemetry data: snapshots, attempts,
  and hourly buckets retain 32 days; daily buckets retain 400 days.

## Alternatives considered

- Summing node averages or percentages: rejected because it is mathematically
  invalid when node traffic volumes differ.
- Polling only the node-configured default window: rejected because the UI
  could not truthfully label fixed historical ranges.
- Computing Release 0.5 statistics from query logs: rejected because it couples
  aggregate visibility to the separate, more privacy-sensitive Release 0.6
  ingestion system.
- Putting statistics polling in a separate service: deferred because the
  bounded worker is small, uses the same credentials and adapter boundary, and
  does not affect DNS availability.

# Statistics Aggregation

Atlas DNS Controller provides controller-owned, read-only statistics collection. It never
proxies DNS, mutates AdGuard Home statistics, or reads query-log events.

## Collection lifecycle

The combined controller process starts the statistics worker after PostgreSQL,
credentials, and the node adapter are ready. It runs one immediate pass, then
runs at `STATISTICS_POLL_INTERVAL` (default `1h`). Up to four nodes are polled
concurrently. A node's eligible range reads are sequential and bounded by
`NODE_REQUEST_TIMEOUT`; the next pass starts only after the current pass ends.

Eligible nodes are enabled, outside maintenance, and report v0.107.72 or later
in the v0.107 API generation with exact `recent` statistics support. v0.107.78
and v0.107.79 are explicitly tested; newer v0.107 patches must pass the same
typed response validation. Before
requesting data, the worker reads that node's current `stats/config` interval.
It requests only the fixed `24h`, `7d`, and `30d` ranges that fit within the
node-local retention boundary; AdGuard Home rejects a `recent` value greater
than that interval. A 24-hour node therefore has one eligible range, and a
successful `1/1` pass is operationally healthy rather than partial.

Longer fixed ranges remain explicit on the Statistics API. When a requested
range exceeds a node's retention, that node is missing with
`STATISTICS_RANGE_EXCEEDS_NODE_RETENTION`; Atlas DNS Controller does not manufacture longer
history from a shorter snapshot. `expected_ranges` on durable poll attempts is
the number eligible for that node during that pass, while `range_errors` also
records the safe reason for configured-but-ineligible ranges.

The first data normally appears after the startup pass completes; operators do
not need to wait one full polling interval. The snapshot reflects the history
currently retained by AdGuard Home, not traffic observed by the controller
since installation.

## Persistence

Migration `000009_release_0_5_statistics` creates three distinct stores:

- `statistics_poll_attempts`: one safe operational result per node pass;
- `statistics_snapshots`: immutable normalized totals, rankings, and source
  series per node and exact range; and
- `statistics_buckets`: node-attributed hourly/daily additive counters keyed by
  source bucket, preventing overlap double-counting.

Raw AdGuard Home JSON, credentials, authorization headers, node URLs, and
query-log events are not stored. Ranked arrays are bounded to 100 source items;
series are bounded to 1,000 points and validated for matching lengths,
non-negative counters, supported time units, and finite averages.

Cleanup runs after each collection pass. Snapshots, attempts, and hourly
buckets retain 32 days. Days with all 24 hourly buckets are rolled up before deletion;
daily buckets retain 400 days. Control-plane revisions, deployments, drift,
and audit retention are unchanged.

## Aggregation rules

For the latest snapshot per included node and requested range:

- DNS, filtering, Safe Browsing, Safe Search, and parental counters are summed;
- blocked and safety percentages use cluster totals as numerator and
  denominator;
- average processing seconds are weighted by each node's DNS query count;
- upstream average seconds are weighted by that upstream's response count;
- time-series buckets with the same source timestamp are summed; and
- rankings merge normalized keys, then use value-descending/key-ascending
  ordering for stable results.

The API returns milliseconds for latency presentation and exposes node-level
contribution, collection time, reason codes, stale state, and aggregate
coverage. Stale snapshots remain visible and attributed; the report becomes
partial rather than silently dropping their known totals.

## Failure and gap behavior

Authentication, TLS, timeout, network, capability, invalid-configuration, and
invalid-response failures become stable attempt error codes. The worker logs
only node IDs, ranges, and safe codes for range failures, never secrets, URLs,
ranked values, or response bodies. A failure in one node or eligible range does
not stop other nodes or already successful ranges. A configured retention
boundary is not a failed request and does not degrade collector health.

Maintenance creates a `maintenance` attempt and no node request. Versions
without exact-range support create an `unsupported` attempt. Disabled nodes
are excluded from expected coverage. Controller downtime creates a collection
gap; the immediate restart poll can recover only the recent history the node
still retains. Statistics reset on a node creates a genuine discontinuity and
is not interpolated.

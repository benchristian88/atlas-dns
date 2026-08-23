# Query Ingestion

Atlas DNS Controller implements bounded controller-side API polling. Query events are
observational data: they never enter desired documents, revision hashes,
deployment verification, or drift comparison, and the controller remains
outside the DNS request path.

## Supported source contract

Atlas DNS Controller accepts AdGuard Home v0.107.52 and later patches in the
v0.107 API generation. v0.107.78 and v0.107.79 are explicitly tested; newer
v0.107 patches use the same bounded typed contract provisionally. It
reads `GET /control/querylog` newest-first with a maximum page size of 500,
using the response `oldest` timestamp as the next request's `older_than`
cursor. The first request omits `older_than`; `search` is empty and
`response_status=all` because filtering belongs to the central API. Atlas DNS Controller does
not use offsets against the changing live log.

The adapter normalizes timestamp, question, type, client/client ID and optional
display name, protocol, response/filtering status, processing milliseconds,
upstream, reason/service, bounded rules and answers, cache, and DNSSEC flags.
The DNS root question name `.` is preserved as a valid domain; ordinary fully
qualified names have one trailing dot removed before case normalization.
Malformed or oversized records are skipped and recorded as an ingestion gap,
with only the node ID and invalid-record count written to safe warning logs.
Legacy `querylog_info` and current `querylog/config` are used only to determine
whether logging is enabled and whether client addresses are anonymized. Atlas DNS Controller
preserves the anonymized identity exactly as received and never reverses it.

The node API supplies no stable event identifier or deterministic event cursor.
Its cursor is timestamp-based, and entries can disappear under node retention
or a clear operation. These limitations are surfaced rather than hidden.

## Polling and checkpoints

The worker runs immediately and then at `QUERY_LOG_POLL_INTERVAL` (30 seconds
by default), with at most four node requests active concurrently. Each enabled
node is handled independently with the configured node timeout. Maintenance,
unsupported, unreachable, authentication, disabled-query-log, malformed-source,
and timeout outcomes become safe per-node attempt/checkpoint evidence.

An old version below v0.107.52 or a proven missing Query Log endpoint is
`QUERY_LOG_UNSUPPORTED`. A malformed/different API generation or a network,
TLS, authentication, timeout, or decode failure is failed/unknown rather than
misreported as unsupported.

Each pass begins at the newest event and follows `oldest`/`older_than` for at
most 20 pages. It stops after exhausting the source or crossing two poll
intervals behind the durable high-water timestamp, with a minimum two-minute
overlap. The checkpoint stores the high-water timestamp, source newest/oldest,
last attempt/success, node version, logging state, safe error, and gap state.
Restart therefore repeats a bounded overlap and relies on database uniqueness;
no in-memory cursor is required for correctness.

Atlas DNS Controller detects and presents malformed records, missing/stalled cursors, a source
window larger than the 10,000-record pass bound, an empty/reset source after a
previous checkpoint, clock regression, and loss caused by node-local retention. A failed
poll never removes earlier central events and never affects DNS service.

## Identity and deduplication

The SHA-256 source fingerprint covers stable normalized source fields:
timestamp, client identifier/protocol, query and type, status/code, processing
time, upstream, filtering reason/service, bounded rules/answers, cache, and
DNSSEC. Mutable client display-name enrichment is deliberately excluded.

Because indistinguishable legitimate events can share every source field, Atlas DNS Controller
assigns their order-of-observation occurrence within a pass. Database uniqueness
is `(node_id, source_fingerprint, source_occurrence)`. Repeated overlap is
discarded while multiple identical events remain represented. If AdGuard changes
the relative order of completely indistinguishable records, perfect identity is
impossible; this conservative design prefers retaining a legitimate event over
aggressive collapse.

## Central API and presentation

Authenticated browser requests use only controller endpoints. The cluster route
supports node scope, bounded substring domain/client search, exact client,
normalized response status and observed query-type filters, and newest-first
keyset pagination on `(source_timestamp, id)`. Coverage reports expected,
included, stale, unsupported, maintenance, logging-disabled, failed, and known-
gap nodes plus the oldest common current-through timestamp.

The `/query-log` page follows global cluster/node scope, keeps Node visible in
every row, debounces search, keeps filters while paging, supports Previous by a
browser cursor stack, and does not reorder an older page or open detail. Detail
shows normalized explanation fields and links to existing rule, rewrite, client,
node, and Configuration Control workflows. Contextual actions only propose a
mutable draft change; saving, publication, and deployment remain explicit.

## Configuration and retention

- `QUERY_LOG_COLLECTION_ENABLED` defaults to `true` and can stop new ingestion
  without deleting retained events.
- `QUERY_LOG_POLL_INTERVAL` defaults to `30s`, bounded from 5 seconds to 1 hour.
- `QUERY_LOG_RETENTION` defaults to `168h` (seven days), bounded from 1 hour to
  90 days.

Cleanup removes at most 10,000 expired events and attempts per pass. Failure is
logged without blocking later polling. Node-local policy remains schema-v2
desired state and is not changed by these controller settings.

At approximate normalized row and index overhead of 1–3 KiB per event, one
million retained events should be budgeted at roughly 1–3 GiB before PostgreSQL
vacuum/headroom. Operators must monitor database growth and shorten retention
or increase polling capacity before sustained input reaches the 10,000-record
per-node/per-pass bound.

## Conditional forwarder

ADR-0029 gives a local forwarder no assigned release. Stable file
identity/offset, rotation handling, disk spooling, compression, and stronger
delivery justify it only if measurements show the supported API path cannot
meet operational requirements. There is no hidden alternate transport.

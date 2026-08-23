# Controller API

## Stable API boundary

The controller serves its browser UI and JSON API from the same origin. JSON routes are versioned under:

```text
/api/v1
```

`GET /health` and `GET /ready` are intentionally unversioned operational probes.

## Administration and recovery

All routes below require an enabled administrator session. Mutations also
require the existing same-origin CSRF token.

- `GET /api/v1/users`, `POST /api/v1/users`, and
  `PATCH /api/v1/users/{userId}` list, create, and
  enable/disable local administrators. Responses omit password hashes and
  session material. The only accepted role is `administrator`.
- `POST /api/v1/users/{userId}/password-reset` replaces the Argon2id credential and
  revokes every target session. No credential is returned.
- `POST /api/v1/system/backups` accepts `{type, passphrase}` and streams a Standard or
  Full `.atlasdnsbackup`. Passphrases are transient. Archive creation is audited.
- `POST /api/v1/system/restore-preflight` accepts bounded multipart `archive` and
  `passphrase` fields, authenticates/validates without mutation, and returns the
  manifest and offline restore plan. Restore execution has no web endpoint.
- `GET /api/v1/system/update` returns cached controller release status and host-guided
  update instructions; `POST /api/v1/system/update/check` forces a bounded refresh.
- `GET/PATCH /api/v1/system/settings` exposes the justified persisted release-check
  setting plus read-only retention and installation facts with optimistic
  `recordVersion`.
- `GET /api/v1/system/version` returns application version, commit, build time,
  development state, and current database schema version.

Every response includes `X-Request-ID`. API responses use `Cache-Control: no-store` and standard browser security headers.

## Authentication and CSRF

Successful setup or login creates:

- `atlas_dns_session`: opaque, HTTP-only, SameSite=Strict session cookie;
- `atlas_dns_csrf`: opaque, SameSite=Strict CSRF cookie readable by the UI.

Cookies are marked Secure when `PUBLIC_BASE_URL` uses HTTPS. Authenticated `POST`, `PATCH`, and `DELETE` requests must copy the CSRF cookie into `X-CSRF-Token`. Only HMAC-SHA-256 hashes of both tokens are stored.

## Errors

```json
{
  "error": {
    "code": "NODE_UNREACHABLE",
    "message": "The AdGuard Home node could not be reached.",
    "field": "baseUrl",
    "requestId": "9ed26873-80f8-4e88-af55-53249687428e"
  }
}
```

`field` is present only for field-specific validation. Stable categories include validation, not found, conflict, authentication, authorisation, rate limiting, node authentication, node TLS, node unreachable, invalid node response, and internal failure.

## Public setup and authentication routes

```text
GET  /api/v1/setup/status
POST /api/v1/setup
POST /api/v1/auth/login
POST /api/v1/auth/logout
GET  /api/v1/auth/me
```

Setup status reports whether setup is required, the configured public URL, controller time, cookie security mode, and prerequisite checks. Initial setup is serialized in PostgreSQL and cannot be repeated after the first user exists.

Login is rate-limited by source address and normalized account identifier. Login failures do not reveal whether an account exists.

## Cluster routes

```text
GET   /api/v1/clusters
POST  /api/v1/clusters
GET   /api/v1/clusters/{clusterId}
PATCH /api/v1/clusters/{clusterId}
```

Create accepts `name` and `description`. Update accepts the complete mutable representation plus `version`. A stale version returns HTTP 409. Responses include an integer `version` and an ETag.

Cluster deletion is deliberately not exposed because safe behavior for
historical configuration, deployment, and audit relationships has no supported
hard-delete contract.

## Statistics route

```text
GET /api/v1/clusters/{clusterId}/statistics?range=24h|7d|30d&nodeId={optional UUID}&limit=1..25
```

The authenticated read defaults to `range=24h` and `limit=10`. Omitting
`nodeId` returns the selected cluster aggregate; supplying a node UUID returns
the same presentation contract for that node after verifying cluster
membership. Invalid ranges, limits, and identifiers return stable validation
errors.

The response contains `state`, scope, generation/freshness timestamps,
coverage counts, node-attributed coverage rows, summed totals, derived
percentages, query-weighted average processing milliseconds, chronological
series, and bounded rankings for queried/blocked domains, clients, upstream
responses, and response-weighted upstream average latency. `state` is `ready`,
`partial`, or `unavailable`; consumers must present coverage and must not infer
that missing nodes contributed zero traffic.

Only controller-collected normalized statistics are returned. The route does
not read query logs, call nodes synchronously, expose node credentials/URLs, or
return raw AdGuard Home payloads. Responses retain the API-wide `no-store`
cache policy. Node coverage uses `STATISTICS_RANGE_EXCEEDS_NODE_RETENTION` when
the selected fixed range is longer than that node's configured statistics
interval; this is an explicit unavailable range, not a zero contribution or an
unhealthy eligible-range poll.

## Query-event routes

```text
GET /api/v1/clusters/{clusterId}/query-events
GET /api/v1/clusters/{clusterId}/query-events/{eventId}
```

Both reads require the existing authenticated administrator session and are
strictly cluster-scoped. The list accepts optional `nodeId`, `cursor`, `search`,
`status`, `queryType`, and exact `client`, plus `limit=1..100` (default 50).
Search is bounded to 256 characters and matches normalized domain, client
identifier, or centrally available display name using parameterized SQL. A node
filter is rejected unless that node belongs to the path cluster.

Results are ordered by `(timestamp DESC, id DESC)`. `nextCursor` is an opaque
base64url controller cursor containing a version, source timestamp, and UUID;
clients must not inspect or modify it. Changing any filter resets the cursor.
The response contains normalized events, observed query-type/status options,
generation time, and node-attributed coverage including explicit global
collection state, stale/unsupported/maintenance/logging-disabled/error/gap
counts, configured `retentionSeconds`, per-node evidence, and common
current-through time. It never returns
node URLs, credentials, raw node payloads, or internal database details.

The detail route verifies both cluster and event UUID and returns the same
stable controller-domain event shape with bounded rules and answers. Neither
route contacts an AdGuard Home node synchronously. API-wide `no-store`, request
IDs, safe errors, and session authorization apply.

## Node routes

```text
GET    /api/v1/clusters/{clusterId}/nodes
POST   /api/v1/clusters/{clusterId}/nodes
GET    /api/v1/nodes/{nodeId}
PATCH  /api/v1/nodes/{nodeId}
DELETE /api/v1/nodes/{nodeId}
POST   /api/v1/nodes/{nodeId}/test-connection
POST   /api/v1/nodes/{nodeId}/filter-refresh
GET    /api/v1/nodes/{nodeId}/dhcp/interfaces
POST   /api/v1/nodes/{nodeId}/dhcp/active-check
```

Create and update use:

```json
{
  "name": "Node A",
  "baseUrl": "https://node-a.example.test",
  "certificatePolicy": "system",
  "customCaPem": "optional write-only PEM",
  "credentials": {
    "username": "write-only",
    "password": "write-only"
  },
  "enabled": true,
  "recordVersion": 1
}
```

`credentials` are required on create and optional on update. If supplied on update, username and password must be supplied together and the action is audited as credential rotation. `customCaPem` is required when first selecting `custom_ca`, is write-only, and may be omitted on later updates to retain it.

Before saving an enabled node, the controller verifies status, authentication,
TLS policy, version, and DNS running state. Configuration changes occur only
through explicit deployment resources.

Node responses include identity, URL, trust policy, enabled state, health, compatibility, version, polling timestamps, latency, safe error code, and `recordVersion`. They never include credentials, ciphertext, nonces, CA contents, or authentication headers. The cluster node-list envelope also includes `refreshedAt` and `staleAfterSeconds`; the latter is three configured health intervals so the UI freshness state follows runtime polling configuration.

The manual `test-connection` action atomically stores its safe observed result and an audit event with a success or failure outcome. Background interval polls update health without generating high-volume audit records.

The DHCP interface route performs a bounded, authenticated, node-specific read
and returns safe interface names, hardware addresses, IP addresses, gateway,
flags, controller-derived availability, and `fetchedAt`. The result is volatile
presentation metadata: it is not stored in desired configuration, revisions,
canonical hashes, or drift state. An unavailable or malformed AdGuard endpoint
returns a stable capability or node-response error; a legacy desired interface
remains the draft value.

The active-DHCP route accepts `{ "interfaceName": "eth0" }` and invokes the
node-specific, non-mutating AdGuard detection operation. It returns aggregate
`none`, `found`, `multiple`, `partial`, or `error` status plus safe IPv4,
IPv4-static-IP, and IPv6 results and `checkedAt`. Raw upstream error text is
discarded. The operation requires CSRF, an enabled node outside maintenance,
and records `dhcp.active_check_requested` followed by exactly one
`dhcp.active_check_succeeded` or `dhcp.active_check_failed` audit event. It
never changes the draft, publishes, deploys, or selects a DHCP node.

Node removal requires `recordVersion` and `confirmName`. It soft-removes the record, disables polling, destroys the stored encrypted credential and CA material, and writes an audit record.

## Configuration inventory routes

```text
POST /api/v1/nodes/{nodeId}/observations
GET  /api/v1/clusters/{clusterId}/configuration-inventory
GET  /api/v1/clusters/{clusterId}/blocklists/presentation
GET  /api/v1/clusters/{clusterId}/allowlists/presentation
GET  /api/v1/clusters/{clusterId}/blocked-services/catalogue
GET  /api/v1/configuration-comparisons?leftSnapshotId={uuid}&rightSnapshotId={uuid}
POST /api/v1/clusters/{clusterId}/configuration-draft/import
PUT  /api/v1/clusters/{clusterId}/configuration-draft
POST /api/v1/clusters/{clusterId}/configuration-draft/validate
```

Observation performs bounded, authenticated GET requests and stores either an immutable canonical schema-v1/v2 snapshot or an immutable failed attempt with a safe error code. v0.107.52 remains schema v1; v0.107.53 and later patches in the v0.107 API generation use schema v2. v0.107.78 and v0.107.79 are explicitly tested; newer v0.107 patches are provisionally compatible only after the typed endpoints Atlas uses validate. Other API generations report unknown compatibility. Inventory returns the latest attempt for each node, current capability profiles, current schema version, and the optional cluster draft. The `draft` member is omitted when no draft exists. Comparison returns `equal` plus differences grouped by section, field, and `shared_managed`, `node_specific_managed`, `observed_only`, or `unsupported` scope.

Import accepts `snapshotId`, `expectedVersion`, and `confirmed: true`. It rejects failed snapshots, cross-cluster snapshots, missing confirmation, and stale draft versions. The transaction updates the draft and writes `configuration.draft_imported`. It never publishes or deploys configuration.

Draft update accepts `expectedVersion` and a complete schema-v2 desired `document`. It saves canonical mutable intent and returns validation issues. Frozen schema-v1 drafts must be refreshed/imported before editing or publication; historical v1 revisions remain deployable and reconcilable. Validation returns the same fleet feature/listener/DHCP preflight used by publication and deployment.

The blocked-services catalogue route reads observed metadata through the controller and never mutates desired state. It returns the union of stable service IDs and names, optional group IDs, per-service supported/unsupported node IDs, per-node `available`, `stale`, `error`, or `unsupported` state, and response freshness. Upstream filtering rules and SVG icons are removed at the adapter boundary. Node URLs, credentials, and raw node errors are never returned. Metadata is cached per node version/capability signature for 15 minutes; version/capability changes force refresh, and an expired matching cache entry is exposed as stale only when a refresh fails.

The blocklist and allowlist presentation routes read `GET /control/filtering/status` through the controller and return category-specific, node-attributed filter ID, enabled state, name, rule count, last-update time, portability classification, fetch time, and safe node result. They include disabled node entries so the UI can explain disable-oriented reconciliation. Separate per-category cache keys prevent blocklist and allowlist results from crossing. A successful value is cached only as a stale fallback for a later failed read. These fields are volatile presentation metadata: they are not written to observed configuration documents, drafts, revisions, canonical hashes, verification hashes, or drift comparison.

Selected IDs remain the canonical `shared.services.blockedServiceIds` set. Save Draft preserves unknown legacy IDs. Validation, publication, and deployment preflight require every selected ID to be present in every enabled node's current catalogue and return node-attributed issues otherwise. Catalogue names, groups, counts, and freshness never participate in revision or drift hashes.

`POST /api/v1/nodes/{nodeId}/filter-refresh` accepts `{ "whitelist": false }` (or true), requires an enabled node outside maintenance, and returns the node/list type with `status: "succeeded"`. The controller records `filters.refresh_requested` before the node call and exactly one `filters.refresh_succeeded` or `filters.refresh_failed` terminal event. Fleet fan-out is performed by the UI so partial node outcomes remain explicit.

AdGuard Home's filter-refresh contract selects only blocklists versus allowlists; it does not accept URLs or filter IDs. The controller therefore does not expose a selected-row refresh operation. Both filter-list pages identify that capability boundary instead of presenting a fleet-wide refresh as a targeted action.

## Revision, deployment, and drift routes

```text
GET  /api/v1/clusters/{clusterId}/configuration-revisions?includeArchived=false
POST /api/v1/clusters/{clusterId}/configuration-revisions
GET  /api/v1/configuration-revisions/{revisionId}
POST /api/v1/configuration-revisions/{revisionId}/archive
POST /api/v1/configuration-revisions/{revisionId}/restore
DELETE /api/v1/configuration-revisions/{revisionId}
GET  /api/v1/configuration-revision-comparisons?leftRevisionId={uuid}&rightRevisionId={uuid}
POST /api/v1/clusters/{clusterId}/configuration-revisions/{revisionId}/deployment-preview
POST /api/v1/clusters/{clusterId}/configuration-revisions/{revisionId}/deployments
POST /api/v1/clusters/{clusterId}/configuration-revisions/{revisionId}/rollback
GET  /api/v1/clusters/{clusterId}/deployments?includeArchived=false
GET  /api/v1/deployments/{deploymentId}
POST /api/v1/deployments/{deploymentId}/cancel
POST /api/v1/deployments/{deploymentId}/archive
POST /api/v1/deployments/{deploymentId}/restore
DELETE /api/v1/deployments/{deploymentId}
GET  /api/v1/clusters/{clusterId}/drift-events
POST /api/v1/drift-events/{driftId}/restore
POST /api/v1/drift-events/{driftId}/adopt
POST /api/v1/nodes/{nodeId}/maintenance
```

Publication requires a non-empty summary and the current draft version. Preview returns structured semantic changes from the active revision, ordered affected nodes/effective hashes, capability or listener issues, strategy/failure policy, and whether a restart is required (false for schema v1). Deployment creation returns HTTP 202 and a durable queued resource; per-node task details expose only safe errors and verification snapshot identifiers. Cancellation is a request honored at a safe node boundary. Rollback requires explicit confirmation and creates a deployment of a historical immutable revision. Drift restore creates a targeted deployment; adoption writes the observed shared state and node override into the optimistic draft but still requires publication and normal deployment.

Lists hide archived records unless `includeArchived=true`. Revision/deployment
responses include immutable archive metadata and server-derived lifecycle
eligibility. Archive/restore accepts `{ "confirmed": true }` and requires an
administrator plus CSRF. A revision can be archived only when it is not active;
a deployment can be archived only in a terminal state.

Hard deletion is deliberately narrower. Revision DELETE accepts
`{ "confirmation": "DELETE REVISION #<number>" }` and succeeds only when the
locked revision is inactive, never deployed, and unreferenced. Deployment DELETE
accepts `{ "confirmation": "DELETE DEPLOYMENT <full-uuid>" }` and succeeds only
when the locked deployment is queued, never started, all node tasks are untouched,
and no drift record references it. Conflicts use stable safe errors. Every
archive, restore, and delete attempt that reaches the service authorization
boundary is audited; the UI is never authoritative about eligibility.

## Audit and version routes

```text
GET /api/v1/audit-events?limit=50&offset=0
GET /api/v1/system/version
```

Audit pagination is bounded to 100 records per request. Audit metadata excludes secrets.

## DHCP operational commands

```text
POST /api/v1/nodes/{nodeId}/dhcp/reset-leases
POST /api/v1/nodes/{nodeId}/dhcp/reset-configuration
GET  /api/v1/nodes/{nodeId}/dhcp/operations?limit=10
```

Both POST routes require authentication, CSRF, an explicit UUID node path, and
a UUID `Idempotency-Key` header. Their JSON bodies contain only the fixed
controller confirmation token: `RESET_LEASES` or
`RESET_DHCP_CONFIGURATION`. The target must be enabled, in maintenance mode,
reachable through its configured trust/authentication policy, and in a cluster
without an active deployment. Configuration reset additionally requires Manual
or Alert reconciliation so a later drift mismatch cannot be silently Enforced;
lease reset changes observed-only data. No fleet DHCP reset route exists.

The controller invokes exactly one no-body AdGuard request:
`POST /control/dhcp/reset_leases` or `POST /control/dhcp/reset`. A terminal
duplicate idempotency key returns the original persistent result without a
second node call. Results contain cluster and node identity, per-node status,
stable safe errors, request ID, terminal audit reference, observation outcome,
and timestamps. They never contain node URLs, credentials, upstream bodies, or
raw upstream errors.

Successful commands immediately create a normal fresh observation. Dynamic
lease changes remain observed-only. A configuration reset does not mutate the
draft or active revision; its observed mismatch remains subject to the existing
restore/adopt drift workflow after the maintenance boundary is reviewed.

## Audited DNS and filtering operational commands

```text
POST /api/v1/clusters/{clusterId}/operational-commands/test-upstream-dns
POST /api/v1/clusters/{clusterId}/operational-commands/test-host-filtering
POST /api/v1/clusters/{clusterId}/operational-commands/clear-dns-cache
POST /api/v1/clusters/{clusterId}/operational-commands/clear-query-log
POST /api/v1/clusters/{clusterId}/operational-commands/reset-statistics
GET  /api/v1/operational-commands/{operationId}
GET  /api/v1/clusters/{clusterId}/operational-commands?command={type}&limit=10
```

POST requests require authentication, CSRF, and a UUID `Idempotency-Key`.
Their target is either `{ "scope": "node", "nodeId": "uuid" }` or the
explicit `{ "scope": "all_compatible_enabled_nodes" }`. Fleet targets are
resolved once when the durable command is created. Disabled nodes are not
targeted; incompatible or maintenance nodes are reported as exclusions.

Upstream tests accept the currently displayed draft resolver fields and draft
version. Values are validated, AES-256-GCM encrypted while queued, and
discarded at terminal completion. Results identify resolvers as `upstream-1`,
`upstream-2`, and so on; raw resolver text and raw AdGuard errors are not
persisted in result or audit DTOs.

Host-filter tests accept:

```json
{
  "target": { "scope": "node", "nodeId": "uuid" },
  "input": {
    "hostname": "ads.example",
    "client": "192.0.2.10",
    "queryType": "AAAA"
  }
}
```

`client` and `queryType` are optional. Hostname-only checks use the filtering
capability available throughout the supported inventory range. Contextual
checks require AdGuard Home v0.107.58 or newer; fleet scope records older nodes
as capability exclusions and selected-node scope rejects an incompatible
target before queueing. Results contain only bounded reason, matched rule text
and filter-list ID, blocked-service name, rewrite CNAME, and IP-address fields.
The hostname/client input, raw node response, node URL, credentials, and raw
node error are absent from audit and result resources.

Cache clear requires `CLEAR_DNS_CACHE`. A successful node call is followed by a
normal configuration observation. Observation failure is reported separately
and does not change command success. None of these operations mutates a draft,
revision, deployment, or active-revision pointer.

Query Log clear requires the exact `CLEAR_QUERY_LOG` confirmation and invokes
one no-body `POST /control/querylog_clear` per frozen target. Statistics reset
requires `RESET_STATISTICS` and invokes one no-body
`POST /control/stats_reset` per target. Both default to explicit node scope in
the UI; fleet scope is accepted only as
`all_compatible_enabled_nodes`. Successful nodes receive a normal fresh
configuration observation so the unchanged enabled/retention/ignored-domain
policy remains coherent. Observation failure is reported separately from the
destructive command result.

These commands permanently remove node-local operational data. They do not
change Query Log or Statistics desired configuration, create a revision, start
a deployment, ingest query records or statistics, or adopt observed state.
Audits use `querylog.clear_*` and `statistics.reset_*` action families and
contain command identity, explicit scope, counts, stable errors, and an input
fingerprint only.

POST returns HTTP 202 for queued/running resources. Poll the operation URL for
`succeeded`, `partial_success`, `failed`, or `interrupted`. A repeated terminal
idempotency key returns the original result without another node call. Running
commands interrupted by controller restart are not automatically replayed.

## Operational probes

```text
GET /health  # process liveness; does not require PostgreSQL
GET /ready   # PostgreSQL connectivity and mutation readiness
```

Stale or failed optional collectors do not fail liveness. PostgreSQL loss
fails readiness.

Authenticated detailed status is cluster-scoped:

```text
GET /api/v1/clusters/{clusterId}/operational-status
```

It returns presentation-ready overall, database/pool/storage, connectivity,
observation, Statistics, Query Log, gap, retention, and worker states with safe
codes and bounded arrays. It never returns raw errors, stack traces,
credentials, node URLs, query contents, or client identifiers.

`GET /metrics` returns 404 unless `METRICS_BEARER_TOKEN` is configured and
requires that bearer token when enabled. Metrics use bounded worker labels.

## HA Operations and Lifecycle

All routes below require authentication. Mutations require same-origin CSRF and
carry the normal request ID; notification-channel mutations additionally use the
administrator authorization guard. Responses expose stable codes and never
return node credentials or webhook destinations.

```text
GET  /api/v1/clusters/{clusterId}/ha-status
GET  /api/v1/clusters/{clusterId}/ha-history?nodeId={nodeId}&limit=50&cursor={cursor}
GET  /api/v1/clusters/{clusterId}/certificates
GET  /api/v1/clusters/{clusterId}/versions
GET  /api/v1/clusters/{clusterId}/upgrades
GET  /api/v1/clusters/{clusterId}/notification-channels
POST /api/v1/clusters/{clusterId}/notification-channels
PATCH /api/v1/notification-channels/{channelId}
POST /api/v1/notification-channels/{channelId}/test
DELETE /api/v1/notification-channels/{channelId}

GET  /api/v1/nodes/{nodeId}/lifecycle
GET  /api/v1/nodes/{nodeId}/maintenance-preflight
PUT  /api/v1/nodes/{nodeId}/lifecycle-settings
POST /api/v1/nodes/{nodeId}/dns-probe
POST /api/v1/nodes/{nodeId}/maintenance
POST /api/v1/nodes/{nodeId}/return-to-service
POST /api/v1/nodes/{nodeId}/upgrades
POST /api/v1/upgrades/{upgradeId}/validate
```

Notification create accepts `name`, `enabled`, and an HTTPS `destination`.
Lists return `destinationSummary` (scheme and host only), `subscribedEvents`,
state, and created/updated timestamps; they never return the encrypted or clear
destination. PATCH updates supported metadata and preserves the stored
destination by default. Destination replacement requires both
`replaceDestination: true` and a new `destination`; blank/implicit replacement
is rejected.

Test performs one bounded synthetic delivery with redirects disabled and returns
only safe success/status/error data. Delete requires
`{ "confirmation": "<exact channel name>" }`. Delete clears the delivery's
channel foreign key rather than cascading history; `channelName` remains as the
safe historical snapshot. Create, update, enable/disable, test, and delete are
administrator-only, CSRF-protected, and audited.

HA history is ordered by `(occurredAt DESC, id DESC)` and accepts `limit=1..100`
(default 50), optional cluster-owned `nodeId`, and the same opaque versioned
base64url keyset-cursor convention as Query Log. The response contains `items`,
optional `nextCursor`, and `hasMore`. Items have `kind=event|notification`;
notification items add safe channel name, delivered/failed/suppressed/pending
state, attempt count, optional HTTP status/sanitized reason, completion time,
and Test identity. Synthetic Test parent events are not duplicated as ordinary
HA transition rows. No destination URL or response body is returned.

Lifecycle settings use optimistic `recordVersion` and configure DNS host/port,
query name/type, expected RCODE, UDP/TCP, and installation type. An empty host
uses the node URL hostname. Maintenance accepts `enabled`, `recordVersion`,
and, only when required, `breakGlass: true` plus the exact confirmation
`CONTINUE_WITHOUT_DNS_REDUNDANCY`. Clearing maintenance through the legacy route
uses the same fail-closed return validation. Entering a node already in
maintenance or returning a node already in service is idempotent and does not
record a duplicate transition event. A failed return reports a stable API error
that names the failed safe checks, includes their safe actionable messages, and
leaves canonical maintenance state enabled. The response retains the normal
request ID. Check statuses are `pass`, `fail`, `not_applicable`, `unknown`, or
the informational `warning`. TLS is `not_applicable` when the fresh AdGuard Home
observation reports encryption disabled; it is required and fail-closed when
enabled, while unavailable applicability is `unknown` and blocking. Existing
drift is returned as a warning and becomes eligible for
normal reconciliation after successful exit.

Creating an upgrade accepts a target version and returns a durable guided
operation. The operator performs the native/systemd or Docker upgrade, then
validates with the current node record version. Atlas DNS Controller freshly reads the
installed version before return checks. Unsupported installation types return
a capability error and failed validation leaves maintenance enabled.

# HA Operations and Lifecycle

HA Operations coordinates maintenance, active DNS evidence, certificates,
versions, guided upgrades, events, and notifications without placing the
controller in the DNS path or owning node package execution.

## Active DNS and capacity

Node API health and active DNS health are separate. A worker sends a bounded DNS
query to each enabled non-maintenance node every 30 seconds using the node URL
host and observed DNS port by default. Operators can configure host, port, query
name/type, expected RCODE, and UDP/TCP. Each network operation has a two-second
bound.

Latest results and 30 days of probe evidence are durable; only transitions
create HA events, retained for one year. A result is healthy only when every
enabled protocol succeeds. Cluster state supports N nodes: healthy requires at
least two fresh serving nodes and no other degradation; degraded retains serving
redundancy with management/convergence/maintenance concerns; at-risk has fewer
than two verified serving nodes.

## Maintenance and return

Preflight blocks active deployments and an observed active DHCP owner and keeps
open drift visible. Removing the last verified DNS node requires the exact
break-glass confirmation. Maintenance excludes the node from normal polling,
deployment, and reconciliation and suppresses its expected DNS-failure alerts.

Return is fail-closed for live safety prerequisites. It freshly checks API,
capability/configuration observation, DNS, availability of configuration/drift
state, DHCP, conditionally applicable TLS, and configured collectors. Existing
drift or an unapplied active revision is retained as a non-blocking
reconciliation warning: because maintenance suppresses deployment and
reconciliation, exit must make the node eligible for repair rather than
deadlocking it in maintenance. Any required failure leaves maintenance enabled
and the safe API error names the failed checks and their operator-safe reasons.

Return checks use four explicit states: `pass`, `fail`, `not_applicable`, and
`unknown` (`warning` remains available for informational reconciliation state).
The current policy is:

| Check | Policy | Evidence and failure behaviour |
| --- | --- | --- |
| Management API | Required | Direct authenticated request to the node's configured base URL. An HTTPS URL performs normal certificate-chain and base-URL hostname verification under the node's system/custom-CA policy; an HTTP-only node remains supported through its explicit insecure-HTTP policy. |
| Observation and capabilities | Required | A fresh complete AdGuard Home configuration read must succeed. |
| DNS | Required | The configured UDP/TCP DNS probes must be healthy. |
| Configuration state | Required when unavailable; otherwise informational | Database/drift state must be readable. Existing drift or an unapplied revision is a reconciliation warning rather than a maintenance deadlock. |
| DHCP safety | Required cluster invariant | Atlas must be able to verify that no more than one node is observed as active DHCP. A node without DHCP does not fail this check. |
| TLS | Conditional | Required only when the fresh `/control/tls/status` observation reports `enabled: true`; disabled TLS is `not_applicable`. Unknown applicability fails closed alongside the required observation check. |
| Statistics and Query Log collectors | Conditional | A configured collector must have resumable state. An unconfigured collector is `not_applicable`. |

The check named `tls` is redacted configuration/certificate validation. During
the fresh observation Atlas calls AdGuard Home `GET /control/tls/status` over
the already-configured management transport and evaluates `enabled`, certificate
and chain validity, key/pair validity, and the public not-before/not-after
timestamps. It does **not** open a separate connection to HTTPS, DNS-over-TLS,
or DNS-over-QUIC ports, and it does not independently match `server_name` or the
certificate SAN list. HTTPS management transport and its expected hostname are
covered by the separate API check. Private keys, certificate chains, paths, and
raw node responses remain outside validation messages and logs.

The Nodes and Node Detail surfaces both use this canonical lifecycle rather
than clearing a browser-local flag. Successful transitions are persisted and
audited before the UI reloads canonical node state. Failed return validation is
audited, remains visible to the operator, and leaves the node in maintenance.
Repeating an already-completed enter or return request is an idempotent success
and does not create another transition event.

## Certificate and version awareness

Certificate state uses redacted observation metadata. Warning begins at 30 days,
critical at seven days, and expired is distinct. Private material and filesystem
paths never enter this model.

The AdGuard Home release checker reads the official GitHub latest-release API at
most every six hours and retains safe stale-cache state after failure.
Compatibility remains derived from explicit adapter profiles, not an upstream
tag alone. During a rolling upgrade, v0.107.78 and v0.107.79 nodes may coexist;
the upgraded node stops showing an available v0.107.79 update while the older
peer continues to show it, without requiring equal patch versions.

## Guided upgrades

Native/systemd and Docker nodes can have a durable guided operation. The
operator uses the platform-native install/container mechanism; the controller
records target/progress and performs fresh installed-version plus complete
return-to-service validation. Unsupported installation types are explicit.
Failure records safe evidence and leaves maintenance enabled. There is no remote
shell, automatic upgrade, Docker socket, or controller-owned rollback.

## Webhook channels

Webhook delivery uses the existing HA event stream. Destinations are HTTPS,
AES-256-GCM encrypted, and write-only. Payloads contain bounded event identity,
type, severity, summary, time, and optional node identity. Normal deliveries are
durable and retry at most five times with safe error codes.

Every existing HA transition is notification-triggering: DNS failed/recovered,
redundancy degraded/at-risk/restored, certificate warning/critical/expired and
recovery, version update/current transitions, planned-maintenance transitions,
return-validation failure, and guided-upgrade transitions. A maintenance-mode
DNS failure is retained as `suppressed` and is not sent. Node-connectivity,
collector-health, deployment, drift, and audit events are separate subsystems
and are not webhook triggers in v1.0.2. Channels subscribe to future
transitions; creating or enabling a channel does not replay an already-active
degraded state.

Administration supports add, edit, enable/disable, delete, and test:

- List/read returns name, safe scheme/host summary, explicit enabled state, the
  currently fixed HA-transition subscription, timestamps, and safe delivery
  state where available—never the destination.
- Edit preserves the encrypted destination unless replacement is explicitly
  requested with a valid new HTTPS value.
- Disabled channels receive no new events and retain configuration for re-enable.
- Test sends one bounded synthetic event directly, follows no redirect, does not
  alter HA state or notify other channels, and records its terminal result in
  Operational History without exposing a destination or response body.
- Delete requires exact-name confirmation. HA events remain unchanged and
  delivery rows retain a safe channel-name snapshot with a nullable channel
  reference.

All mutations require an administrator session, CSRF, validation, and audit.
Names must not contain secrets. Destination userinfo/fragments are rejected;
path and query are hidden from summaries and diagnostics.

## Node Detail presentation

Node Detail is the operational decision surface for one node. It uses common
page/header/group/field/status/action primitives and groups overview, DNS,
maintenance/DHCP, TLS, software, collectors, and history. It links to the
canonical Configuration Control, Drift, Deployments, DHCP, Statistics, Query
Log, Operational Status, and Audit views instead of duplicating those datasets.
Every group uses the shared padded panel-body mode because its content is not a
self-padding settings-row list. Nested probe and guided-upgrade forms use the
bounded shared panel form; tables and wrapping action rows retain side and
bottom inset at desktop and mobile widths.

## Retention and evidence

Probe results, transition events, upgrade operations, webhook channels, and
delivery attempts are separate from desired configuration and deployment
history. Bounded cleanup never removes audit or configuration history as a side
effect. Notification channel deletion preserves operational event/delivery
evidence as described above.

HA Operations combines immutable transition parents and per-destination
delivery results as separate Operational History rows. Delivery state is
`delivered`, `failed`, `suppressed`, or `pending` while retry remains possible.
Only 2xx means delivered. Failures retain a bounded safe classification such as
timeout, connection refused, TLS verification failure, or HTTP status; response
bodies and destination path/query never enter history. The authenticated list
uses 50-row (maximum 100) opaque keyset pages ordered by
`(occurred_at DESC, id DESC)`. Delivery creation time is the stable ordering
timestamp; completion remains supplemental evidence.

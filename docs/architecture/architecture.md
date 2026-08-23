# Architecture Overview

Atlas DNS Controller is a management plane around independently operating AdGuard
Home nodes. It coordinates desired configuration, deployment, drift, visibility,
and safe lifecycle work without joining the live DNS request path.

## System context

```text
Administrator browser
  │ same-origin HTTPS, authenticated /api/v1
  ▼
Atlas DNS Controller
  ├─ API and frontend
  ├─ deployment/reconciliation workers
  ├─ health/statistics/query-log workers
  ├─ HA lifecycle/notification workers
  └─ backup/update-awareness services
       │                         │
       │ SQL                     │ bounded HTTP(S) administration APIs
       ▼                         ▼
  PostgreSQL 17             AdGuard Home nodes
                                  ▲
DNS clients ──────────────────────┘ UDP/TCP/encrypted DNS directly
```

The browser never receives node credentials or calls nodes directly. The
controller does not bind a DNS port, proxy queries, or become a resolver. If the
controller or PostgreSQL stops, every AdGuard Home node continues serving its
last applied configuration.

## Components

### Controller process

One Go process serves the React application and versioned `/api/v1` API and runs
bounded background jobs. Request-scoped work propagates context and request IDs;
durable deployment and command jobs resume or reach explicit interrupted/failed
states after process failure. Public `/health` is liveness, `/ready` includes
PostgreSQL, and `/metrics` exists only when a sufficiently strong bearer token is
configured.

### PostgreSQL

PostgreSQL is the system of record for administrators/sessions, clusters/nodes,
encrypted credentials, desired drafts, immutable observations/revisions,
deployments and per-node results, drift, HA events, webhook deliveries, audit,
Statistics, Query Log, backup/update metadata, and lifecycle archive state.

Desired configuration, observed configuration, applied revision, deployment
results, and audit history remain distinct records. Migrations are append-only
after release and timestamps are UTC.

### AdGuard Home adapter

The adapter makes bounded, version-aware calls to native AdGuard Home
administration APIs. It maps the supported v0.107 API generation to explicit capabilities,
normalizes managed configuration, and discards credentials, TLS private
material, node response bodies, and unsafe URL detail before domain persistence
or logging. A newer v0.107 patch is provisionally compatible only after the
typed endpoints Atlas uses pass semantic validation. Other unknown API
generations remain observable but are blocked from managed write operations.

The standard topology is agentless. Statistics and Query Log use native APIs.
A local Query Log forwarder is not part of the current runtime and would require
measured evidence and a new decision.

### Frontend

The React/TypeScript frontend is a same-origin API client. It presents loading,
empty, partial, stale, error, and success states and preserves node attribution
through telemetry views. The server, not the UI, remains authoritative for
authorization, CSRF, capability, concurrency, and lifecycle deletion checks.

## Source-of-truth model

- **Draft:** mutable desired configuration with optimistic concurrency.
- **Revision:** immutable published desired state.
- **Observation:** immutable normalized state read from one node.
- **Applied revision:** last revision semantically verified on a node.
- **Active revision:** revision verified across the complete deployment target.
- **Drift:** durable difference between active desired and fresh observed state.

Shared values and node-specific values are modeled separately. Listener
addresses/port are verification-only. DHCP configuration/leases are node-specific
and deployment ordering disables former owners before enabling the desired owner.

## Configuration lifecycle

```text
edit/import draft
  → validate draft and every target capability
  → publish immutable revision
  → preview and explicitly create deployment
  → lock/revalidate all targets
  → apply one node at a time
  → read back into a new immutable observation
  → semantic verification
  → activate revision after total success
  → continuously evaluate drift
```

The initial strategy is sequential and stop-on-failure. Cancellation occurs only
between nodes. A failed or interrupted deployment never silently activates its
revision. Rollback creates a deployment of an existing immutable revision.

Reconciliation policy is Manual, Alert, or Enforce. Enforce uses the same
verified deployment path and excludes maintenance nodes.

## Operational data flows

Health, Statistics, and Query Log collectors read supported node APIs on bounded
intervals. Stored rows retain cluster and node identity. Coverage metadata makes
maintenance, unsupported nodes, stale data, failures, and known ingestion gaps
explicit. Retention cleanup is bounded and separate from configuration history.

Active DNS probes query each node over UDP/TCP independently from the management
API. HA event transitions, maintenance, certificate/version warnings, and guided
upgrade records coordinate operator work but do not run remote upgrade commands.

Webhook deliveries originate from durable HA events. Destinations are encrypted
write-only configuration. Delivery evidence retains a safe channel-name snapshot
if the channel is later deleted.

## Historical lifecycle and recovery

Revisions and deployments are history. Terminal records may be archived, which
hides them from default lists but preserves immutable content and relationships.
The active revision cannot be archived. Hard deletion is restricted to a
transactionally proven unused revision or a queued deployment with no started or
effectful node task and no reference.

Archive status is control-plane data in portable backups. Restore is offline to
a new empty database. Standard backup excludes high-volume operational table
data; Full includes it. Sessions and release caches are never restored.

## Availability and trust boundaries

- **Controller unavailable:** DNS continues; management, collection, and
  reconciliation pause.
- **PostgreSQL unavailable:** readiness fails and state-changing work stops; DNS
  continues on nodes.
- **One node unavailable:** other nodes may continue; the controller reports
  reduced capacity and blocks unsafe lifecycle actions.
- **Network partition:** results remain explicit per node; no success is inferred.
- **Unknown node capability:** observation may continue; affected writes stop.

Controller HA itself is not implemented. The reference deployment is one
controller and PostgreSQL instance around multiple independently serving DNS
nodes.

## Related references

- [Configuration model](configuration-model.md)
- [Reconciliation engine](reconciliation-engine.md)
- [Deployment topology](deployment.md)
- [Security guide](../security/security.md)
- [Database schema](../database/schema.md)
- [Architecture decisions](../decisions/README.md)

# Backend Design

## Executables

### controller

Initial all-in-one process:

- HTTP server.
- API.
- Frontend static assets.
- Scheduler.
- Background workers.

It may later be split into API and worker services.

### conditional forwarder

Not part of the supported runtime or a numbered release. ADR-0029 requires
measured API-ingestion limitations and a new review before implementation.

## Package boundaries

- `auth`: users, passwords, sessions, permissions.
- `domain`: core entities and value objects.
- `database`: repositories and transactions.
- `adguard`: external AdGuard Home adapter.
- `config`: controller runtime configuration.
- `reconciliation`: desired versus observed state.
- `telemetry`: statistics and query events.
- `jobs`: scheduling and job execution.
- `api`: HTTP transport and DTO mapping.
- `operationalhealth`: shared health states, aggregation, freshness mapping,
  bounded worker tracking, and presentation models.
- `useradmin`: administrator lifecycle validation and audit construction.
- `backup`: versioned encrypted archive creation, validation, compatibility,
  and empty-database offline restore.
- `updates`: bounded/cached stable-release metadata and installation-specific
  guidance without command execution.
- `systemsettings`: optimistic persisted controller settings and audit pairing.

### Release 0.1 concrete packages

- `internal/domain`: UUIDs, stable errors, users, sessions, clusters, nodes, audit types, validation, and `ManagementService` orchestration.
- `internal/database`: `pgx` pool, embedded migration runner, PostgreSQL repositories, transactions, and atomic protected-change/audit writes.
- `internal/auth`: Argon2id password hashing, HMAC session/CSRF tokens, login throttling, versioned credential encryption, and authentication service.
- `internal/adguard`: bounded, direct status/version and configuration adapter with explicit TLS policy, version capabilities, supported writers, and stable failure mapping.
- `internal/jobs`: health polling and session cleanup.
- `internal/api`: standard-library route registration, DTO decoding, authentication/CSRF middleware, security headers, error mapping, request IDs, and frontend serving.
- `internal/config`: bootstrap/network/secret environment validation plus one-time legacy runtime seeds.
- `internal/version`: build-injected controller version metadata.
- `internal/configuration`: canonical schema 2, one-way historical schema-1 read conversion, deterministic normalisation, hashing, validation, and ownership-aware structured differences.
- `internal/inventory`: observation, capability, comparison, audited draft import, and audited filter-refresh orchestration.
- `internal/controlplane`: desired draft validation, immutable revisions, deployment preview/creation/execution, rollback, drift evaluation, and reconciliation policy orchestration.

Release 0.2 extends `internal/adguard` with narrow configuration reads for `/control/status`, `/control/dns_info`, and `/control/filtering/status`. Listener addresses and port come from AdGuard Home's `ServerStatus` contract; shared DNS parameters come from `DNSConfig`. Raw payloads, counters, generated IDs, and timestamps remain inside the adapter. A missing or invalid listener identity makes the observation fail instead of creating an unusable import snapshot.

Release 0.3 adds narrow writes for shared schema-v1 DNS and filtering fields. Revision reads derive the API `active` flag with an explicit false value while the cluster has no active revision; nullable SQL state does not cross into the non-nullable API model. `internal/jobs` runs the durable deployment executor and periodic drift evaluator. Deployment and reconciliation checkpoints remain in PostgreSQL even though the worker is currently in the combined process.

Release 0.4 keeps those boundaries and adds schema-v2 mapping/writes inside `internal/adguard`, richer canonical types in `internal/configuration`, capability gating and DHCP ordering in `internal/controlplane`, and one audited explicit refresh method in `internal/inventory`. The API and React client exchange canonical documents, never raw AdGuard responses. No new service or settings repository was introduced.

`cmd/controller` wires these boundaries and owns graceful process lifecycle. `cmd/migrate` is a thin explicit migration entry point.
`cmd/atlas-dns-backup` is the Release 0.9 system-administration boundary for
portable create, preflight, and offline restore. Web backup and preflight reuse
the same `internal/backup` implementation; no online restore API exists.

## Domain services

Initial services:

- UserService
- SessionService
- ClusterService
- NodeService
- CapabilityService
- ObservationService
- RevisionService
- DeploymentService
- ReconciliationService
- StatisticsService
- QueryLogService
- AuditService

## Error model

Use typed domain errors:

- ValidationError
- NotFoundError
- ConflictError
- AuthenticationError
- AuthorisationError
- NodeUnavailableError
- NodeAuthenticationError
- CapabilityError
- ApplyError
- VerificationError

Transport layers map these to stable API responses.

## Transactions

Use transactions for:

- Creating a revision and its configuration payload.
- Activating a revision.
- Creating a deployment and node tasks.
- Recording an adopted drift change.
- Rotating encrypted credentials.
- Creating an audit event with a protected state change.

Release 0.1 uses a transaction for initial administrator creation, successful login/session creation, logout revocation, cluster creation/update, node creation/update/removal, and manual node connection-test results. A protected change fails if its audit event cannot be written.

Release 0.3 transactions pair draft edits, revision publication, deployment creation/cancellation/completion, drift detection/resolution, policy changes, and maintenance changes with their audit records. Revision activation occurs in the successful deployment completion transaction.

Release 0.6 records a node poll atomically: normalized events are batch-inserted
with node-scoped conflict handling, then its attempt and checkpoint are written
in the same transaction. A failed transaction advances no checkpoint. The
query-log worker is independent from health, statistics, deployment, and
reconciliation; it runs one pass at a time, bounds node concurrency and source
pages, propagates cancellation/timeouts, and never mutates a node.

Release 0.7 adds the authenticated cluster operational-status read model. It
reuses node/observation/Statistics/Query Log records and PostgreSQL metadata
rather than adding a parallel health database. Process worker state is bounded
and resets honestly to unknown on restart. Claim-loop failures back off
cancellably to 30 seconds; fixed-schedule collectors remain isolated per node.

## Release 0.1 failure behaviour

- Startup fails before serving when required configuration, secrets, PostgreSQL connectivity, or migrations are invalid.
- `/health` remains a process-only liveness check; `/ready` fails when PostgreSQL is unavailable.
- State-changing API work fails closed when it cannot be stored and audited.
- Node transport, TLS, authentication, and payload failures have different stable error codes.
- Poll failures retain the prior successful `last_seen_at`, set `last_polled_at`, and expose a safe error code.
- Controller shutdown cancels workers and gracefully drains HTTP; it performs no node mutation.

Release 0.8 adds a separate active-DNS health repository/service boundary. A
four-node bounded worker records samples and state transitions without allowing
one failure to stop other probes. Maintenance and upgrade services reuse the
existing transactional node management boundary for optimistic/audited state
changes, then persist separate lifecycle events. Notifications are derived from
durable events, encrypted at rest, bounded, idempotent per channel/event, and
suppressed for expected maintenance DNS failures.

Release 0.9 keeps the existing administrator-only permission model. User
disable/password reset and session revocation commit with their audit record;
the repository locks administrator state while enforcing the final-enabled-
administrator invariant. Backup uses PostgreSQL custom dumps and an `age`
scrypt recipient, includes the external credential key only inside the
authenticated encrypted payload, and excludes sessions. Restore is offline,
targets a new empty database, and uses a single PostgreSQL transaction. Update
awareness accepts only the project's stable GitHub release URL, caches results,
and returns inert host guidance.

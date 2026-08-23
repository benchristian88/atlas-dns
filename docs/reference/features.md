# Feature Catalogue

This is the authoritative catalogue of capabilities present in the current
Atlas DNS Controller tree. Release chronology belongs in [CHANGELOG.md](../../CHANGELOG.md).
Compatibility labels and incomplete external validation remain governed by the
[compatibility matrix](../operations/compatibility-matrix.md) and validation
records.

## DNS Configuration Management

- Imports supported AdGuard Home state into a mutable desired-state draft.
- Manages shared DNS behavior, filtering lists, allowlists, custom rules,
  persistent clients, rewrites, blocked services, safety services, Safe Search,
  Query Log/statistics policy, and guarded DHCP configuration/static leases.
- Keeps listener addresses/ports, DHCP ownership, and other node-specific values
  outside the shared configuration model.
- Validates target capabilities before publishing an immutable revision.
- Deploys sequentially, stops on failure, reads each node back, and activates a
  revision only after complete verified convergence.
- Supports semantic revision comparison and deployment-based rollback.

## High Availability

- Registers multiple AdGuard Home nodes with encrypted credentials and explicit
  version/capability profiles.
- Separates management API reachability, complete observation, active UDP/TCP
  DNS service, desired-state convergence, and collector health.
- Coordinates maintenance entry/return with remaining-capacity, active DHCP,
  deployment, drift, TLS, version, and collection checks.
- Detects direct managed changes as drift and supports Manual, Alert, and
  Enforce reconciliation policies.
- Provides node lifecycle detail and cluster HA event history without carrying
  DNS traffic or requiring a node agent.

## Visibility and Analytics

- Aggregates bounded AdGuard Home statistics for cluster and node scopes with
  explicit freshness and partial-coverage state.
- Retains a centrally searchable Query Log with mandatory source-node
  attribution, bounded cursor pagination, collection evidence, and node-local
  privacy policy preservation.
- Links Query Log findings into existing rule, rewrite, and client workflows;
  proposed changes still require draft publication and deployment.
- Shows Dashboard summaries without hiding partial or stale input.

## Operations and Monitoring

- Reports controller, PostgreSQL, node observation, worker, Statistics, Query
  Log, retention, and approximate storage state in Operational Status.
- Exposes public minimal `/health`, PostgreSQL-aware `/ready`, and optional
  bearer-protected Prometheus metrics.
- Records per-node deployment phases/results, drift incidents, HA events,
  lifecycle operations, and audit events.
- Sends bounded HTTPS webhook notifications with write-only endpoints, safe
  endpoint summaries, edit/pause/delete/test lifecycle, delivery evidence, and
  retained historical delivery identity after channel deletion. Delivery/Test
  outcomes appear in cursor-paginated Operational History with safe failure
  diagnostics.

## Administration

- Bootstraps the first local administrator and supports multiple local
  administrator accounts.
- Creates, disables/re-enables, and resets administrator credentials while
  preventing self-disable and loss of the final enabled administrator.
- Provides state-derived Setup Guide, System Settings, About/build metadata,
  update awareness, Backup & Restore, and audit views.
- Supports System, Light, and Dark themes and accessible responsive navigation.

## Security and Audit

- Uses secure HTTP-only browser sessions, CSRF protection, request IDs,
  administrator authorization, login rate limiting, and transactional safety
  checks.
- Encrypts stored node credentials and webhook destinations; never returns
  hidden secrets through list/read APIs.
- Audits authentication, user, node, configuration, deployment, drift,
  lifecycle, webhook, backup, restore-preflight, and update-setting actions.
- Redacts node secrets, TLS private material, webhook URL path/query/userinfo,
  and AdGuard Home response bodies from normal state and diagnostics.

## Backup and Recovery

- Creates passphrase-encrypted Standard control-plane or Full history-inclusive
  portable archives and supports non-mutating restore preflight.
- Restores offline only to a new empty PostgreSQL database and emits the
  recovered credential key separately.
- Includes revision/deployment archive status in control-plane recovery data;
  safely hard-deleted unused records are absent from later backups.
- Excludes sessions and release caches from restore.

## Software Lifecycle

- Caches stable controller-release awareness and presents installation-specific
  host guidance without executing commands.
- Coordinates native/systemd or Docker AdGuard Home upgrade records and
  post-upgrade validation without owning remote package execution.
- Archives immutable historical revisions and terminal deployments while
  retaining references and audit history.
- Permits hard deletion only for a server-proven unreferenced revision or a
  never-started, effect-free deployment, with strong confirmation and audit.

## Deliberate boundaries

- No normal DNS proxying, controller-based resolver, or controller availability
  dependency for DNS service.
- No automatic node package upgrades, Docker socket, remote shell, or default
  node-side agent.
- No TLS private-key/certificate mutation, broad RBAC, OIDC, or automatic
  deletion of operational history.
- A local Query Log forwarder remains evidence-triggered and under
  consideration; API polling is the current standard path.

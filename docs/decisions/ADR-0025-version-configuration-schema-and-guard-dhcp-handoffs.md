# ADR-0025: Version broader configuration and guard DHCP handoffs

**Status:** Accepted

**Date:** 2026-07-30

**Related release:** 0.4

## Context

Release 0.4 broadens the authoritative model beyond schema v1 DNS and blocklist fields. Existing observations and immutable revisions cannot be rewritten without invalidating their canonical hashes and historical meaning. AdGuard Home v0.107.52 supports the schema-v1 contract but lacks the complete Safe Search contract used by schema v2. DHCP also differs from shared policy: it is node-specific and enabling two servers can disrupt a network.

TLS status responses can contain certificate and private-key material. The controller needs useful TLS inventory without allowing those secrets into snapshots, revisions, API responses, logs, or browser state.

## Decision

- Introduce canonical configuration schema v2 and allow both versions in PostgreSQL. Never rewrite schema-v1 snapshots, drafts, or revisions.
- Continue schema-v1 inventory and historical reconciliation for AdGuard Home v0.107.52. Use schema v2 for the explicitly reviewed v0.107.53–v0.107.78 contract, where patch-level feature flags cover subsequent cache, timeout, rewrite, ignored-list, and filter-interval additions. Treat newer contracts as unknown until reviewed.

## v1.0.2 compatibility clarification

The exact v0.107.78 ceiling above described the original reviewed boundary; it
is not a permanent patch allowlist. v0.107.79 is now explicitly tested. Later
patches in the same v0.107 API generation may use schema v2 provisionally only
after the complete typed observation and capability preflight succeeds. Other
API generations remain unknown and writes fail closed. This preserves the
decision's version-aware, capability-aware intent while avoiding patch-number
releases when the consumed contracts are unchanged.
- Project current observations down to the revision schema before historical verification or drift comparison. Compare only shared-managed and node-managed fields for convergence; observed-only and unsupported metadata remain visible but cannot create drift.
- Model DNS, allowlists, clients, rewrites, blocked services and schedules, safety services, query-log policy, and statistics policy as shared desired state. Model DHCP configuration and static leases in UUID-keyed node overrides. Dynamic DHCP leases and redacted TLS status are observed-only.
- Permit DHCP to be enabled on at most one node in a desired document. A deployment that hands off the role orders every desired-disabled node before the desired-enabled node and retains sequential stop-on-failure behavior.
- Parse only public TLS status fields. Never decode, persist, return, or log certificate chains, private keys, or filesystem paths. TLS mutation remains unsupported until controller-managed secret references have a separate design.
- Filter refresh is an explicit per-node operation outside immutable revision deployment. Record requested and terminal audit events, and expose partial fleet outcomes in the UI.

## Consequences

- Schema-v1 rollback and Enforce reconciliation continue after upgrading the controller.
- Operators on v0.107.52 can keep using the 0.3 feature boundary but must upgrade nodes before importing or publishing schema v2.
- DHCP role handoff can leave no active controller-managed DHCP node after a failure, but cannot intentionally enable the new node before old managed nodes are disabled. Recovery remains explicit and audited.
- Query-log and statistics settings are managed on each node; event ingestion and aggregation remain separate later releases.
- Adding another compatibility-specific field requires an explicit capability flag or a new schema version rather than silently changing canonical meaning.

## Deferred

- TLS certificate/key deployment and secret-reference lifecycle.
- Parallel or rolling deployment, automatic partial recovery, scheduled maintenance windows, and field-level drift ignore.
- Central statistics and query-log ingestion.

## Review triggers

Review when the minimum supported AdGuard Home version changes, TLS secret references are designed, multi-active DHCP becomes a supported topology, or schema v3 is proposed.

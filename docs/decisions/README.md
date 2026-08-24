# Architecture Decision Records

Architecture Decision Records capture durable decisions and their rationale.
The audited status vocabulary is **Accepted**, **Superseded**, **Deprecated**,
or **Proposed**. Individual ADR content remains historical and is not rewritten
to match later implementation chronology.

| ADR | Status | Decision |
|---|---|---|
| [ADR-0001](ADR-0001-build-a-separate-controller-instead-of-forking-adguard-home.md) | Accepted | Build a separate controller instead of forking AdGuard Home |
| [ADR-0002](ADR-0002-keep-the-controller-out-of-the-dns-request-path.md) | Accepted | Keep the controller out of the DNS request path |
| [ADR-0003](ADR-0003-use-desired-state-configuration-as-the-source-of-truth.md) | Accepted | Use desired-state configuration as the source of truth |
| [ADR-0004](ADR-0004-implement-the-controller-and-forwarder-in-go.md) | Accepted | Implement the controller and forwarder in Go |
| [ADR-0005](ADR-0005-use-react-typescript-and-vite-for-the-frontend.md) | Accepted | Use React, TypeScript, and Vite for the frontend |
| [ADR-0006](ADR-0006-use-postgresql-as-the-initial-system-of-record.md) | Accepted | Use PostgreSQL as the initial system of record |
| [ADR-0007](ADR-0007-integrate-through-a-version-aware-adguard-home-api-adapter.md) | Accepted | Integrate through a version-aware AdGuard Home API adapter |
| [ADR-0008](ADR-0008-implement-query-log-polling-before-a-node-forwarder.md) | Accepted | Implement query-log polling before a node forwarder |
| [ADR-0009](ADR-0009-use-sequential-verified-deployments-initially.md) | Accepted | Use sequential verified deployments initially |
| [ADR-0010](ADR-0010-separate-shared-configuration-from-node-specific-overrides.md) | Accepted | Separate shared configuration from node-specific overrides |
| [ADR-0011](ADR-0011-store-immutable-configuration-revisions-and-use-deployment-based-rollback.md) | Accepted | Store immutable configuration revisions and use deployment-based rollback |
| [ADR-0012](ADR-0012-support-enforce-alert-and-manual-reconciliation-policies.md) | Accepted | Support Enforce, Alert, and Manual reconciliation policies |
| [ADR-0013](ADR-0013-start-with-local-authentication-and-add-oidc-later.md) | Accepted | Start with local authentication and add OIDC later |
| [ADR-0014](ADR-0014-use-debian-lxc-and-systemd-as-the-reference-deployment.md) | Accepted | Use Debian LXC and systemd as the reference deployment |
| [ADR-0015](ADR-0015-make-central-query-log-collection-privacy-conscious-and-configurable.md) | Accepted | Make central query-log collection privacy-conscious and configurable |
| [ADR-0016](ADR-0016-make-node-management-capability-aware-and-version-aware.md) | Accepted | Make node management capability-aware and version-aware |
| [ADR-0017](ADR-0017-use-a-monorepo-and-documentation-first-delivery-model.md) | Accepted | Use a monorepo and documentation-first delivery model |
| [ADR-0018](ADR-0018-defer-controller-ha-until-after-the-single-controller-product-is-stable.md) | Accepted | Defer controller HA until after the single-controller product is stable |
| [ADR-0019](ADR-0019-limit-early-dhcp-support-to-safe-inventory-and-single-active-node-workflows.md) | Accepted | Limit early DHCP support to safe inventory and single-active-node workflows |
| [ADR-0020](ADR-0020-defer-final-licensing-selection-pending-legal-and-commercial-review.md) | Superseded by ADR-0033 | Defer final licensing selection pending legal and commercial review |
| [ADR-0021](ADR-0021-release-0-1-runtime-and-security-foundations.md) | Accepted | Define Release 0.1 runtime and security foundations |
| [ADR-0022](ADR-0022-support-git-based-systemd-and-docker-compose-installation-in-0-1-1.md) | Accepted | Support git-based systemd and Docker Compose installation in 0.1.1 |
| [ADR-0023](ADR-0023-freeze-release-0-2-read-only-configuration-inventory.md) | Accepted | Freeze Release 0.2 as a read-only configuration inventory |
| [ADR-0024](ADR-0024-release-0-3-authoritative-deployment-and-reconciliation-boundary.md) | Accepted | Define the Release 0.3 authoritative deployment and reconciliation boundary |
| [ADR-0025](ADR-0025-version-configuration-schema-and-guard-dhcp-handoffs.md) | Accepted | Version broader configuration and guard DHCP handoffs |
| [ADR-0026](ADR-0026-adopt-adguard-v2-inspired-ui-language.md) | Accepted | Adopt an AdGuard Home v2-inspired operator model |
| [ADR-0027](ADR-0027-use-query-backed-inline-details-for-the-ha-revision-lifecycle.md) | Accepted | Use query-backed inline details for the HA revision lifecycle |
| [ADR-0028](ADR-0028-aggregate-exact-node-statistics-in-the-controller.md) | Accepted | Aggregate exact node statistics in the controller |
| [ADR-0029](ADR-0029-remain-agentless-by-default.md) | Accepted | Remain agentless by default and defer a local Query Log forwarder until evidence requires it |
| [ADR-0030](ADR-0030-coordinate-ha-lifecycle-without-owning-node-upgrades.md) | Accepted | Coordinate HA lifecycle and guided upgrades without owning node package execution |
| [ADR-0031](ADR-0031-use-passphrase-encrypted-portable-controller-backups-and-offline-restore.md) | Accepted | Use passphrase-encrypted portable controller backups and offline restore |
| [ADR-0032](ADR-0032-use-cached-github-release-awareness-and-host-guided-controller-updates.md) | Accepted | Use cached GitHub release awareness and host-guided controller updates |
| [ADR-0033](ADR-0033-license-atlas-dns-controller-under-busl-1-1.md) | Accepted | License Atlas DNS Controller under Business Source License 1.1 |
| [ADR-0034](ADR-0034-use-a-left-rail-and-operational-dashboard.md) | Accepted | Use a left rail and operational dashboard |

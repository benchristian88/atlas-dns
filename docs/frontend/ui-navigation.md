# UI Navigation

This is the canonical route and route-ownership reference. Visual behavior,
responsive presentation, and keyboard interaction are defined in
[Navigation and Application Shell](navigation-and-shell.md).

## Current route map

| Route | Owner | Navigation group |
|---|---|---|
| `/` | Cluster dashboard | Dashboard |
| `/statistics` | Aggregated exact node Statistics | Monitoring |
| `/query-log` | Combined node-attributed Query Log | Monitoring |
| `/system/operational-status` | Controller and collector diagnostics | Monitoring |
| `/settings/general` | General settings | Settings / AdGuard Home |
| `/settings/dns` | DNS settings | Settings / AdGuard Home |
| `/settings/encryption` | Encryption inventory and guidance | Settings / AdGuard Home |
| `/settings/clients` | Persistent clients | Settings / AdGuard Home |
| `/settings/dhcp` | DHCP configuration and leases | Settings / AdGuard Home |
| `/filters/blocklists` | DNS blocklists | Filters / AdGuard Home |
| `/filters/allowlists` | DNS allowlists | Filters / AdGuard Home |
| `/filters/rewrites` | DNS rewrites | Filters / AdGuard Home |
| `/filters/blocked-services` | Blocked services | Filters / AdGuard Home |
| `/filters/custom-rules` | Custom filter rules | Filters / AdGuard Home |
| `/ha/nodes` | Managed node inventory | HA Controller / Atlas |
| `/ha/nodes/{nodeId}` | Node lifecycle detail | HA Controller / Nodes |
| `/ha/operations` | Fleet HA lifecycle and history | HA Controller / Atlas |
| `/ha/notifications` | HA lifecycle webhook delivery | HA Controller / Atlas |
| `/ha/configuration` | Configuration Control | HA Controller / Atlas |
| `/ha/revisions` | Immutable configuration revisions | HA Controller / Atlas |
| `/ha/deployments` | Deployment execution and node results | HA Controller / Atlas |
| `/ha/drift` | Convergence and drift resolution | HA Controller / Atlas |
| `/system/users` | Administrator accounts | Administration |
| `/system/audit` | Audit log | Administration |
| `/system/settings` | Controller runtime settings | Administration |
| `/system/backups` | Backup and restore | Administration |
| `/system/updates` | Controller update awareness | Administration |
| `/system/about` | Build and product information | Administration |
| `/setup-guide` | State-derived setup reference | Help utility |

Settings and Filters author managed AdGuard Home desired state. HA Controller
owns Atlas-specific orchestration and delivery behavior, including
Notifications. Administration owns users and controller/system operations.
Operational Status is an observe experience and therefore appears in
Monitoring even though its stable route remains under `/system`. Setup Guide is
reference/help, not first-run onboarding. No Integrations route exists.

## Compatibility redirects

The browser retains query strings and fragments while redirecting.

| Previous route | Canonical route |
|---|---|
| `/settings/filters` | `/filters/blocklists` |
| `/settings/rewrites` | `/filters/rewrites` |
| `/settings/services` | `/filters/blocked-services` |
| `/settings/privacy` | `/settings/general` |
| `/settings/infrastructure` | `/settings/encryption` |
| `/ha/history` | `/ha/revisions` |
| Any canonical path with a trailing slash | The same path without the trailing slash |

## Route behavior

- Routes are stable, guarded, refreshable, and bookmarkable.
- Unknown paths render an explicit Not Found page and never Dashboard.
- Cluster and selected-node scope remain application context; secrets never
  appear in URLs.
- Revision, deployment, and drift selection use `revisionId`, `deploymentId`,
  and `driftId` query parameters and preserve unrelated query state.
- Node lifecycle detail keeps the Nodes parent and link active.
- Desktop and mobile use the same route hierarchy.
- Breadcrumbs remain reserved for detail views rather than top-level pages.

## Configuration lifecycle ownership

`/ha/configuration` is lifecycle control, not a duplicate settings editor. It
contains the read-only draft/change summary, validation, advanced observation
and import/adoption, and immutable publication. Revision comparison and
rollback belong to `/ha/revisions`; execution belongs to `/ha/deployments`;
continuing divergence belongs to `/ha/drift`.

Historical route migration evidence remains in the
[pre-1.0 frontend implementation archive](../archive/pre-1.0/frontend/implementation/).

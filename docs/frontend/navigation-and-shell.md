# Navigation and Application Shell

## v1.1 outcome

Atlas uses a persistent left rail for product navigation and a thin top bar for
shared context and utilities. The shell makes the distinction between managed
AdGuard Home configuration and Atlas controller functions explicit. It does
not imply multi-controller or multi-cluster orchestration that the backend does
not provide.

## Information architecture

```text
Dashboard

Monitoring
├── Statistics
├── Query Log
└── Operational Status

Settings                         AdGuard Home configuration
├── General
├── DNS
├── Encryption
├── Clients
└── DHCP

Filters                          AdGuard Home configuration
├── DNS Blocklists
├── DNS Allowlists
├── DNS Rewrites
├── Blocked Services
└── Custom Filter Rules

HA Controller                    Atlas controller functions
├── Nodes
├── HA Operations
├── Notifications
├── Configuration Control
├── Revisions
├── Deployments
└── Drift

Administration
├── Users
├── Audit Log
├── System Settings
├── Backups
├── Updates
└── About

Help utility
└── Setup Guide
```

Settings and Filters exclusively author desired AdGuard Home configuration.
Notifications belongs to HA Controller because it configures Atlas delivery of
HA lifecycle events. There is no general Integrations destination. Setup Guide
is state-derived reference/help and is not a first-run onboarding mechanism.
Guided onboarding is a state-aware authenticated workflow offered for
incomplete setup or launched manually from Setup Guide; it does not add a
second permanent navigation section.

## Route map

| Route | Menu owner | Destination |
|---|---|---|
| `/` | Dashboard | Cluster dashboard |
| `/statistics` | Monitoring | Statistics |
| `/query-log` | Monitoring | Query Log |
| `/system/operational-status` | Monitoring | Operational Status |
| `/settings/general` | Settings | General |
| `/settings/dns` | Settings | DNS |
| `/settings/encryption` | Settings | Encryption |
| `/settings/clients` | Settings | Clients |
| `/settings/dhcp` | Settings | DHCP |
| `/filters/blocklists` | Filters | DNS Blocklists |
| `/filters/allowlists` | Filters | DNS Allowlists |
| `/filters/rewrites` | Filters | DNS Rewrites |
| `/filters/blocked-services` | Filters | Blocked Services |
| `/filters/custom-rules` | Filters | Custom Filter Rules |
| `/ha/nodes` | HA Controller | Nodes |
| `/ha/nodes/{nodeId}` | HA Controller / Nodes | Node lifecycle detail |
| `/ha/operations` | HA Controller | HA Operations |
| `/ha/notifications` | HA Controller | Notifications |
| `/ha/configuration` | HA Controller | Configuration Control |
| `/ha/revisions` | HA Controller | Revisions |
| `/ha/deployments` | HA Controller | Deployments |
| `/ha/drift` | HA Controller | Drift |
| `/system/users` | Administration | Users |
| `/system/audit` | Administration | Audit Log |
| `/system/settings` | Administration | System Settings |
| `/system/backups` | Administration | Backups |
| `/system/updates` | Administration | Updates |
| `/system/about` | Administration | About |
| `/setup-guide` | Help utility | Setup Guide |
| `/onboarding` | Setup Guide/manual offer | Guided onboarding or completed review |

All pre-v1.1 supported routes remain supported. Compatibility redirects retain
query strings and fragments. Unknown routes render Not Found and never silently
fall through to Dashboard. Authentication, route guards, browser refresh, and
deep-link behavior are unchanged.

## Desktop left rail

The rail uses the project-owned line-icon renderer and shows icons with labels.
Groups are native buttons with `aria-expanded` and `aria-controls`. The group
that owns the current route remains visibly active and open; node-detail routes
keep Nodes current. Operators may open or close inactive groups.

The collapse control is at the bottom of the rail. Collapsed state is
browser-local presentation state under `atlas-dns.sidebar-collapsed`; it does
not create an API setting or database record. Collapsed links and group buttons
retain accessible names and native title tooltips. Selecting a group while
collapsed expands the rail before revealing its children.

Keyboard behavior:

- Tab reaches every group, link, utility, and collapse control;
- Enter or Space toggles a group;
- Arrow Down or Arrow Right opens a group and focuses its first child;
- visible focus uses the shared Atlas focus token;
- the current link uses `aria-current="page"`.

## Utility top bar

The top bar contains existing shared capabilities only:

- selected cluster;
- Entire Cluster or selected-node scope;
- active revision and aggregate health where space permits;
- active deployment link when present;
- last node-inventory refresh state;
- Light, Dark, or System theme control;
- notification shortcut when notification support is available; and
- user/account menu and Sign Out.

It does not repeat primary navigation. Smaller viewports progressively hide
secondary facts while preserving cluster selection, theme, notifications, and
the account action.

## Mobile and responsive contract

At tablet and phone widths the left rail becomes a left-hand modal drawer. The
drawer uses the same labels, order, grouping, active state, and utility item as
desktop. Escape and the close control dismiss it and restore focus to the menu
trigger. Group disclosures never depend on hover.

The shell uses `minmax(0, 1fr)`, explicit inline-size containment, and local
table scrolling. Dashboard health cards reflow from five to three, two, and one
columns; the activity, attention, and recent-change grid becomes a single
column; KPI cells wrap two-by-two; and node tables keep their established
contained horizontal treatment. The shell does not use document-level
horizontal clipping as a substitute for component responsiveness. iOS safe
areas remain supported and browser zoom is not disabled.

The onboarding progress rail scrolls locally on narrow screens, source cards
become one column, and actions wrap. Exit, Back, validation errors, retry, and
all labelled native fields remain reachable without document-level overflow.

## Dashboard purpose

Dashboard is the concise operational answer to:

```text
Is DNS healthy?
Are my nodes reachable?
Is HA healthy?
Is collection healthy?
What needs attention?
What changed recently?
```

It composes existing Nodes, HA status, Operational Status, Statistics,
versions, revisions, deployments, drift, and safe audit summaries. It does not
create a second alert engine, derive traffic from Query Log, invent node roles,
or manufacture unavailable metrics. Detailed action and coverage remain on the
owning pages.

## Failure and security behavior

Node inventory is the essential dashboard source and uses the shared retryable
error state. Supplementary source failures retain available dashboard data and
show an explicit partial-source warning. Unavailable statistics remain an em
dash or unavailable panel rather than zero. Audit summaries use safe action and
resource labels only; metadata and secrets are never rendered.

# Navigation and Application Shell

## v1.1 outcome

Atlas uses a persistent left rail for product navigation, help, and signed-in
identity. There is no global top utility bar. The shell makes the distinction between managed
AdGuard Home configuration and Atlas controller functions explicit. It does
not imply multi-controller or multi-cluster orchestration that the current UI
does not provide.

Atlas retains cluster-scoped domain/API architecture, but v1.1 exposes only the
current cluster because no supported multi-cluster creation/switch workflow
exists yet. Cluster IDs, models, persistence, and cluster-scoped API contracts
remain authoritative and must not be removed as a consequence of this UI choice.

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
| `/account` | Account menu | My Account |
| `/account/preferences` | Account menu | Preferences |
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

Setup Guide, the collapse control, and the signed-in account menu are at the
bottom of the rail. The account menu shows the actual display name and role and
opens My Account, Preferences, and Sign out. My Account and Preferences are not
primary navigation items and do not duplicate Administration → Users.

Collapsed state is
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

## Page ownership without a top bar

- Dashboard is always the current cluster overview. Its DNS activity and domain
  rankings use the cluster-wide Statistics report.
- Statistics owns the Entire Cluster/individual-node traffic selector and fixed
  range choices. Its generated/collection time is dataset-local freshness.
- Revision, health, active deployment, attention, and freshness remain on their
  canonical Dashboard, Configuration Control, Revisions, Deployments,
  Operational Status, and HA Operations surfaces.
- HA Controller → Notifications remains the notification management surface;
  there is no global bell or alert badge.
- Preferences owns System, Light, and Dark appearance. My Account owns current
  identity and the authenticated current-password-verified password change.

## Mobile and responsive contract

At tablet and phone widths the left rail becomes a left-hand modal drawer opened
by a compact floating navigation trigger rather than a replacement utility bar. The
drawer uses the same labels, order, grouping, active state, and utility item as
desktop. Its bottom account menu keeps My Account, Preferences, and Sign out
reachable. Escape and the close control dismiss it and restore focus to the menu
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

It composes existing Nodes, HA status, Operational Status, cluster-wide Statistics,
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

Equivalent authenticated table row disclosures use `+` while collapsed and `−`
while expanded, with `aria-expanded`, `aria-controls`, and a descriptive label.
This applies to Revisions, Deployments, Drift, Query Log, and Audit Log, not to
select menus, sorting, navigation groups, or independent native details blocks.

HA Operations applies the same partial-source principle to HA summary, node,
certificate, version, upgrade, and Operational History reads. A failed source
gets a scoped warning and retry without blanking successful sections. Retained
last-good data is labelled stale. Notifications likewise keeps useful
last-known-good policy/channel data visible after a refresh failure with an
announced stale warning.

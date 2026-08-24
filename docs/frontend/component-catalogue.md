# Component Catalogue

## Current implementation

Shared primitives are implemented under `web/src/components/`. Feature-specific
tables, catalogues, selectors, schedules, and deployment/drift dialogs live
under `web/src/features/` and compose these generic primitives.

Blocked Services uses the reusable `ScheduleEditor` under
`web/src/components/` and the blocked-service `ServiceCatalogue`,
`ServiceGroup`, and `ServiceToggle`
composition under `web/src/features/blockedservices/`. The selector consumes
controller presentation metadata; Clients reuse the same catalogue
presentation.

Blocklists and Allowlists use the shared `FilterListTable`, filter-list
projection, URL editor, disable-oriented confirmation, per-node results, and refresh-command
composition under `web/src/features/filterlists/`. DNS Blocklists and DNS
Allowlists use that composition through separate route wrappers, desired-state
fields, presentation reads, copy, and API flags.

Persistent Clients use the shared `TagMultiSelect` and compose `DataTable`, `Dialog`,
`ConfirmDialog`, `IdentifierListEditor`, `UpstreamEditor`, the shared
`ServiceCatalogue`, and `ScheduleEditor` under `web/src/features/clients/`.
Persistent Clients use that composition for searchable rows and structured
add/edit/remove draft interactions.

DNS Rewrites compose `DataTable`, `Dialog`, `ConfirmDialog`, `Field`, `SettingRow`,
`SettingsGroup`, `StatusBadge`, `ConvergenceSummary`, feedback, and
`UnsavedChangesNotice` under `web/src/features/rewrites/`. DNS Rewrites use that
composition for searchable rows, contract-bounded validation and type
inference, capability-aware enablement, and confirmed draft-only deletion.

DHCP uses the shared `LeaseTable` and composes it with `Dialog`,
`ConfirmDialog`, `DurationField`, `Field`, `SettingsGroup`, `ScopeIndicator`,
`StatusBadge`, and feedback under `web/src/features/dhcp/`. DHCP uses separate
per-node sections for discovered interfaces, audited active-server checks,
network fields, observed active leases, and draft-managed static leases.

General Settings composes `PageContainer`, `PageHeader`, `SettingsGroup`,
`SettingRow`,
`DurationField`, `DomainListField`, `ScopeIndicator`, `CapabilityWarning`,
feedback, and `UnsavedChangesNotice` under `web/src/features/general/`.
`DurationField` now optionally supports exact custom-unit multipliers so
millisecond schema values can use friendly units without conversion loss.

DNS Settings composes `UpstreamEditor`, `NetworkListField`, `DurationField`,
`Field`, `SettingRow`, `SettingsGroup`, `ScopeIndicator`, capability feedback,
and draft feedback under `web/src/features/dns/`. DNS resolver lists remain
lossless browser-side inputs; schema-v2 ordered/set canonicalisation remains
authoritative. Cache sizes use exact bytes with binary KiB/MiB presentation,
and DNS TTL/timeout values use exact whole-second duration conversion. Upstream
testing uses the separate controller command boundary.

Operational DNS commands compose `OperationalCommandDialog`,
`PartialSuccessPanel`, and
`StatusBadge` on DNS Settings for current-draft upstream testing and confirmed
DNS cache clearing. Both commands have explicit selected-node/fleet scope,
durable node-attributed results, and remain outside desired state.

Custom Filter Rules uses its specialist `RuleEditor` page and
composes the same operational primitives for host-filtering tests. Hostname,
optional client, optional query type, exact scope, compatibility exclusions,
and node-attributed matched rules remain separate from Save Draft and desired
state.

Query Log clearing and Statistics reset compose `OperationalCommandDialog`,
`PartialSuccessPanel`, and
`StatusBadge` in the General Settings Query Log and Statistics policy groups.
Both destructive commands use typed confirmation, narrow selected-node
defaults, explicit compatible-fleet scope, and durable node-attributed results
without changing the corresponding desired policy.

## Shell

The shell uses `ThemeProvider`, `ThemeControl`, the shared `AtlasBrand`
renderer, and one project-owned `Icon` renderer. Desktop navigation is a
collapsible left rail with controlled group disclosures, active/open ancestry,
labelled collapsed icons, and arrow-key entry into child links. Mobile uses the
same hierarchy in a controlled modal drawer. Theme and asset behavior is
documented in `theme-brand-and-pwa.md`.

- `ApplicationSidebar`
- `PrimaryNavigation`
- `NavigationGroup`
- `MobileNavigationDrawer`
- `UtilityTopBar`
- `ClusterSelector`
- `ScopeSelector`
- `ActiveRevisionIndicator`
- `ActiveDeploymentIndicator`
- `PageContainer`
- `PageHeader`
- `NotFoundPage`

## Settings

- `SettingRow`
- `SettingsGroup`
- `Field`
- `DurationField`
- `NetworkListField`
- `DomainListField`
- `UrlListField`
- `OrderedTextEditor`
- `RuleEditor`
- `ScopeIndicator`
- `CapabilityWarning`
- `UnsavedChangesNotice`

## Structured resources

- `DataTable`
- `FilterListTable`
- `ClientTable`
- `RewriteTable`
- `LeaseTable`
- `ServiceCatalogue`
- `ServiceGroup`
- `ServiceToggle`
- `ScheduleEditor`
- `TagMultiSelect`
- `IdentifierListEditor`
- `UpstreamEditor`

## HA display

- `MetricCard`
- `SummaryTileGrid`
- `StatusBadge`
- `NodeBadge`
- `RevisionBadge`
- `ConvergenceSummary`
- `StructuredDiff`
- `ProgressTimeline`
- `PartialSuccessPanel`

`MetricCard` is the single summary/stat tile for Dashboard, Statistics,
Operational Status, HA Operations, and Node Lifecycle. It owns the primary
surface, label/value gap, optional supporting detail, and responsive wrapping;
feature pages provide content only.

`SummaryTileGrid` is the shared definition-list treatment for two-by-two inset
status/value summaries. Dashboard controller/DNS summaries and Operational
Status Core Services provide semantic values and supporting metadata while the
component owns tile hierarchy and spacing.

`SettingsGroup` has explicit `rows` and `padded` body modes. Divided settings
rows retain their own vertical rhythm; ordinary panel content, tables, nested
forms, and action groups use the padded mode. `panel-form` bounds nested forms,
and the shared `row-actions` treatment wraps actions with token spacing.

Dashboard's five health cards, DNS activity chart/KPIs, attention and recent
change lists, node table, and domain rankings remain feature compositions. They
reuse shared cards, feedback, status, table, heading, icon, and button
treatments while preserving the semantics and failure state of each source.

`DataTable` optionally renders one expanded record as an adjacent table row.
Feature code owns the record-specific disclosure button and operational detail;
the shared table owns row adjacency, full-column span, selected styling, and
backward compatibility for non-expandable consumers. Revisions, Deployments,
and Drift combine this with query-backed selection. Large immutable revision
JSON remains a nested, collapsed native disclosure.

## Feedback and overlays

- `Banner`
- `Toast`
- `EmptyState`
- `ErrorState`
- `LoadingSkeleton`
- `Dialog`
- `ConfirmDialog`
- `DeploymentDialog`
- `DriftResolutionDialog`
- `OperationalCommandDialog`

## Governance

Feature pages may compose shared components but must not create local replacements for:

- navigation;
- buttons;
- cards;
- tables;
- dialogs;
- settings rows;
- badges;
- duration controls;
- list editors;
- destructive confirmation.

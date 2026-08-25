# Frontend Design System

## Purpose

This document defines the visual and interaction system for Atlas DNS Controller.

The design should feel familiar to AdGuard Home users while remaining an original implementation designed for multi-node management, desired-state configuration, revisions, deployments, and drift control.

The application uses:

- A persistent, collapsible desktop left rail.
- A responsive mobile navigation drawer with the same hierarchy.
- A thin utility top bar for cluster, scope, health, refresh, theme, notification,
  and account context.
- Semantic light and dark themes.
- Shared form, table, dialog, status, and feedback components.
- Clear separation between Save Draft, Publish Revision, Deploy, Verify, and Reconcile.

---

## Design principles

### Familiar before novel

Routine DNS administration should use familiar AdGuard Home terminology, density, grouping, and control patterns.

### HA context must remain visible

The interface must make these concepts explicit:

- Active cluster.
- Entire Cluster or selected node.
- Active revision.
- Cluster health.
- Active deployment.
- Convergence and drift.

### API types do not dictate UI controls

The AdGuard Home API is a transport contract, not a user-interface specification.

Examples:

- Known service IDs become a searchable service catalogue.
- Filter URLs become a structured table.
- Persistent clients become a table and edit dialog.
- DNS rewrites become a searchable table.
- Durations become friendly duration controls.
- IP and CIDR collections become validated structured lists.
- Custom filter rules remain a specialist text editor because the text itself is the domain representation.

### Preserve the controller lifecycle

The interface must distinguish:

```text
Save Draft
→ Publish Revision
→ Deploy
→ Verify
→ Reconcile
```

Saving a form must never imply that node configuration has already changed.

---

## Application shell

### Desktop left rail

Use left-hand primary navigation.

```text
Dashboard
Monitoring
Settings
Filters
HA Controller
Administration
```

Nested groups:

```text
Monitoring
├── Statistics
├── Query Log
└── Operational Status
```

```text
Settings
├── General
├── DNS
├── Encryption
├── Clients
└── DHCP
```

```text
Filters
├── DNS Blocklists
├── DNS Allowlists
├── DNS Rewrites
├── Blocked Services
└── Custom Filter Rules
```

```text
HA Controller
├── Nodes
├── HA Operations
├── Notifications
├── Configuration Control
├── Revisions
├── Deployments
└── Drift
```

Administration contains:

- Users
- Audit Log
- System Settings
- Backups
- Updates
- About

Setup Guide is a utility item at the bottom of the rail. It is reference/help,
not first-run onboarding. Sign Out remains in the account menu. Settings and
Filters contain only AdGuard Home configuration; Notifications remains an
Atlas HA Controller function. Do not add an Integrations destination.

The compact theme icon button sits in the utility top bar and opens explicit
Light, Dark, and System choices. Atlas theme-specific lockup assets provide the visual
brand foundation; phone layouts use the approved symbol-only asset rather than
compressing the lockup.

### Utility top bar

The top bar appears beside the rail and does not repeat primary navigation.

It contains:

- Cluster selector.
- Scope selector.
- Active revision.
- Cluster health.
- Active deployment indicator.
- Last-refreshed state.
- Theme, notification, and account actions.

Example:

```text
[Home DNS ▾] [Entire Cluster ▾] Revision 24  ● Healthy
```

### Mobile

Below the desktop breakpoint:

- Replace the left rail with a hamburger-triggered full-height navigation drawer.
- Preserve the same navigation hierarchy.
- Show Monitoring, Settings, Filters, HA Controller, and Administration as
  expandable groups.
- Keep cluster and scope context visible or accessible through a context sheet.
- Do not invent a separate mobile-only information architecture.
- Keep the authenticated shell within the layout viewport on first render.
  Document-level horizontal scrolling or clipping is not a responsive strategy;
  wide tables own their deliberate contained scrolling.
- Respect iOS safe-area insets in browser and standalone modes while preserving
  normal browser zoom.

---

## Colour roles

Use semantic tokens rather than raw colours inside feature components.

Atlas Blue is the interaction and selection colour. Green is reserved for
health and success. Amber communicates warnings, red communicates danger or
failure, and neutral colours provide structure and surface hierarchy. Brand
Teal remains artwork-only unless a separately documented interaction role is
approved. Feature pages must not redefine these roles.

Required roles:

```css
--page
--page-subtle
--header
--card
--popup
--border
--text
--text-muted
--text-disabled
--input
--input-border
--primary
--primary-hover
--primary-soft
--success
--success-soft
--info
--info-soft
--warning
--warning-soft
--danger
--danger-soft
--success-border
--info-border
--warning-border
--danger-border
--neutral-soft
--link
--overlay
--focus
```

### Surface hierarchy

All pages use the same four-level model:

```text
Page canvas     = --atlas-page
Primary surface = --atlas-card
Elevated popup  = --atlas-popup
Subtle / inset  = --atlas-page-subtle
Border          = --atlas-border
Strong border   = --atlas-border-strong
```

`--atlas-header` is the shell surface and `--atlas-input` is the control
surface. Both map into the hierarchy above; they do not create feature-local
surface systems. Borders reinforce layer changes but must not be the only
thing separating a card from the page. Heavy card shadows are not part of the
system.

The current theme mappings are:

| Role | Light | Dark |
|---|---|---|
| Page canvas | `#F3F5F7` | `#151A20` |
| Primary surface/card | `#FFFFFF` | `#1D242D` |
| Elevated popup | `#FFFFFF` | `#222A34` |
| Subtle/inset surface | `#E9EEF3` | `#252E38` |
| Border | `#D4DCE6` | `#34404D` |
| Strong border | `#B8C4D1` | `#465463` |
| Primary text | `#24303D` | `#E8EDF2` |
| Secondary text | `#5E6C7D` | `#AAB5C1` |

The page canvas is always darker than the primary surface. Inset cells are
visibly distinct from their parent card in both themes. Neutral surfaces must
not be tinted Atlas Blue.

### Interaction and semantic mappings

| Role | Light foreground / soft / border | Dark foreground / soft / border |
|---|---|---|
| Brand interaction | `#2563EB` / `#E9EFFF` | `#2563EB` / `#1B2D4D` |
| Success / health | `#2F6B43` / `#EDF7F0` / `#B9D9C3` | `#7BC493` / `#1E3428` / `#2C513B` |
| Information | `#2E6090` / `#EDF4FA` / `#BFD2E3` | `#8BBCE4` / `#203344` / `#31506A` |
| Warning | `#7A4B0E` / `#FFF6E8` / `#E5CDA6` | `#E6B767` / `#3A3020` / `#5B492A` |
| Danger / failure | `#923842` / `#FCEFF1` / `#E8BDC2` | `#E89A9F` / `#3D272B` / `#62383E` |

Primary buttons, active navigation, links, focus rings, and selected controls
use Atlas Blue. Semantic colours communicate state only. Normal healthy state
uses compact dots and badges rather than large green-filled regions. Status
components always retain a text label, and warnings/errors retain explanatory
copy where action is required.

### Historical dark-theme reference palette

The original project palette is retained as historical context only:

```css
--bg-app: #111827;
--bg-header: #151f2d;
--bg-card: #1d2939;
--bg-subtle: #162131;
--border: #2d3a4d;
--text: #f1f5f9;
--text-muted: #9fb0c4;
--accent: #25a875;
--accent-soft: #66d9a5;
--info: #60a5fa;
--warning: #f6b85f;
--danger: #f07b7b;
```

These values should be mapped into semantic tokens rather than referenced directly throughout feature CSS.

The implementation source of truth for actual token values is:

```text
web/src/styles/design-tokens.css
```

---

## Status semantics

Use stable labels:

- Healthy
- Degraded
- Unreachable
- Converged
- Drifted
- Pending
- Applying
- Verifying
- Failed
- Maintenance
- Incompatible
- Observed Only
- Unsupported

Status roles:

| Status type | Colour role |
|---|---|
| Healthy, Converged, Success | Green |
| Informational, Observed | Blue |
| Pending, Warning, Drifted | Amber |
| Failed, Blocked, Unreachable | Red |
| Maintenance, Disabled, Unknown | Grey |
| Incompatible | Purple or red |

Colour must never be the only indicator. Use text and iconography as well.

`StatusBadge` has one compact size: 0.72rem type, 3px vertical and 8px
horizontal padding, full-pill radius, 1.4 line height, and a 6px semantic dot.
Dashboard page, controller, DNS, and node health all use this same variant. A
larger badge must be introduced only as a documented shared variant; flex/grid
parents must not stretch the compact pill.

---

## Typography

Recommended font stack:

```css
font-family: "Rubik", -apple-system, BlinkMacSystemFont, "Segoe UI", Arial, sans-serif;
```

Use the existing application font if changing fonts creates unnecessary migration risk.

### Type scale

| Token | Size | Line height | Use |
|---|---:|---:|---|
| Display | 40px | 48px | Rare onboarding or empty-state headline |
| H1 | 32px | 40px | Page title |
| H2 | 24px | 28px | Major section |
| H3 | 20px | 24px | Card or group heading |
| Body large | 18px | 28px | Introductory copy |
| Body | 16px | 24px | Main copy and controls |
| Body small | 14px | 20px | Tables, labels, metadata |
| Caption | 12px | 16px | Secondary metadata |

Use weights 400, 500, 600, and 700.

Most interface text should use 400 or 500. Avoid excessive heavy bold.

Metric values may use approximately 28–32px.

### Dashboard typography roles

The Dashboard uses the compact implemented application scale and shared
spacing tokens rather than introducing feature-local type tokens:

| Role | Implementation |
|---|---|
| Page eyebrow | `.eyebrow`: 0.72rem, 700, uppercase, muted, tracked |
| Page title | shared `h1`: 1.45rem, 600 |
| Panel eyebrow | `.eyebrow`: same semantic treatment as the page eyebrow |
| Panel and section title | shared `h2`: 1.08rem, 600 |
| Panel description | 0.88rem, 400, muted, 1.5 line height |
| Top KPI label | shared `.metric-card span`: 0.82rem, sentence case, muted |
| Top KPI value | shared `.metric-card strong`: 1.6rem, 550 |
| Summary-tile label | 0.7rem, 650, uppercase, muted, tracked |
| Summary-tile value | 1.05rem, 600 |
| Node metadata | shared `.detail-list` and `.muted` treatments |

Visual uppercase is presentation only. Panel titles remain semantic `h2`
elements and node names remain `h3` elements.

---

## Spacing

Use a 4px base scale.

Common values:

- 4px: micro spacing.
- 8px: compact gap.
- 12px: control gap.
- 16px: mobile outer padding and compact card padding.
- 20px: settings-row spacing.
- 24px: desktop card padding and major section gap.
- 32px: large section separation.
- 40px and 48px: major layout separation only.

### Layout dimensions

- Main header: approximately 64px.
- Context row: approximately 44–52px.
- Desktop page padding: 24px.
- Mobile page padding: 16px.
- Control radius: 6px.
- Card radius: 8px.
- Dialog radius: 12px.
- Badge radius: full pill.

Prefer dividers over excessive nested cards.

### Panel and card anatomy

Bordered panels use one predictable hierarchy:

```text
panel border
  header: title, description, optional actions
  divider
  body: content, nested surfaces, table or form
  optional wrapping action row
```

The shared `SettingsGroup` supports two intentional body-spacing modes:

- `rows` for self-padding `SettingRow` or equivalent divided-row content;
- `padded` for ordinary content, nested forms, action groups, summary tiles,
  empty states, and bordered tables.

Padded bodies use 20px block inset inside the panel's 24px desktop or 16px
mobile inline inset. Ordinary cards use 24px padding on desktop and 16px on
phones. Nested operational forms use the shared bounded `panel-form` treatment
so fields remain readable on wide screens while retaining the full available
width on small screens.

Do not place direct content into the row mode unless that content owns its
block spacing. A divider must always be followed by either body padding or the
first self-padding row. Buttons must not rely on the outer border as their
visual inset.

---

## Page width classes

Every canonical route receives one shared width class from the route table.
Feature code must not rely on an omitted container to become accidentally Full
or define arbitrary max-width values.

### Narrow

Use for:

- Login.
- Focused forms.
- Route-state pages.

### Standard

Available for bounded embedded content and focused compositions, but no
canonical authenticated application route currently uses this width.

### Wide

Use for:

- Dashboard.
- Statistics and Query Log.
- Settings and Filters.
- Nodes.
- Node Detail and HA Operations.
- Configuration Control.
- Revisions.
- Deployments.
- Drift.
- Setup Guide.
- All System administration routes.

### Full

Reserved for a future route whose content demonstrably requires the complete
available desktop width. No current canonical route uses Full.

Every canonical authenticated application route shares the Dashboard's Wide
primary-application measure. This keeps Monitoring, Settings, Filters, HA
Controller, Administration, and Setup Guide aligned as operators move between
sections. Mobile always uses the available inline size regardless of the
desktop maximum.

### Canonical route assignment

| Routes | Width | Content reason |
|---|---|---|
| `/` | Wide | Dashboard cards and node overview. |
| `/statistics` | Wide | Matches Dashboard and provides room for charts, rankings, and node coverage. |
| `/settings/general`, `/settings/dns`, `/settings/encryption`, `/settings/clients`, `/settings/dhcp` | Wide | Primary configuration forms, capability context, and structured tables. |
| `/filters/blocklists`, `/filters/allowlists`, `/filters/rewrites`, `/filters/blocked-services`, `/filters/custom-rules` | Wide | One coherent Filters family with tables, catalogues, and editors. |
| `/query-log` | Wide | Matches Dashboard while its investigation table scrolls locally when required. |
| `/ha/nodes`, `/ha/nodes/{nodeId}`, `/ha/operations`, `/ha/notifications`, `/ha/configuration`, `/ha/revisions`, `/ha/deployments`, `/ha/drift` | Wide | Operational tables, comparisons, grids, and lifecycle controls. |
| `/setup-guide` | Wide | Aligns reference/help with the primary application frame. |
| `/system/users`, `/system/audit`, `/system/operational-status`, `/system/settings`, `/system/backups`, `/system/updates`, `/system/about` | Wide | Aligns Administration and Monitoring with Dashboard; dense tables remain locally scrollable. |

---

## Navigation styling

- Keep top-level labels compact.
- Use an Atlas Blue active edge, text treatment, and soft background.
- Avoid large filled navigation tabs.
- Active child routes mark the parent menu active.
- Active child routes keep their parent group open.
- Desktop and mobile navigation use the same labels and hierarchy.
- Navigation must be keyboard accessible.
- Desktop groups use native disclosure buttons. Enter/Space toggles; Arrow Down
  or Arrow Right opens and focuses the first child. Collapsed controls retain
  accessible labels and title tooltips.
- Mobile groups are controlled peer disclosures, never depend on hover, and
  the drawer restores focus to its trigger when dismissed.

---

## Buttons

### Primary

Use for the single main page action:

- Add Node.
- Save Draft.
- Publish Revision.
- Deploy Revision.

Only one primary action should normally appear in a page header.

### Secondary

Use for:

- Compare.
- Test.
- Refresh.
- Export.
- Cancel.
- View details.

### Danger

Use only for destructive actions:

- Remove Node.
- Delete Draft.
- Clear Query Log.
- Reset Statistics.
- Reset DHCP.
- Revoke Session.

### Ghost or text action

Use for low-frequency contextual actions.

All buttons require:

- Default.
- Hover.
- Active.
- Focus.
- Disabled.
- Loading states.

---

## Forms

### SettingRow

Use for:

```text
Setting title and description              Control
Optional warning or help                   Optional status
```

Desktop uses two columns.

Mobile moves the control below the description.

### SettingsGroup

Use for:

- Related switches.
- Radio selections.
- Small option groups.

### Field

Every field contains:

- Label.
- Control.
- Optional help text.
- Validation message.
- Optional suffix or unit.
- Optional scope indicator.

### Control rules

- Boolean → switch.
- Small enum → radio or segmented control.
- Large enum → select.
- Duration → presets plus custom duration.
- IP or CIDR → validated network field.
- Known ID catalogue → grouped searchable selector.
- Structured list → table or repeatable rows.
- Domain-specific rules → specialist editor.
- Secret → redacted inventory or secret reference.

### Draft state

Settings pages must show:

- Unsaved changes.
- Save Draft status.
- Current active revision.
- Scope.
- Capability warnings.
- Affected nodes.

Saving a draft must not imply deployment.

---

## Tables

Use tables for:

- Nodes.
- Clients.
- Blocklists.
- Allowlists.
- DNS rewrites.
- Static leases.
- Active leases.
- Revisions.
- Deployments.
- Drift.
- Query records.

Table rules:

- Use 14px body text.
- Keep row density compact.
- Use subtle row hover.
- Place actions at the right.
- Include node attribution in cluster views.
- Use text plus icon or badge for status.
- Provide loading, empty, filtered-empty, error, stale, and partial-success states.
- Use horizontal scrolling or structured mobile rows where necessary.
- Do not create unique table styling per feature.
- Operational record details use an adjacent expandable table row with a real
  disclosure button, `aria-expanded`, and `aria-controls`. Only one row is open
  per table. Revisions, Deployments, and Drift persist the selected ID in the
  URL and scroll a valid deep link into view once after loading.
- Very large secondary data, such as a complete immutable revision document,
  stays behind an independently operable collapsed disclosure inside the
  inline detail.

Tables inside bordered panels use the padded-table pattern by default: the
shared horizontal scroller remains a bordered nested surface inside a padded
panel body. Its hover background and horizontal overflow are clipped by that
surface, while the panel body supplies bottom and side inset. Loading and empty
states occupy the same padded body. A full-bleed table is allowed only as an
explicit documented variant with deliberate outer-border and footer handling;
it must not result from omitting body padding.

---

## Cards

Use cards only for coherent groups:

- Related settings.
- Dashboard metrics.
- Node status.
- Deployment summary.
- Revision summary.
- Capability or warning panels.

Do not put every individual field into its own card.

Panel action groups use the shared wrapping row with an 8px token gap and
aligned controls. Header actions remain in the panel header; body actions stay
inside the padded body or a deliberately separated footer. Primary, secondary,
and destructive hierarchy is preserved when the row wraps.

### Dashboard information hierarchy

The Dashboard answers, in order: whether DNS, APIs, HA, and collection are
healthy; what needs attention; what DNS is doing; what changed; and the state
of each node.

- The five primary evidence cards are DNS Serving, API Reachable, HA Status,
  Collection, and Attention.
- Attention composes existing Operational Status, HA, drift, deployment,
  certificate, and update evidence. It does not create a second alert engine.
- DNS activity uses the canonical 24-hour Statistics report for Queries,
  Blocked percentage, Safety Interventions, and Average Processing. Coverage
  diagnostics remain on Statistics and Operational Status.
- Recent Changes composes safe revision, deployment, and audit summaries.
- Nodes shows supported DNS, API, version, last-seen, update, and management
  fields. It does not invent a primary/standby role.
- Top queried and blocked domains reuse Statistics rankings.
- Unknown data is not rendered as zero. Loading, unavailable, refresh-error,
  and partial-report copy remains explicit while the panels stay discoverable.

The HA Controller Nodes inventory summary uses the five-card compact health
surface shared with Operational Status, HA Operations, and Node Detail. The
cards retain fleet health, degraded, unreachable, incompatible, and last
observation evidence while reflowing from five columns to three, two, and one.
The shared four-tile convergence summary remains unchanged for Drift and other
consumers.

---

## Shared components

### Application shell

- AppShell
- ApplicationSidebar
- UtilityTopBar
- PrimaryNavigation
- NavigationGroup
- MobileNavigationDrawer
- ContextBar
- ClusterSelector
- ScopeSelector
- ActiveRevisionIndicator
- ActiveDeploymentIndicator
- PageContainer
- PageHeader
- NotFoundPage

### Settings and forms

- SettingRow
- SettingsGroup
- Field
- DurationField
- NetworkListField
- DomainListField
- UrlListField
- OrderedTextEditor
- RuleEditor
- ScopeIndicator
- CapabilityWarning
- SecretField
- UnsavedChangesNotice

### Structured resources

- DataTable
- FilterListTable
- ClientTable
- RewriteTable
- LeaseTable
- ServiceCatalogue
- ServiceGroup
- ServiceToggle
- ScheduleEditor
- TagMultiSelect
- IdentifierListEditor
- UpstreamEditor

### HA display

- HealthBadge
- StatusBadge
- NodeBadge
- NodeStatusCard
- RevisionBadge
- DriftBadge
- HealthSummaryCard
- MetricCard
- SummaryTileGrid
- ConvergenceSummary
- StructuredDiff
- DeploymentProgress
- ProgressTimeline
- PartialSuccessPanel

### Feedback and overlays

- Banner
- Toast
- EmptyState
- ErrorState
- LoadingSkeleton
- Dialog
- ConfirmationDialog
- ConfirmDialog
- DeploymentDialog
- DriftResolutionDialog
- OperationalCommandDialog

Feature pages may compose shared components but must not create local replacements for:

- Navigation.
- Buttons.
- Cards.
- Tables.
- Dialogs.
- Settings rows.
- Badges.
- Duration controls.
- Structured list editors.
- Destructive confirmation.

---

## Feature-specific control expectations

### Blocked Services

Use:

- Searchable grouped service catalogue.
- Human-readable names.
- Service toggles or checkboxes.
- Selected count.
- Group actions.
- Schedule editor.
- Compatibility warnings.

Do not use a free-text service-ID field.

### Blocklists and Allowlists

Use:

- Separate tables.
- Enabled state.
- Add/edit dialogs.
- URL validation.
- Refresh selected and refresh all.
- Rule count.
- Last-update metadata.
- Per-node application state.

### Clients

Use:

- Searchable table.
- Add/edit dialog.
- Validated identifiers.
- Tag selector.
- Grouped safety controls.
- Inheritance controls.
- Reusable blocked-service selector.
- Removal confirmation.

### DNS Rewrites

Use:

- Searchable table.
- Validated domain and answer fields.
- Add/edit dialog.
- Delete confirmation.
- Draft and convergence context.

### DHCP

Use:

- Node-specific interface selector.
- Validated network controls.
- Active-DHCP check.
- Active leases table.
- Static leases table and dialogs.
- Explicit one-active-node state.

### Custom Filter Rules

Use:

- Specialist text or code editor.
- Syntax help.
- Validation.
- Test-host action.
- Search.
- Line-level revision comparison.

---

## Feedback hierarchy

Use:

1. Inline validation for field errors.
2. Card-level error for a failed section.
3. Page banner for blocking page errors.
4. Persistent operation panel for deployment or reconciliation failure.
5. Toast for brief non-blocking confirmation.

Do not use a toast as the only record of a deployment failure.

---

## Loading and async operations

For:

- Node checks.
- Imports.
- Validation.
- Deployments.
- Reconciliation.
- Filter refresh.
- Operational commands.

The UI must:

- Show operation started.
- Preserve operation identity.
- Allow safe navigation away.
- Surface completion globally.
- Show per-node results.
- Distinguish partial success.
- Link to persistent details.

---

## Accessibility

- Target WCAG AA contrast.
- Use visible keyboard focus.
- Make disclosures, drawers, account menus, and navigation keyboard accessible.
- Use native controls or accessible equivalents.
- Trap and return focus in dialogs.
- Move focus to page title after route navigation.
- Move focus to the first invalid field after failed submission.
- Use text labels in addition to colour.
- Provide textual or tabular equivalents for charts.
- Use approximately 44px touch targets where practical.
- Announce long-running operation state changes.

---

## Theme behaviour

- Support light, dark, and system preference.
- Use identical semantic token names in both themes.
- Persist browser-local preference under one provider; do not turn it into a
  global controller setting.
- Default invalid, missing, or unavailable persistence to System.
- Resolve System through `prefers-color-scheme` and react to OS changes only
  while System is selected.
- Apply the resolved theme before first paint where practical and keep browser
  theme-colour metadata synchronized.
- Review charts, tooltips, dialogs, code editors, badges, and tables separately in both themes.
- Avoid neon effects or excessive contrast in dark mode.
- Use a real approximately 44px icon button and an explicit keyboard/touch menu
  for Light, Dark, and System. The selected preference remains visible through
  icon, accessible name, and checked menu state; no hidden gesture is required
  to return to System.

Approved brand colours are Atlas Blue `#2563EB`, Atlas Teal `#0EA5A3`, Atlas
Charcoal `#111111`, and White `#FFFFFF`. Primary interaction may use Atlas Blue;
semantic success, warning, error, and information roles remain independent.
Theme-specific logos use supplied assets rather than CSS filters. Asset and
persistence details are in `theme-brand-and-pwa.md`.

---

## Component governance

Every shared component should include:

- A documented purpose.
- Supported variants.
- Loading, disabled, and error states.
- Mobile example.
- Light and dark examples.
- Unit or interaction tests.
- Accessibility review.
- Storybook or equivalent showcase where available.

Existing frontend presentation is not authoritative where it conflicts with
this design system, [Frontend Design](frontend-design.md),
[Feature Presentation Rules](feature-presentation-rules.md), or ADR-0026.

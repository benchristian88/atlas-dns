# Frontend Design

## Design goal

The frontend is an operational management surface for multiple AdGuard Home
nodes. It should feel familiar to an AdGuard Home operator while making cluster
scope, node attribution, desired state, deployment progress, drift, and partial
observability explicit.

The controller remains outside the DNS request path. Frontend language and
workflows must not imply that the controller resolves, proxies, balances, or
fails over normal DNS traffic.

## Frontend architecture

The React and TypeScript application is served by the controller and uses the
typed `/api/v1` client layer. Feature code owns page composition and transient
interaction state; shared components own stable visual and accessibility
patterns. Server state remains distinct from form state.

Pages must model initial loading, empty, error, stale, unavailable, unsupported,
maintenance, and partial-success states where those states are meaningful.
Controller APIs remain the authority for validation, capability checks,
optimistic concurrency, destructive-operation eligibility, and secret
redaction.

Current detailed references are:

- [design system](design-system.md);
- [component catalogue](component-catalogue.md);
- [navigation and application shell](navigation-and-shell.md);
- [route reference](ui-navigation.md);
- [feature presentation rules](feature-presentation-rules.md).

## Application shell

Desktop uses a persistent, collapsible left rail. Mobile uses a modal drawer
with the same labels, grouping, and route ownership. A thin utility top bar
shows the selected cluster, cluster or node scope, active revision, health,
refresh state, and active deployment when present.

The shell owns navigation, theme selection, user actions, and shared context.
Feature pages do not reproduce primary navigation or maintain a competing scope
model. Unknown routes render Not Found and never fall through to Dashboard.
Settings/Filters pages also do not repeat a generic Scope/state card: the shell
owns cluster, scope, active revision, health, and deployment context, while page
notices own unsaved state and page-specific capability impact.

## Navigation model

The primary product areas are:

- Dashboard;
- Monitoring for Statistics, Query Log, and Operational Status;
- Settings for General, DNS, Encryption, Clients, and DHCP;
- Filters for blocklists, allowlists, rewrites, blocked services, and custom
  rules;
- HA Controller for Nodes, HA Operations, Notifications, Configuration
  Control, Revisions, Deployments, and Drift;
- Administration for users, audit, controller settings, backups, updates, and
  About; and
- Setup Guide as a help/reference utility.

Settings and Filters are exclusively AdGuard Home desired configuration.
Notifications is an Atlas controller function and must not appear under
Settings. There is no general Integrations destination.

The canonical route table, compatibility redirects, route ownership, and menu
behavior are defined in [UI Navigation](ui-navigation.md) and
[Navigation and Application Shell](navigation-and-shell.md).

## Page patterns

Top-level pages use a title, concise purpose, relevant context or freshness
state, and one clear primary task. Repeated resource collections use searchable
tables or responsive cards; selection opens query-backed inline detail when a
durable record must remain linkable.

Settings pages edit the shared draft. They expose lifecycle context and link to
Configuration Control for validation and publication. Saving a settings page
does not publish or deploy.

HA pages have separate responsibilities:

- Nodes owns managed node identity, observation, compatibility, and health.
- Configuration Control owns draft review, validation, observation/import or
  adoption, and publication.
- Revisions owns immutable history, semantic comparison, deployment preview,
  and rollback by deploying a historical revision.
- Deployments owns durable execution progress and per-node results.
- Drift owns current desired-versus-observed divergence and resolution.

Operational commands use explicit scope, confirmation where destructive,
durable per-node results, and audit evidence. Secret values never enter route
state, visible frontend state, diagnostics, or exports.

## Dashboard model

Dashboard is a concise cluster-health overview, not a replacement for detailed
feature pages. It presents five evidence-backed primary cards where data is
available—DNS Serving, API Reachable, HA Status, Collection, and Attention—and:

- node health and HA redundancy;
- controller subsystem state for API, Statistics, and Query Log;
- DNS activity summaries for queries, blocked percentage, safety interventions,
  and query-weighted average processing time;
- actionable operational/HA, drift, deployment, certificate, and update state;
- recent safe revision, deployment, and audit summaries;
- compact node DNS/API/version/freshness/update state; and
- top queried and blocked domains.

Statistics and Query Log summaries preserve freshness, coverage, partial-input
state, and source-node attribution. Detailed coverage and investigation remain
on their owning pages.

## Component and design principles

- Use project-owned visual assets and an original AdGuard-inspired interface.
- Preserve the desired draft → immutable revision → verified deployment
  lifecycle in labels and actions.
- Present domain controls, not raw transport payloads.
- Keep node-specific values distinct from shared desired configuration.
- Show capability differences and unavailable operations explicitly.
- Use text with semantic color; color alone never communicates status.
- Prefer accessible native controls and predictable keyboard interaction.
- Keep destructive actions narrow, server-validated, and strongly confirmed.

## Responsive behavior

Desktop is the primary administration surface, but every operator workflow must
remain usable at narrow widths. Cards wrap, secondary table columns collapse or
move into detail, and dialogs or complex comparisons may become focused
full-screen views. The context needed to understand scope, health, freshness,
and node attribution must remain visible.

The mobile drawer is an alternate presentation of the same hierarchy, not a
different information architecture. Controls must not rely on hover, and
closing a drawer or dialog restores focus to its trigger. Dashboard layouts
reflow at large-desktop, laptop, tablet, and phone widths; wide node/query
tables retain local scrolling.

The authenticated shell has an explicit inline-size containment contract. It
does not depend on document-level overflow clipping; context and data tables own
their intentional local scrolling. Safari/iOS safe-area insets are respected,
and browser zoom remains available.

## Accessibility

Target WCAG AA contrast, visible focus, keyboard navigation, semantic headings,
native form controls, labelled status, and live announcements for meaningful
asynchronous progress. Charts supplement textual values and never carry the
only copy of critical information.

## Historical design record

Release-by-release frontend implementation notes, migration specifications,
regression evidence, and screenshots are retained in the
[pre-1.0 physical archive](../archive/pre-1.0/README.md). They are evidence of
how the interface evolved, not current design authority.

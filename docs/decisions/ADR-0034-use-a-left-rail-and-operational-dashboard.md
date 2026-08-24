# ADR-0034: Use a left rail and operational dashboard

## Status

Accepted.

## Date

24 August 2026.

## Context

ADR-0026 established the AdGuard Home-inspired operator language, grouped
configuration ownership, global context, and controller lifecycle. Its
horizontal navigation presentation served the pre-1.0 migration, but the
stable product now has Monitoring, HA lifecycle, and Administration surfaces
that exceed a compact header's useful capacity.

Operators need to distinguish managed AdGuard Home configuration from Atlas
controller functions and answer DNS, node, HA, collection, attention, and
recent-change questions without navigating several pages. The controller
remains single-controller, outside the DNS request path, and dependent on
durable node-owned evidence rather than browser-derived health.

## Decision

Atlas DNS Controller will:

1. Use a persistent, collapsible left rail on desktop and the same hierarchy
   in a modal mobile drawer.
2. Reserve the top bar for cluster/scope, revision/health/refresh context,
   theme, supported notifications, and account utilities.
3. Organize routes as Dashboard; Monitoring; AdGuard Home Settings; AdGuard
   Home Filters; HA Controller; Administration; and Setup Guide help/reference.
4. Place Operational Status under Monitoring and Notifications under HA
   Controller without changing their underlying controller contracts.
5. Keep all supported routes, compatibility redirects, authentication guards,
   deep links, browser refresh, and explicit Not Found behavior.
6. Build Dashboard only from existing Nodes, HA, Operational Status,
   Statistics, revision, deployment, drift, version, and safe audit APIs.
7. Never invent traffic, alerts, node roles, multi-cluster behavior, or
   unavailable values to match a visual concept.

The left-rail and route-grouping portions of this ADR supersede the horizontal
shell presentation in ADR-0026. ADR-0026 remains authoritative for the
AdGuard-inspired operator model, domain controls, global context, lifecycle,
capability, redaction, DHCP, and controller-independence decisions.

## Architecture boundary

This is a frontend information-architecture and presentation decision. It adds
no database migration, API contract, notification policy, onboarding flow,
runtime-setting migration, configuration schema change, or DNS-path behavior.
Collapsed-rail and theme preferences remain browser-local presentation state.

## Failure and security behavior

- Node inventory is the essential Dashboard source and uses a retryable error.
- Supplementary source failures retain available data with an explicit partial
  warning.
- Missing metrics are unavailable, never zero by default.
- Attention is a projection of existing operational evidence, not a second
  alert engine.
- Recent audit items expose safe action/resource labels only, never metadata or
  secrets.
- Wide tables own local scrolling; the document does not rely on accidental
  horizontal overflow.

## Consequences

- Navigation scales without duplicating primary routes in the top bar.
- AdGuard Home configuration and Atlas controller functions have explicit
  ownership.
- Desktop, tablet, and phone layouts share one route hierarchy.
- Dashboard creates additional bounded reads of existing presentation-ready
  APIs and must preserve partial-source states.
- The project-owned icon renderer becomes the single shell icon vocabulary;
  no second icon framework is introduced.

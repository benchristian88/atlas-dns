# ADR-0026: Adopt an AdGuard Home v2-Inspired Operator Model

## Status

Accepted.

## Context

AGH HA Controller Release 0.4 has a strong desired-state, revision, deployment, verification, and drift architecture, but its frontend presentation does not consistently match the operator model of AdGuard Home.

The implementation audit identified:

- flat sidebar navigation rather than familiar horizontal grouped navigation;
- stale schema-v1 presentation in Configuration Control;
- raw service IDs instead of a Blocked Services catalogue;
- filter subscriptions, clients, rewrites, leases, identifiers, tags, and ignored domains presented as textareas or oversized inline forms;
- missing global scope, revision, health, and active-deployment context;
- unknown paths capable of rendering Dashboard;
- several missing audited operational commands.

The goal is not to copy AdGuard Home source code or assets. The goal is to adopt the familiar operator-facing concepts and extend them for HA control.

## Decision

AGH HA Controller will:

1. Adopt horizontal primary navigation:

```text
Dashboard
Statistics
Settings
Filters
Query Log
HA Controller
Setup Guide
```

2. Use grouped dropdown menus:

Settings:

- General
- DNS
- Encryption
- Clients
- DHCP

Filters:

- DNS Blocklists
- DNS Allowlists
- DNS Rewrites
- Blocked Services
- Custom Filter Rules

HA Controller:

- Nodes
- Configuration Control
- Deployments
- Drift
- Change History

3. Add a global context row containing:

- cluster;
- Entire Cluster or selected node;
- active revision;
- cluster health;
- active deployment.

4. Treat the AdGuard Home API as a transport contract, not a UI specification.

5. Present known domain concepts with suitable controls:

- service IDs as a grouped searchable catalogue;
- subscriptions as tables;
- clients and rewrites as tables plus dialogs;
- durations as friendly duration controls;
- identifiers and networks as validated structured lists;
- custom rules as a specialist editor.

6. Preserve the controller lifecycle:

```text
Save Draft
→ Publish Revision
→ Deploy
→ Verify
→ Reconcile
```

7. Preserve:

- capability-aware preflight;
- immutable revision history;
- durable sequential deployment;
- read-back verification;
- TLS redaction;
- single-active DHCP safety;
- controller independence from DNS traffic.

8. Implement the change as Release 0.4.1 through staged migration rather than a big-bang rewrite.

9. Keep the five HA Controller destinations as distinct task surfaces:

- Nodes owns managed infrastructure, health, compatibility, availability, and
  candidate registration/editing. Exact Node Detail owns existing-node tests
  and maintenance decisions.
- Configuration Control owns the mutable-draft approval, validation, publication, and advanced adoption workflow.
- Deployments owns durable execution events and per-node verification results.
- Drift owns current convergence incidents, policy, restore/adopt decisions,
  and read-only maintenance context; maintenance mutation links to Node Detail.
- Change History owns immutable revision history, semantic revision comparison, and deployment-based rollback.

Shared comparison and status primitives may be reused, but canonical navigation
items must not render the same page merely because their backend data comes
from the same control-plane service.

## Consequences

### Positive

- Familiar product navigation.
- Better discoverability.
- Reduced raw-data presentation.
- Reusable components for Releases 0.5 and 0.6.
- Lower risk of future large UI rewrite.
- Stronger distinction between routine settings and HA lifecycle control.
- Clear forward-looking, execution, convergence, infrastructure, and historical ownership within HA Controller.

### Negative

- Release 0.5 pauses while the frontend foundation is corrected.
- Existing routes and components require migration.
- Visual baselines and browser tests require updates.
- Some missing controller read/command operations must be added carefully.

## Alternatives rejected

### Complete functionality first, redesign later

Rejected because later statistics, query-log, and operational pages would multiply the migration cost.

### Pixel-for-pixel AdGuard Home clone

Rejected because AGH HA Controller has different lifecycle, scope, and HA requirements, and must maintain an original implementation.

### Retain the sidebar

Rejected because the approved product direction is to match AdGuard Home's top-level mental model and add one HA Controller menu.

## Implementation

Follow:

- `docs/archive/pre-1.0/planning/release-0.4.1-ui-alignment-roadmap.md`;
- `docs/development/regression-safety-rules.md`;
- `docs/frontend/feature-presentation-rules.md`.

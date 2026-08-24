# HA Controller Responsibility Separation

## Outcome

The v1.1 HA Controller navigation contains seven canonical destinations. The
five configuration-lifecycle routes retain distinct task surfaces, while HA
Operations and Notifications expose existing controller lifecycle capability:

| Route | Responsibility |
|---|---|
| `/ha/nodes` | Managed infrastructure, health, compatibility, capabilities, observation freshness, applied revision, latency, convergence, and node actions |
| `/ha/operations` | DNS capacity, maintenance, certificates, guided upgrades, and retained operational history |
| `/ha/notifications` | Atlas HA lifecycle webhook channels; encrypted write-only destinations and bounded tests |
| `/ha/configuration` | Forward-looking schema-v2 draft/change review, whole-draft validation, immutable publication, and advanced observation/import/adoption |
| `/ha/revisions` | Immutable revision list and adjacent inline detail, revision comparison, deployment status, preview/confirmation, and deployment-based rollback |
| `/ha/deployments` | One unified active and historical execution table with inline ordered per-node tasks, safe failure detail, verification, request correlation, and cancellation |
| `/ha/drift` | Current convergence summary and inline semantic incidents, restore/adopt/maintenance, related resources, and separated cluster policy |

`/ha/history` redirects to `/ha/revisions`. Unknown paths continue
to render Not Found, and trailing-slash redirects retain query strings and
fragments.

## Architecture boundary

This is an information-architecture and presentation change. It does not add a
database migration, notification policy, or controller endpoint. The pages reuse existing typed
controller APIs and the shared semantic-diff/status primitives. Schema-v2
desired state, optimistic draft concurrency, immutable revisions and hashes,
capability-aware preflight, durable sequential stop-on-failure deployment,
read-back verification, total-success activation, drift policy, rollback,
maintenance, TLS redaction, and DHCP safety remain unchanged.

Observation import and drift adoption update only the mutable draft. Both flows
lead to Configuration Control and still require validation and publication.
Revisions deploys an existing immutable revision and never edits or
republishes it.

## Failure behavior

- Initial read failures show the shared retryable error state.
- Refresh errors remain visible without inventing successful state.
- Cancellation is offered only for controller-supported active deployment
  states and remains safe-boundary cancellation.
- Missing actor display names, drift severity/source/acknowledgement, retry
  commands, and nested detail routes are not fabricated from incomplete data.
- Nodes uses controller inventory and control-plane reads only; the browser
  never calls an AdGuard Home `/control/` endpoint.

## Validation

Responsibility tests prove that Configuration Control excludes immutable
history/rollback, Revisions excludes draft publication/import,
Deployments does not load drift, and Drift does not load deployment history.
They also cover the enriched Nodes presentation and Axe structural WCAG A/AA
checks for the four separated lifecycle pages. Route tests cover each distinct
route kind, canonical routes, redirects, and Not Found.

## Deliberately deferred

- Actor UUID-to-display-name resolution.
- Persisted drift severity, likely source, acknowledgement, or ignore policy.
- A dedicated retry-deployment controller command.
- Nested revision/deployment/drift/node detail routes.
- Draft-versus-historical-revision comparison.
- Full authenticated real-browser critical-flow automation and new per-page
  visual baselines.

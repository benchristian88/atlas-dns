# v1.1 page-responsibility audit follow-up

This is the implementation follow-up to the completed v1.1 page-responsibility
audit. It does not replace or rewrite the audit findings as a plan. Current
operator behavior is authoritative in the Administration guide, Controller API,
database schema, and frontend design system.

## Evidence/data findings completed

- Audit Log answers **who changed something**. It uses compact rows, accessible
  adjacent detail, `auditEventId` selection, exact older-event retrieval, and
  opaque `(created_at, id)` cursor pagination.
- Known audit actions use typed allowlisted fields. Previous/new is used only
  where both were recorded. Persistence, API representation, and the unknown UI
  fallback enforce bounded redaction; raw JSON is not the primary design.
- Actor rows use a current resolvable label where available and retain the
  immutable actor UUID in detail without claiming a historical name snapshot.
- Canonical links target existing Node, Revision, Deployment, Drift, Users,
  Settings, Notifications, Configuration, and HA routes. No audit-only resource
  route was invented.
- Dashboard remains a current-cluster overview. Health, HA, collection,
  Attention, Nodes, revisions/deployments, drift, Recent Changes, DNS activity,
  and rankings are cluster-wide. Statistics alone owns the optional Entire
  Cluster/individual-node traffic selector.
- Recent Changes reads a database-scoped selected-cluster audit feed plus an
  explicit Controller-global allowlist. Other clusters are excluded before the
  query limit. Dashboard never renders audit metadata.
- Revision/deployment domain records win only over audit twins matched by
  durable resource UUID, type, and lifecycle action. Timestamp then ID controls
  final ordering. Audit-only items link to the exact Audit Log event.
- A failed Recent Changes source retains valid other sources and produces a
  scoped partial warning; it does not blank Dashboard health.

## Boundaries preserved

Audit Log remains separate from Operational History: the former is actor and
administration evidence, while the latter is retained operational outcome and
delivery evidence. Recent Changes remains a concise attention aid rather than a
third history system.

The controller remains outside the DNS request path. No configuration,
deployment, drift, credential, or revision database semantics changed. Audit
metadata redaction is additive defense around the existing append-only record.

## Deferred decision

Non-archived failed deployments may continue appearing in Attention as
historical evidence. Acknowledgement/resolution needs a separate domain and
product decision. v1.1 does not infer or persist either state. Historical actor
display-name snapshots are also deliberately outside this workstream.

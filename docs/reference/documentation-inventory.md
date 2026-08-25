# Documentation Inventory

This inventory records the physical location and authority of each significant
documentation family. A family row classifies every file beneath that path
unless a more specific row overrides it.

| Current path or family | Classification | Authority / status | Historical |
|---|---|---|---|
| `README.md` | CURRENT PRODUCT DOCUMENTATION | Product front door and quick start. | No |
| `SECURITY.md` | CURRENT PRODUCT DOCUMENTATION | Vulnerability reporting policy. | No |
| `CONTRIBUTING.md`, `AGENTS.md` | CURRENT DEVELOPMENT / CONTRIBUTOR DOCUMENTATION | Repository contribution and agent rules. | No |
| `CHANGELOG.md` | HISTORICAL VALIDATION / RELEASE EVIDENCE | Authoritative release chronology. | Yes |
| `docs/README.md` | CURRENT PRODUCT DOCUMENTATION | Canonical documentation entry point. | No |
| `docs/getting-started/*` | CURRENT PRODUCT DOCUMENTATION | Installation guidance. | No |
| `docs/user-guide/*` | CURRENT PRODUCT DOCUMENTATION | Operator workflows. | No |
| `docs/administration/*` | CURRENT PRODUCT DOCUMENTATION | Controller administration. | No |
| `docs/operations/runbook.md`, `backup-and-restore.md`, `compatibility-matrix.md`, `upgrade-policy.md` | CURRENT PRODUCT DOCUMENTATION | Runtime, recovery, compatibility, and supported-upgrade guidance. | No |
| `docs/operations/backup-format.md` | CURRENT ARCHITECTURE / TECHNICAL REFERENCE | Portable archive contract. | No |
| `docs/operations/release-1.0-readiness.md` | RELEASE EVIDENCE | Local validation results and explicit external 1.0 gates. | No |
| `docs/product/support-and-deprecation-policy.md` | CURRENT PRODUCT DOCUMENTATION | Stable 1.x community support boundary. | No |
| `docs/reference/features.md` | CURRENT PRODUCT DOCUMENTATION | Sole authoritative feature catalogue. | No |
| `docs/reference/rename-inventory.md` | RELEASE EVIDENCE | Release 1.0 identifier-family audit and retained-reference rationale. | No |
| `docs/reference/documentation-inventory.md` | CURRENT DEVELOPMENT / CONTRIBUTOR DOCUMENTATION | Documentation governance and classification. | No |
| `docs/architecture/*`, `docs/api/*`, `docs/backend/*`, `docs/database/*`, `docs/diagrams/*` | CURRENT ARCHITECTURE / TECHNICAL REFERENCE | Current system and contract references. | No |
| `docs/security/security.md` | CURRENT PRODUCT DOCUMENTATION | Security controls and operator guidance. | No |
| `docs/decisions/README.md`, `docs/decisions/ADR-*.md` | ADR / DECISION HISTORY | Canonical individual decisions and status register. | Yes |
| `docs/frontend/design-system.md`, `component-catalogue.md` | CURRENT ARCHITECTURE / TECHNICAL REFERENCE | Current visual primitives and shared components. | No |
| `docs/frontend/frontend-design.md`, `navigation-and-shell.md`, `ui-navigation.md` | CURRENT ARCHITECTURE / TECHNICAL REFERENCE | Current frontend architecture, shell, routes, and navigation. | No |
| `docs/frontend/feature-presentation-rules.md`, `ha-controller-responsibility-separation.md`, `query-log.md`, `operational-status.md`, `theme-brand-and-pwa.md` | CURRENT ARCHITECTURE / TECHNICAL REFERENCE | Current frontend behavior and safety boundaries. | No |
| `docs/frontend/page-responsibility-audit-follow-up-1.1.md` | RELEASE EVIDENCE | v1.1 evidence/data finding implementation status and explicit deferrals; not a replacement audit. | No |
| `docs/frontend/reference/*` | CURRENT ARCHITECTURE / TECHNICAL REFERENCE | Upstream AdGuard Home comparison/reference material; not controller design authority. | No |
| `docs/development/coding-standards.md`, `local-development.md`, `regression-safety-rules.md`, `release-process.md`, `testing.md` | CURRENT DEVELOPMENT / CONTRIBUTOR DOCUMENTATION | Active contributor workflow and quality gates. | No |
| `docs/roadmap/roadmap.md` | CURRENT PRODUCT DOCUMENTATION | Sole forward-looking roadmap; not a release promise. | No |
| `docs/archive/pre-1.0/README.md` | HISTORICAL PLANNING | Physical archive entry point and authority warning. | Yes |
| `docs/archive/pre-1.0/frontend/implementation/*` | HISTORICAL IMPLEMENTATION | Release 0.4/0.4.1 implementation audits, specifications, and phase evidence. | Yes |
| `docs/archive/pre-1.0/validation/*` | HISTORICAL VALIDATION / RELEASE EVIDENCE | Point-in-time regression and release validation reports. | Yes |
| `docs/archive/pre-1.0/planning/*` | HISTORICAL PLANNING | Superseded release plan, UI roadmap, and backlog. | Yes |
| `docs/archive/pre-1.0/product/frontend-alignment-brief.md` | HISTORICAL PLANNING | Original UI migration brief. | Yes |
| `docs/archive/pre-1.0/product/product-design-document.md` | HISTORICAL IMPLEMENTATION | Historical Release 0.7 product design baseline. | Yes |
| `docs/archive/pre-1.0/screenshots/*` | HISTORICAL VALIDATION / RELEASE EVIDENCE | Point-in-time visual regression evidence. | Yes |
| Removed `docs/product/feature-ledger.md` | OBSOLETE / DUPLICATED | Deleted redirect; `docs/reference/features.md` is authoritative. | Yes |
| Removed `docs/decisions/ADR-compendium.md` | OBSOLETE / DUPLICATED | Deleted duplicate; individual ADRs are authoritative. | Yes |
| `scripts/README.md` | CURRENT DEVELOPMENT / CONTRIBUTOR DOCUMENTATION | Narrow script reference. | No |

## Governance

- Current docs describe capability and operation by task, not release chronology.
- The root changelog owns release chronology; the roadmap owns future direction.
- Individual ADRs own decision history.
- Historical implementation, validation, planning, product exploration, and
  screenshots live physically under `docs/archive/pre-1.0/`.
- Archived material is evidence, not current authority.
- A behavior change updates the relevant current guide, reference, API/schema
  contract, tests, and changelog entry in the same change.

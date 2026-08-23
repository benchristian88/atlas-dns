# Changelog

All notable changes to Atlas DNS Controller are documented in this file.

The stable 1.x line follows Semantic Versioning for documented public
interfaces.

## 1.0.2 - Unreleased

### Added

- Webhook delivery outcomes now appear as separate rows in HA Controller → HA
  Operations → Operational History, including destination name, test/real
  identity, delivery state, attempts, HTTP status, and a bounded sanitized
  failure reason.
- Operational History now uses authenticated server-side cursor pagination with
  a 50-row default, 100-row maximum, and accessible Previous/Next controls.

### Security

- Upgraded the supported Go toolchain to Go 1.27 and updated `pgx`,
  `golang.org/x/crypto`, and `golang.org/x/text` past the vulnerabilities
  identified by `govulncheck` during the 1.0.2 audit.
- Added `govulncheck` to pull-request, branch, and release gates, restricted CI
  workflow permissions to read-only repository contents, and refreshed every
  third-party GitHub Action to a current immutable commit pin.
- Added adversarial regression coverage for authenticated route boundaries,
  session and CSRF failures, mass-assignment rejection, node URL validation,
  redirect refusal, and bounded AdGuard response bodies.

### Fixed

- HA DNS failure and recovery transitions now have end-to-end regression
  coverage across durable event creation, per-channel queueing, webhook
  delivery, deduplication, and Operational History. Test Webhook now records
  the same safe operational delivery evidence as real notifications.
- Fixed the PostgreSQL notification-queue INSERT type inference error that
  rolled back real HA transition events/deliveries while the direct Test
  Webhook path continued to succeed.
- Added AdGuard Home v0.107.79 compatibility for node observation, Query Log,
  exact-range Statistics, configuration inventory/write verification, and
  mixed v0.107.78/v0.107.79 rolling upgrades.
- Removed the v0.107.78 patch ceiling that rejected node observation and
  disabled collectors before making an AdGuard API request.
- Query Log now reserves `QUERY_LOG_UNSUPPORTED` for an old contract or a
  proven missing endpoint; unknown API generations and transport/response
  failures remain distinct failed/unknown states.

### Changed

- AdGuard Home v0.107.52 remains the minimum managed version. v0.107.79 is the
  latest explicitly tested patch. Newer patches in the v0.107 API generation
  are provisionally compatible only after Atlas's typed capability and semantic
  validation succeeds; other API generations remain unknown and fail closed.
- Status and DNS inventory now validate and normalize the distinct
  `protection_disabled_duration` and `protection_disabled_until` runtime
  semantics. Additive upstream fields remain tolerated.
- Safe node diagnostics now identify the AdGuard version, method, endpoint,
  HTTP status/content type, and decode/semantic failure without retaining
  response bodies or secrets.

### Internal

- Added representative v0.107.78/v0.107.79 status, DNS, Query Log, and
  Statistics fixtures plus mixed-version, rolling-upgrade, additive-field,
  provisional-patch, missing-endpoint, and malformed-contract regressions.
- Added append-only migration `000015_release_1_0_2_notification_history` for
  bounded delivery diagnostics and the delivery/event history query index. The
  released `000001`–`000014` baseline remains unchanged.

## 1.0.1 - Unreleased

### Fixed

- Nodes can return a node from Maintenance Mode through the canonical
  fail-closed return-to-service lifecycle, reconcile the persisted server
  state, and display validation or request failures instead of silently
  leaving the action unchanged.
- Return-to-service no longer deadlocks on drift that maintenance itself keeps
  from reconciling. Required live checks still fail closed; existing drift is
  retained as an actionable warning and reconciliation resumes after exit.
- Return-to-service now treats disabled AdGuard Home TLS as explicitly not
  applicable instead of failing on retained certificate metadata. Enabled TLS
  remains fail-closed for invalid, mismatched, not-yet-valid, or expired
  certificates and returns a safe actionable reason.
- Dashboard Statistics health now retains still-fresh successful evidence
  across an intentional maintenance poll skip, and correctly rolls a genuine
  per-node partial collection up to the controller's degraded state.
- Deleting and re-adding a node now prunes only the deleted UUID's mutable draft
  override, preserves immutable history, and gives deployment preview clear
  refresh/import/publish guidance for a replacement node's new identity.
- Deployment preview now distinguishes missing capability discovery, an
  unsupported API version, unavailable feature capabilities, missing current
  observations, and an all-maintenance target set.
- The five HA Controller → Nodes summary tiles share one row above the desktop
  breakpoint and wrap to three and one columns at smaller viewports without
  changing other convergence summaries.
- Setup Guide treats a new cluster with no configured nodes as incomplete
  onboarding and keeps genuine API failures distinct from empty state.
- Authenticated layouts are width-contained on iPhone-class viewports and use
  iOS safe-area insets without disabling browser zoom.

### Changed

- Every canonical route now has a deliberate page-width assignment; Filters
  are consistently Wide, Statistics and Query Log match the Wide Dashboard,
  and all System routes use Standard.
- Replaced the header theme select with a compact accessible icon menu that
  preserves System, Light, and Dark preferences.
- Removed redundant page-local Scope/state summaries from General, DNS,
  Blocklists, Allowlists, and Blocked Services.

### Internal

- Confirmed that the immutable `000001`–`000014` migration chain shipped in
  1.0.0 is the physical v1.0 database baseline. Release 1.0.1 is schema-neutral
  and adds no migration.

## 1.0.0 - 2026-08-13

### Changed

- Completed the product and technical rename to Atlas DNS Controller / Atlas
  DNS across UI, module/imports, commands, service/user/paths, Docker/Compose,
  browser namespaces, environment variables, backup identity, build metadata,
  release infrastructure, and current documentation.
- Established 1.0 as the fresh-install stable baseline. Pre-1.0 installations
  and backups are not supported migration sources; future 1.x upgrades use
  append-only checksum-verified migrations and documented compatibility gates.
- Replaced source-build production installation paths with versioned Linux
  amd64/arm64 release archives, verified native installation, and a prebuilt
  multi-platform GHCR image consumed by Docker Compose and Portainer.

### Security and licensing

- Licensed Atlas DNS Controller under BUSL-1.1 with the owner-provided
  non-commercial personal/homelab Additional Use Grant, Apache-2.0 Change
  License, and August 12, 2032 Change Date.
- Renamed controller cookies/browser storage, preserved secret-redaction and
  update-execution boundaries, added OCI licence/source metadata, and retained
  checksum verification before native installation.

### Compatibility

- Introduced Atlas backup format identity (`.atlasdnsbackup`,
  `ATLASDNSBACKUP`, application `atlas-dns`) and explicit fail-closed rejection
  of pre-1.0 archives.
- Published stable 1.x upgrade, migration, backup, compatibility, support, and
  deprecation policies.

### Validation

- Release-candidate automation covers Go tests/race/vet/build, frontend unit,
  accessibility, asset, type, lint and build checks, native archive assembly,
  checksums, and multi-platform image publication. Clean-host/browser/real-node
  evidence remains a required pre-tag release gate and is not inferred from
  local automation.

## 0.9.2 - 2026-08-11

### Added

- Standardized Node Detail on shared page, header, settings-group, field,
  status, action, responsive, and accessibility primitives with canonical links
  to configuration, drift, deployments, telemetry, status, and audit.
- Complete encrypted webhook administration: edit with hidden-destination
  preservation or explicit replacement, enable/disable, exact-name deletion,
  bounded no-redirect test, safe endpoint summary, audit, and retained delivery
  identity after channel deletion.
- Audited archive/restore for immutable revisions and terminal deployments, plus
  transactionally restricted hard deletion of only unreferenced unused revisions
  and never-started, effect-free deployments.
- Append-only migration `000014_release_0_9_2_lifecycle_polish` for archive
  metadata, webhook endpoint summaries, and non-cascading delivery snapshots.

### Changed

- Product documentation is organized by operator task and capability, with one
  feature catalogue, product-front-door README, authoritative Docker/native
  install guides, current architecture/security/user/admin guides, a
  forward-looking roadmap, and a classified pre-1.0 history index.
- Default development version is `0.9.2-dev`. The technical product name remains
  AGH HA Controller; the Release 1.0 rename inventory does not execute the rename.

### Fixed

- Standardize shared card and divided-panel spacing with token-based padded
  bodies, wrapping action rows, contained tables, and bounded nested forms on
  Node Detail, DNS Blocklists/Allowlists, and HA notification administration.
- Prevent Dashboard node-card health badges from stretching beyond the shared
  compact status-pill dimensions.
- Align Operational Status Core Services with the shared panel header and
  semantic summary-tile hierarchy used by the established Dashboard language.

### Security

- Lifecycle mutations are administrator-only, CSRF-protected, strongly
  confirmed, audited, and recheck references under transaction locks.
- Webhook secrets are never echoed; summaries omit userinfo/path/query/fragment,
  tests are bounded, and deletion preserves operational/audit evidence.

### Validation

- Local automated and documentation validation is recorded in
  `docs/archive/pre-1.0/validation/release-0.9.2-validation.md`. Inherited external browser,
  iOS, packaged install, PostgreSQL, backup/restore, and real-node gates remain
  explicit rather than being inferred from local checks.

## 0.9.1 - 2026-08-11

### Added

- Browser-local System, Light, and Dark preference with pre-paint resolution,
  persistence, OS-change handling, synchronized browser theme colour, and an
  accessible header selector.
- Approved Atlas V3 angled-gap marks, supplied light/dark lockups, and one
  reconciled favicon, Apple touch-icon, and Android/PWA asset family.
- Automated brand/manifest asset validation covering canonical bytes, exact
  dimensions, references, technical naming, and removal of legacy icon paths.

### Fixed

- Replace independent uncontrolled desktop menus with one coordinated hover,
  click/touch, keyboard, outside-click, peer-switching, and delayed-leave model.
- Make mobile navigation groups controlled peer disclosures and restore drawer
  trigger focus after Escape or backdrop/close-button dismissal.

### Naming

- Atlas is staged as visual brand artwork only. Repository, module, binary,
  service, image, environment, configuration, API, database, manifest, and
  release-artifact technical names remain AGH HA Controller until Release 1.0.

## 0.9.0 - 2026-08-09

### Added

- Passphrase-encrypted version-1 Standard and Full controller backups using
  authenticated `age` encryption, PostgreSQL custom dumps, bounded manifests,
  SHA-256 integrity, browser download/preflight, and an offline empty-database
  restore CLI.
- First-class local User Administration with multiple administrators,
  enable/disable, credential reset, immediate session revocation, audited
  changes, and transactional final-administrator protection.
- Cached stable controller release awareness, persisted check setting, guided
  Docker/native update commands, and consistent app/build/schema metadata.
- Completed Setup Guide, System Settings, Backup & Restore, Updates, Users, and
  About pages; neutral application mark, favicon, Apple/PWA icons, and manifest.
- Append-only migration `000013_release_0_9_productisation`, compatibility and
  support policy drafts, backup-format ADR, and update-boundary ADR.

### Security

- Portable credential keys exist only inside the passphrase-encrypted payload;
  database passwords are removed from child-process arguments, uploads and
  extraction are bounded, paths are fixed, and live online restore is excluded.
- Controller update metadata accepts only bounded stable release metadata and
  repository-owned HTTPS release links. No host command or Docker socket API is
  introduced.

### Naming

- Product text remains AGH HA Controller. The deliberate Atlas DNS Controller
  rename remains Release 1.0 work.

## 0.8.0 - 2026-08-09

### Added

- Independent UDP/TCP DNS service probes, N-node HA capacity, bounded probe
  retention, and transition-only operational history.
- Preflighted planned maintenance, DHCP and deployment safety, typed
  break-glass handling, and validated return-to-service.
- Redacted certificate-expiry classification, cached AdGuard Home release
  awareness, installation-type classification, and durable guided upgrades.
- Encrypted generic webhooks with transition deduplication, maintenance-aware
  suppression, bounded retry, and safe payloads.
- HA Operations, Node Detail, Dashboard, and Operational Status integration.

### Changed

- Release 0.7 Operational Status remains the controller/integration-health
  source and now exposes DNS and redundancy as independent dimensions.
- Default controller, image, installer, and web version is 0.8.0.

### Validation

- Operator validation completed for the real-node DNS/lifecycle workflows,
  PostgreSQL migration, packaged browser experience, Docker Compose, and
  native/systemd clean-install, upgrade, restart, and rollback gates.

## 0.6.0 - 2026-08-0

### Added

- Immediate and configurable collection of normalized query events from every
  enabled compatible AdGuard Home node, using source `older_than` cursors,
  restart-safe per-node checkpoints, bounded concurrency, overlap, conservative
  node-scoped deduplication, and explicit gap evidence.
- Append-only migration `000010_release_0_6_query_log` for query events,
  ingestion checkpoints, attempts, keyset/search indexes, and bounded retention.
- Authenticated cluster-scoped query-event list and detail APIs with server-side
  domain/client search, node/status/type/client filters, deterministic cursor
  pagination, and node-attributed coverage/freshness.
- Responsive `/query-log` interface with mandatory node attribution, structured
  details, conservative refresh, older/newer navigation, partial-state notices,
  and contextual links into mutable draft authoring workflows.

### Changed

- Default controller, image, installer, and web version is 0.6.0.
- Central query-event retention defaults to seven days and is independent from
  node-local query-log enablement, anonymisation, ignore rules, and retention.

### Known limitations

- AdGuard Home does not expose a stable query-event identifier. AGH HA Controller combines a
  strong normalized fingerprint with the event's occurrence ordinal, preferring
  preservation of legitimate identical events over aggressive collapsing.
- API polling cannot recover events already removed by node-local retention;
  AGH HA Controller records and presents detected gaps. The later forwarder remains the
  higher-fidelity ingestion path.

## 0.5.0 - 2026-08-09

### Added

- Immediate and configurable interval collection of exact 24-hour, 7-day, and
  30-day AdGuard Home statistics with bounded concurrency, per-node evidence,
  capability exclusions, maintenance handling, safe errors, and retention.
- Append-only migration `000009_release_0_5_statistics` for normalized
  snapshots, collection attempts, overlap-safe hourly buckets, and daily
  rollups.
- Authenticated cluster/node statistics API with summed additive counters,
  query-weighted processing time, response-weighted upstream latency, stable
  ranked-value merging, time series, freshness, and explicit coverage.
- Responsive `/statistics` experience using the global scope selector, three
  fixed ranges, summary cards, accessible activity chart, ranked panels, and
  node-attributed partial/empty/error states. The dashboard retains health
  focus and adds only a compact 24-hour summary.

### Changed

- Default controller, image, installer, and web version is 0.5.0.
- AdGuard Home v0.107.72 through v0.107.78 has the explicit
  `statistics_exact_range` capability. Older configuration-compatible nodes
  remain manageable but are excluded from exact historical aggregation.

### Deferred

- Custom statistics ranges and combined query-log ingestion remain out of
  scope; query-log ingestion is Release 0.6.

## 0.4.1 - 2026-08-02

### Added

- Distinct HA Controller task pages: infrastructure-focused Nodes,
  forward-looking Configuration Control, execution-focused Deployments,
  convergence-focused Drift, and immutable Change History.
- Shared structured semantic comparison presentation for snapshot, revision,
  and desired-versus-observed contexts, plus explicit advanced adoption
  ownership under Configuration Control.
- Release 0.4.1 Phase 10 exact route/redirect and Deployments/Drift focus
  regressions, dedicated redacted Encryption coverage, Axe WCAG structural
  checks, and light/dark visual baselines at 320, 768, 1199, 1200, and 1440
  pixels plus the active mobile drawer hierarchy.
- Release 0.4.1 Phase 9C-3 audited destructive Query Log clearing and Statistics reset with typed confirmation, narrow selected-node defaults, explicit compatible-fleet scope, durable per-node results, post-command observation, safe errors, idempotency, and unchanged desired policy/revisions.
- Release 0.4.1 Phase 9C-2 audited host-filtering tests from Custom Filter Rules with hostname, optional client/query type, explicit selected-node/fleet scope, version-aware capability exclusions, encrypted queued input, bounded node-attributed rule results, partial success, and idempotency without desired-state mutation.
- Release 0.4.1 Phase 9C-1 durable audited DNS operational commands: current-draft upstream tests, confirmed selected-node/fleet cache clearing, encrypted queued input, per-node/per-resolver results, idempotency, restart-safe non-replay, and post-clear observation.
- Release 0.4.1 Phase 8B node-scoped reset-leases and reset-DHCP-configuration operational commands with maintenance/deployment and configuration-reset reconciliation-policy guards, typed confirmation, durable per-node results, per-user idempotency, redacted requested/terminal audits, and immediate post-command observation.
- Authenticated CSRF-protected DHCP reset and durable-result controller routes plus migration `000005_release_0_4_1_dhcp_operations`; there is no fleet reset route and destructive commands never mutate desired state.
- Release 0.4.1 Phase 7 DNS Rewrites searchable table and add/edit dialogs with contract-bounded domain/answer validation, inferred A/AAAA/CNAME/passthrough presentation, duplicate prevention, confirmed draft-only deletion, capability-aware enablement, and draft/revision/convergence context.
- Release 0.4.1 Phase 5B dedicated DNS Allowlists page, shared Blocklist/Allowlist table and dialog composition, observed-only rule-count/freshness and per-node application metadata, safer removal confirmation, and audited refresh-all partial results using allowlist semantics.
- Authenticated `GET /api/v1/clusters/{clusterId}/allowlists/presentation` endpoint with category-separated stale caching and safe node-attributed metadata.
- Release 0.4.1 Phase 4 Blocked Services page with a controller-mediated, version-aware per-node catalogue; searchable grouped service selection; compatibility warnings; selected counts and group actions; shared schedule editing; and publication/deployment preflight for unsupported IDs.
- Authenticated `GET /api/v1/clusters/{clusterId}/blocked-services/catalogue` observed-metadata endpoint with bounded per-version caching, stale/partial node results, and safe metadata redaction.
- Canonical configuration schema v2 covering broader DNS behavior, blocklists and allowlists, persistent clients, DNS rewrites, blocked-service schedules, safety services, Safe Search, query-log policy, statistics policy, redacted TLS inventory, and node-specific DHCP configuration/static leases.
- Version-aware schema projection so AdGuard Home v0.107.52 and immutable schema-v1 revisions remain observable and deployable while schema v2 supports the current v0.107.53–v0.107.78 contract.
- Patch-level capability handling for upstream timeout, cache enablement, rewrite enablement, ignored-list activation, and v0.107.78 filter intervals; newer unreviewed contracts are reported as unknown.
- Capability-gated, sequential deployment of schema-v2 settings with managed-field read-back verification and safe single-active-node DHCP handoff ordering.
- Audited per-node blocklist and allowlist refresh API and partial-result UI workflow.
- AdGuard Home-style nested settings navigation and responsive forms for DNS, filters, clients, rewrites, services and safety, log/statistics policy, TLS inventory, and DHCP.
- Migration `000004_release_0_4` and ADR-0025.

### Changed

- Record Release 0.4 functional, Docker installation, and native/systemd
  installation validation as completed by the operator on 3 August 2026.
- Move revision history/comparison/rollback out of Configuration Control into
  Change History, and separate deployment execution from continuing drift
  state without changing backend contracts or lifecycle semantics.
- Complete Phase 10 route, accessibility, visual, packaging, cleanup, and documentation hardening; preserve all compatibility redirects and desired-state safety boundaries.
- Replace the superseded broad settings component with a dedicated redacted Encryption inventory page and move all remaining presentation colours to semantic tokens.
- Default controller, image, installer, and web version is 0.4.1.
- Replace the Release 0.4 inline DNS rewrite editor at `/filters/rewrites` while preserving the schema-v2 desired-state representation, controller-only browser boundary, and separate Save Draft, Publish, and Deploy lifecycle.
- Move Safe Browsing, parental control, and Safe Search presentation to Settings > General; `/filters/blocked-services` now contains only the blocked-service catalogue and inactivity schedule while preserving the existing desired-state fields.
- Release 0.3, including Docker and systemd installation and functional validation, is recorded as complete.
- Use the primary application sidebar as the sole settings navigation and place larger DHCP node, client, and blocked-services schedule headings inside their corresponding configuration cards.

### Fixed

- Remove the remaining accidental same-page behavior for
  `/ha/configuration`/`/ha/history` and `/ha/deployments`/`/ha/drift` while
  preserving canonical routes, active navigation, redirects, and Not Found.
- Raise light-theme semantic foreground contrast to WCAG AA token pairs and
  remove sidebar-era/raw presentation colour aliases.
- Prevent the rewrites, persistent-clients, and DHCP static-lease editors from crashing on non-secure HTTP origins by using stable local row keys instead of the secure-context-only browser UUID API.
- Avoid redundant `/control/dhcp/set_config` writes when a node's schema-v2 DHCP configuration already matches the immutable revision. This prevents disabled, unconfigured DHCP nodes from rejecting an otherwise successful deployment after DNS settings have been applied, while static leases continue to reconcile.
- Include the safe AdGuard Home HTTP method, operation path, and status in `NODE_APPLY_FAILED` task details and show that existing per-node diagnostic on the Deployments page without retaining node response bodies.

### Deferred

- TLS certificate/key mutation remains excluded until controller-managed secret references exist.
- Central statistics ingestion and combined query-log ingestion remain Releases 0.5 and 0.6.
- Field-level drift ignore, parallel deployment, automatic partial recovery, and controller high availability retain their later roadmap positions.

### Removed

- Superseded broad `ManagedSettingsPage` DNS/filter editors and redundant
  placeholder files from populated frontend directories. Compatibility
  redirects remain intact.

## 0.3.2 - 2026-07-30

### Fixed

- Read node listener addresses and DNS port from AdGuard Home's `/control/status` contract during configuration inventory. The initial 0.3.0 adapter incorrectly expected those fields from `/control/dns_info`, causing imported drafts to contain empty listener overrides.
- Reject incomplete legacy snapshots at import and show node names with refresh/re-import guidance instead of opaque `nodeOverrides.<uuid>` validation messages.
- Treat an unset cluster `active_revision_id` as `false` when loading revisions. PostgreSQL comparison with `NULL` previously caused an internal error while a published revision had not yet been activated and failed the Release 0.3 integration workflow before its first deployment.

## 0.3.0 - 2026-07-30

### Added

- Separate authoritative schema-v1 desired documents with shared DNS/filtering policy and per-node listener overrides.
- Optimistic draft editing/validation, immutable numbered revisions, semantic revision comparison, and deployment-based rollback.
- Durable sequential deployments with all-target preflight, per-node tasks, safe cancellation, restart interruption, read-back verification, active/applied revision state, and complete audit history.
- Supported AdGuard Home DNS/filtering writer with whitelist preservation and safe error mapping.
- Deduplicated drift events, Manual/Alert/Enforce reconciliation, automatic verified restoration, maintenance mode, and convergence state.
- Versioned revision/deployment/drift APIs plus React draft, history, deployment timeline, policy, and drift-action surfaces.
- Migration `000003_release_0_3`, ADR-0024, unit/contract coverage, and the two-node Release 0.3 integration workflow.

### Changed

- Default controller, image, installer, and web version is 0.3.0.
- Releases 0.2, 0.2.1, and 0.2.2 are recorded as operator-validated.

### Deferred

- Listener writes, whitelist authoring, wider AdGuard settings, field-level ignore rules, parallel/rolling strategies, scheduled maintenance windows, and richer partial-deployment recovery.

## 0.2.2 - 2026-07-29

### Fixed

- Restart and verify the systemd service after replacing release artifacts. Previous upgrade reruns used `systemctl enable --now`, which left an already-running older API process serving a newly installed frontend.

## 0.2.1 - 2026-07-29

### Fixed

- Prevent the Configuration page from crashing when a cluster has no imported draft. The API now omits an absent draft, while the UI remains compatible with the `draft: null` response emitted by 0.2.0.

## 0.2.0 - 2026-07-29

### Added

- Canonical configuration schema v1 for read-only DNS and filtering inventory.
- Version-aware capability profiles and immutable successful/failed node snapshots.
- Ownership-aware semantic diff API and configuration comparison UI.
- Explicit, audited import into an optimistic non-authoritative cluster draft.
- AdGuard Home v0.107.52 and v0.107.61 compatibility fixtures.
- Migration `000002_release_0_2` and ADR-0023.

### Changed

- Default controller and web version is 0.2.0.
- Production Make builds no longer require ripgrep, fixing the systemd installer warning.
- Roadmap records operator validation of Releases 0.1 and 0.1.1.

## 0.1.1 - 2026-07-28

### Added

- Production Dockerfile and Docker Compose installation with PostgreSQL persistence, health checks, non-root/read-only controller runtime, and source builds.
- Git-checkout Debian/systemd installer that builds the application, provisions PostgreSQL and the service account, generates protected secrets, and preserves existing runtime configuration on upgrade.
- Explicit regression coverage that the one-time initial administrator setup cannot be repeated.

### Changed

- Default build and runtime version is 0.1.1.
- Installation, architecture, operations, security, product, and roadmap documentation now describe the supported Docker and systemd paths.
- CI uses its PostgreSQL service directly.

### Removed

- Disposable local Compose/node-simulator environment and its Make/script/documentation surface.

### Added

- Release 0.1 PostgreSQL schema and checksum-protected migration runner.
- First-run local administrator, Argon2id authentication, secure sessions, CSRF protection, and login throttling.
- Audited cluster and node management APIs with optimistic concurrency.
- AES-256-GCM encrypted node credentials and explicit TLS trust policies.
- Read-only AdGuard Home health/version adapter and automatic polling.
- React dark-mode setup, login, dashboard, node-management, and audit surfaces.
- Health/readiness endpoints, request IDs, stable API errors, build tooling, tests, and CI.
- ADR-0021 and Release 0.1 feature ledger.

### Changed

- Reconciled architecture, API, database, security, testing, operations, and roadmap documentation with the implemented Release 0.1 design.

### Earlier scaffold

- Initial repository scaffold.
- Architecture documentation.
- Roadmap and release plan.
- Frontend design specification.
- Database design specification.
- Security and operations documentation.
- AI and contributor instructions.

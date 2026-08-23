# v1.0.2 Security Audit

Date: 2026-08-20  
Scope: Atlas DNS Controller v1.0.2 security, dependency, toolchain, CI, and
release readiness workstream.

## Release recommendation

**PASS — no known release-blocking security findings remain.**

No authentication bypass, trusted-identity forgery, privilege escalation,
credential disclosure, deployment-authorization bypass, mutable published
revision path, executable injection, active bootstrap bypass, or unrestricted
outbound-request primitive was found. The reachable vulnerable Go modules and
unsupported Go toolchain found at the start of the review were upgraded. The
remaining risks below either require database/host administrator compromise or
are deliberate boundaries of a private-network DNS management product.

This audit did not add webhook notification history or Operational History
pagination. The existing webhook implementation was reviewed, but feature work
remains a separate change boundary.

## Scope and method

The review traced requests and data across API routing, authentication,
authorization, PostgreSQL repositories and migrations, credential encryption,
AdGuard Home adapters, revisions, deployments, drift, audit records, setup,
frontend rendering, backups, install paths, containers, CI, and release tools.
It also reviewed current architecture, decision, database, backend, frontend,
development, security, operations, roadmap, and product documentation, plus
recent repository history.

The browser, all request fields, managed-node responses, release metadata, and
configured outbound URLs were treated as untrusted. Tests use local fake HTTP
servers and the test PostgreSQL database; no real DNS node was changed.

OWASP ASVS, OWASP Top 10, and OWASP API Security Top 10 were used as coverage
prompts. The repository-specific paths and Atlas threat model remained the
primary evidence.

## Security architecture and trust boundaries

```text
Administrator browser
  -> optional trusted HTTPS reverse proxy
  -> same-origin React application and Go /api/v1 server
  -> service/repository authorization and validation
  -> PostgreSQL 17
  -> AES-256-GCM credential/destination encryption
  -> bounded HTTP(S) AdGuard Home and HTTPS webhook clients
  -> managed private-network nodes / webhook receivers
```

Background health, inventory, telemetry, query-log, reconciliation, deployment,
and HA workers use the same repositories and adapter boundaries. The controller
never proxies normal DNS requests; AdGuard Home nodes continue serving DNS when
Atlas is unavailable.

Atlas has no external identity provider, JWT, browser bearer token, tenant, or
low-privilege application role. An opaque random cookie identifies a server-side
session. All v1 accounts are administrators. PostgreSQL is a single system of
record, and node credentials are decrypted only at server-side adapter use.

### Crown-jewel assets

- administrator password hashes, active session records, and the session HMAC
  secret;
- the credential encryption key, encrypted node credentials, and encrypted
  webhook destinations;
- desired drafts, immutable published revisions, deployment authority and
  results, drift evidence, and live DNS configuration;
- PostgreSQL contents, query-log data, audit evidence, and encrypted portable
  backup archives and passphrases.

### Threat actors considered

- an unauthenticated network client;
- a malicious browser or administrator sending requests the UI does not expose;
- a future authenticated non-administrator (the database currently rejects such
  a role, but administrator-only handlers still enforce the role);
- a compromised AdGuard Home node returning malicious, malformed, or large
  content;
- an attacker influencing an administrator-configured node or webhook URL;
- an attacker obtaining only a database dump, a portable backup, logs, or host
  filesystem access.

## Authentication, sessions, and browser boundary

`internal/auth.Service` hashes local passwords with Argon2id (64 MiB, three
iterations, parallelism two). There are no default credentials. Setup obtains a
database lock and creates the first administrator only while no user exists, so
parallel or later setup attempts fail closed.

Session and CSRF values are independent 256-bit random values. Purpose-separated
HMAC-SHA-256 hashes are stored in PostgreSQL; raw session values exist only in
cookies. The session cookie is HTTP-only, Strict SameSite, path `/`, and Secure
when `PUBLIC_BASE_URL` is HTTPS. The readable CSRF cookie is also Strict
SameSite, and mutations require an equal header value whose session-bound hash
matches. Logout revokes the session. Password reset and account disable revoke
the affected user's sessions.

Login uses a constant-work dummy password hash for unknown identities and a
per-process source-IP/email limiter (five failures per 15 minutes). It uses the
TCP peer address and deliberately does not trust forwarded client-IP headers.

The HTTP route inventory contains seven intentionally public entries:
`/health`, `/ready`, token-protected `/metrics`, setup status/setup, login, and
the frontend fallback. All 87 other registered routes use `authenticated` or
`administrator` middleware. API misses return JSON 404 and never fall through
to the frontend. Protected mutations require CSRF at the backend boundary.

Request JSON is limited to 1 MiB, decoded into explicit input DTOs with unknown
fields rejected, and limited to exactly one JSON object. Actor identity is
derived from the authenticated request context, never from body fields.

## Authorization and object access

The current product model is one organisation and one supported role:
`administrator`. There is therefore no tenant-owner boundary for cluster, node,
revision, deployment, drift, or query-event IDs. Authentication authorizes
normal management functions; user administration, backup, update/settings,
notification-channel management, and destructive revision/deployment lifecycle
actions additionally check the server-side administrator role.

Nested object access is resolved server-side and constrained by its parent where
one exists: query-event detail includes cluster ID, nodes are checked against
clusters, revisions are checked against clusters, deployment targets come from
the cluster/revision state, and node-scoped telemetry rejects nodes outside the
cluster. Changing a browser-supplied ID does not establish ownership or actor
identity. If a non-administrator or multi-organisation role is introduced, this
matrix must be redesigned before that role is enabled; frontend visibility is
not an authorization control.

## PostgreSQL access and RLS determination

Native installation creates one non-superuser login/database-owner role.
Containers use the configured PostgreSQL application role. No migration grants
`SUPERUSER`, `CREATEDB`, `CREATEROLE`, or `BYPASSRLS`; no `SECURITY DEFINER`
function or RLS policy exists. `pg_trgm` is the only extension.

The application role owns the database/schema so it can run append-only
migrations. It can consequently modify every table if the application process
or DB credential is fully compromised. This is an accepted least-privilege
weakness, not a remote authorization control. A separate migration owner and
restricted runtime role is the preferred future hardening design.

RLS is not appropriate to the present schema: all accounts are administrators
in the same organisation and there is no row-ownership attribute to enforce.
Application services, transactions, constraints, and foreign keys enforce the
current access model. Adding RLS without a tenant/ownership model would not
protect against the table-owning runtime role.

| Table | Security-sensitive | Intended application access | RLS appropriate | DB privilege issue |
|---|---:|---|---:|---|
| `users` | Yes | Admin read/create/update; password hash never serialized | No | Runtime owner can write directly |
| `sessions` | Yes | Auth subsystem create/read/touch/revoke | No | Runtime owner can read/revoke |
| `clusters` | Yes | Authenticated admin CRUD | No | Runtime owner can write directly |
| `nodes` | Yes | Authenticated admin CRUD; ciphertext internal only | No | Runtime owner can read ciphertext/write state |
| `audit_events` | Yes | Services insert; authenticated read; no API update/delete | No | Runtime owner can alter evidence |
| `node_capability_profiles` | Yes | Worker/service replace and read | No | Runtime owner can write directly |
| `observed_snapshots` | Yes | Workers insert/read; external state stays observed | No | Runtime owner can write directly |
| `configuration_drafts` | Yes | Authenticated explicit import/update | No | Runtime owner can write directly |
| `configuration_revisions` | Yes | Publish/read; archive metadata only; unused delete guarded | No | Runtime owner can bypass immutability |
| `deployments` | Yes | Service create/status/archive; guarded unstarted delete | No | Runtime owner can write directly |
| `deployment_nodes` | Yes | Executor records per-node result | No | Runtime owner can write directly |
| `drift_events` | Yes | Workers create; explicit restore/adopt resolution | No | Runtime owner can write directly |
| `operational_commands` | Yes | Authenticated service create/read/status | No | Runtime owner can read encrypted payload/write state |
| `operational_command_node_results` | Yes | Executor per-node result write/read | No | Runtime owner can write directly |
| `statistics_poll_attempts` | Low | Worker write; operational read | No | Runtime owner can write directly |
| `statistics_snapshots` | Low | Worker write; authenticated aggregate read | No | Runtime owner can write directly |
| `statistics_buckets` | Low | Worker write; authenticated aggregate read | No | Runtime owner can write directly |
| `query_ingestion_checkpoints` | Yes | Worker state only | No | Runtime owner can write directly |
| `query_ingestion_attempts` | Yes | Worker state/diagnostics | No | Runtime owner can write directly |
| `query_events` | Yes | Worker insert/retention delete; authenticated bounded read | No | Runtime owner can read private DNS history |
| `node_lifecycle_settings` | Yes | Authenticated update with concurrency/audit | No | Runtime owner can write directly |
| `dns_probe_results` | Yes | Worker/service safe-result insert/read | No | Runtime owner can write directly |
| `ha_operational_events` | Yes | Service insert; authenticated history read | No | Runtime owner can alter evidence |
| `upgrade_operations` | Yes | Authenticated lifecycle service write/read | No | Runtime owner can write directly |
| `upstream_release_cache` | Low | Worker replace/read; rendered as untrusted text | No | Runtime owner can write directly |
| `notification_channels` | Yes | Admin CRUD; encrypted destination internal only | No | Runtime owner can read ciphertext/write state |
| `notification_deliveries` | Yes | Worker insert/status; authenticated operational use | No | Runtime owner can alter evidence |
| `controller_release_cache` | Low | Update checker replace/read | No | Runtime owner can write untrusted metadata |
| `system_settings` | Yes | Admin read/update with concurrency/audit | No | Runtime owner can write directly |

All reviewed queries use PostgreSQL parameters. Dynamic query-log filters and
sort/order fragments are controller-owned fixed clauses, and pagination/filter
values are validated and bounded. No attacker-controlled SQL identifier or
shell fragment was found.

## Credential, secret, backup, and logging findings

Node credentials use AES-256-GCM with a random nonce and node/resource-bound
additional authenticated data. The 32-byte key is supplied separately from
PostgreSQL. Node domain JSON omits credential ciphertext, nonce, key metadata,
custom CA data, and plaintext credentials. Updates preserve the hidden secret
unless both replacement username and password are deliberately supplied.
Webhook destinations use the same authenticated encryption boundary and expose
only a scheme/host summary.

A stolen PostgreSQL dump alone contains ciphertext and password hashes but not
the credential key, so it does not trivially reveal node credentials. A portable
Atlas backup deliberately contains both the dump and credential key inside an
authenticated age/scrypt passphrase envelope. Every backup is therefore a
crown-jewel secret. The CLI receives passphrases from protected regular files,
removes DB passwords from process arguments, uses private temporary files, and
writes a restored key with mode 0600.

Structured request logs record method, path, status, request ID, and duration,
not headers or bodies. Audits contain controlled action/resource metadata and do
not include credentials or raw query events. Adapter failures discard response
bodies and return bounded, controlled diagnostics. Health endpoints expose only
service readiness/version-level information; metrics require a sufficiently
strong bearer token when enabled.

## Node, webhook, and untrusted-response boundary

Node URLs support only absolute HTTP/HTTPS URLs without userinfo, query, or
fragment. Plain HTTP requires the explicit per-node `insecure_http` certificate
policy. HTTPS verifies system trust or a node-scoped custom CA with TLS 1.2
minimum; `InsecureSkipVerify` is never used. The status probe revalidates the
stored URL immediately before use.

Private, loopback, link-local, hostname, IPv4, IPv6, and administrator-selected
ports remain allowed because private node management is the product's purpose.
Node transports ignore proxy environment variables, reject all redirects, and
bound dialing, TLS, response headers, total request time, idle connections, and
response bodies. The status and configuration adapters cap normal JSON bodies
at 1 MiB and validate typed/semantic contracts. Compromised-node values reach
React as text; the frontend contains no `dangerouslySetInnerHTML` or direct
`innerHTML` sink.

DNS rebinding is not pinned to a first resolution and there is no destination
CIDR/port allowlist. Because private destinations are legitimate and node URLs
are administrator-controlled, blocking private ranges would break the product.
Operators should isolate node administration interfaces on a trusted management
network. A future optional outbound allowlist is a hardening opportunity.

Webhook destinations require HTTPS, reject userinfo/fragments and redirects,
verify TLS, impose a ten-second timeout, and never return destination or response
body data. Configuration/test/delete requires administrator identity and CSRF.
The default transport may honor an operator-configured HTTPS proxy. Arbitrary
HTTPS webhook hosts are a deliberate administrator capability, so trusted
network egress policy remains important.

## Revision, deployment, drift, and audit integrity

Publication copies a validated draft into a separate revision record. No API or
repository method edits the published document. Archive changes only lifecycle
metadata; hard deletion is limited to unused revisions after transactional
reference checks. Direct database ownership is outside this application
immutability guarantee and is documented above.

Deployment requests identify a cluster and revision; they do not resubmit a
configuration document or arbitrary target list. The service loads and checks
the server-side revision, records actor/revision/targets, and the executor reloads
the revision and validates every node and observation before the first mutation.
Nodes are changed sequentially, read back, and the revision becomes active only
after full verified success. Per-node results persist. Repeated operations use
durable deployment/operation records and explicit state; privileged destructive
lifecycle actions require exact confirmation and audit.

Observed configuration is distinct from desired configuration. Drift stays an
explicit durable event. Restore deploys desired state; adopt changes the mutable
draft for later review/publication. A node cannot silently make observed state
the authoritative revision.

## HTTP, frontend, runtime, and installation findings

The Go server uses read-header, read, write, and idle timeouts. API responses are
`no-store` and include CSP, frame denial, MIME-sniff prevention, no-referrer, and
restricted browser-permission headers. CORS is not enabled; browser use is
same-origin. Atlas does not emit HSTS because it may receive HTTP behind a
trusted TLS terminator; the production proxy owns HSTS and public HTTPS policy.

React's normal escaping protects node names, query-log content, descriptions,
errors, and release text. Release notes render as text, and external release
links are server-validated and use safe link attributes. No unsafe HTML sink or
client-derived authorization identity was found.

The production container runs as UID 10001 with a read-only root filesystem,
`no-new-privileges`, bounded tmpfs, and no Docker socket, host network, or
privileged mode. The native systemd service uses a dedicated non-login user,
`NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=strict`, and `ProtectHome`.
Installer-generated secrets use cryptographic randomness and protected files.
Backup commands use direct argument arrays rather than a shell. Shell installer
inputs used in SQL/paths are constrained before use.

## Dependency and toolchain inventory

Versions and maintenance status were checked against upstream Go, Node.js,
PostgreSQL, npm, package, and official GitHub Action sources on the audit date.
Only security fixes and compatible in-range frontend updates were applied;
unrelated frontend major upgrades were deliberately excluded.

Authoritative references: [Go downloads](https://go.dev/dl/?mode=json),
[Go support policy](https://go.dev/doc/devel/release), [Go 1.25 notes](https://go.dev/doc/go1.25),
[Go 1.26 notes](https://go.dev/doc/go1.26), [Go 1.27 notes](https://go.dev/doc/go1.27),
[Node release status](https://nodejs.org/en/about/previous-releases), and
[PostgreSQL versioning](https://www.postgresql.org/support/versioning/).
GitHub Action release tags were resolved against each action's official Git
repository and the resulting commit, rather than a floating major tag, is stored
in the workflows.

| Component | Before / current | Supported/current upstream | Issue and reachability | Action | Upgrade risk |
|---|---|---|---|---|---|
| Go language/toolchain | 1.24.0 / 1.27.0 | 1.27.0 stable; Go supports the latest two major releases | 1.24 unsupported and no longer receives fixes | Updated module, CI, Docker, native-release baseline, and docs | Low; full tests/builds required |
| `filippo.io/age` | 1.3.1 / 1.3.1 | 1.3.1 | No finding; runtime backup encryption | Kept | None |
| `pgx/v5` | 5.7.5 / 5.9.2 | 5.10 available; 5.9.2 fixes audited advisories | GO-2026-5004 reached `QueryRow`, but Atlas SQL was constant/parameterized; two imported-package advisories also present | Updated to minimum fixed 5.9.2 | Low |
| `x/crypto` | 0.45.0 / 0.52.0 | Newer compatible releases available | Prior SSH advisories were in unused packages; GO-2026-5932 flags the unmaintained `openpgp` package with no fixed version. Atlas imports only Argon2, not SSH/OpenPGP | Updated through the fixed SSH line; accepted unreachable module-only advisory | Low |
| `x/text` (transitive) | 0.31.0 / 0.39.0 | Newer compatible releases available | GO-2026-5970 reachable through pgx normalization; malformed input could loop | Updated to fixed 0.39.0 | Low |
| `x/sync`, `x/sys` (transitive) | 0.18/0.38 / 0.21/0.45 | Compatible current dependency selections | No reachable finding | Updated by `go mod tidy` | Low |
| Node.js container/CI | 22 / 22 | 22 Maintenance LTS; 24 Active LTS | Supported through April 2027; no audit finding | Kept supported LTS to avoid an unrelated runtime major | Low/none |
| npm | Bundled with Node 22 | Current Node-22 bundle | Production audit: zero vulnerabilities | Kept with Node image/setup | None |
| React/React DOM | 19.2.8 | 19.2.8 in resolved lock | Runtime; zero npm audit findings | Lock retained/refreshed | Low |
| Biome, user-event, React types, axe | 2.5.5/14.6.1/19.2.x/4.10.3 | 2.5.9/14.6.5/19.2.x/4.13.0 | Build/test-only compatible updates | Updated within declared ranges | Low |
| Vite / Vitest / TypeScript / jsdom / React plugin | 6.4.3 / 3.2.7 / 5.9.3 / 26.1.0 / 4.7.0 | New major releases available | Build/test-only; zero audit findings; jsdom retains deprecated transitive `whatwg-encoding`; majors carry compatibility cost | Kept; review in separate maintenance work | Avoided medium breaking-change risk |
| PostgreSQL | 17-bookworm | PostgreSQL 17 supported to November 2029 | No version finding; image pulls current 17.x patch | Kept consistent in runtime/release; CI uses major 17 | Low |
| Debian images | bookworm slim/runtime | Supported Debian stable line for selected images | No scanner finding available locally | Kept; floating image rebuilds must be scanned in release CI/registry | Low |
| GitHub Actions | checkout 4, setup-go 5, setup-node 4, upload-artifact 4, Docker actions 3/6 | checkout 7.0.1, setup-go 7.0.0, setup-node 7.0.0, upload-artifact 7.0.1, Docker actions 4.2/4.3/4.6/7.3 | Old embedded Node runtimes/deprecation exposure | Updated every action to official immutable commit SHA | Low |
| Native/systemd release tooling | Host shell plus CI Go | Go 1.27 CI cross-builds | No downloaded build script or shell-injection path found | Kept; Go baseline updated | Low |

The Go 1.25–1.27 release notes were reviewed for Atlas-relevant changes. Go
1.25 makes `GOMAXPROCS` container/cgroup-aware and rejects SHA-1 signatures in
TLS 1.2 by default; the latter may deliberately reject an obsolete managed-node
certificate. Go 1.26 enables the new garbage collector and heap-base
randomization. Go 1.27 adds the `stdversion` test check, backs the v1 JSON API
with its new implementation while preserving API behavior (exact error text may
differ), and may produce different but valid gzip bytes. Atlas sets no legacy
`GODEBUG`/`GOEXPERIMENT` escape hatch. Authentication, TLS/custom-CA, JSON,
backup-envelope, integration, race, and cross-architecture release tests passed
under Go 1.27.

CI now defaults to `contents: read`, receives no privileged secrets on the PR
workflow, installs deterministically with `npm ci`, runs `npm audit --omit=dev`
and pinned `govulncheck@v1.7.0`, and keeps every third-party action on an
immutable commit. The release job alone receives `contents: write` and
`packages: write`, only on tag/manual release triggers. It builds before login,
logs in only when publishing, refuses an existing release/image identity, and
publishes provenance and SBOM metadata.

## Findings and remediation

| Severity | Type | Finding / precondition / impact | Status and evidence |
|---|---|---|---|
| Medium | Security weakness | Go 1.24 was unsupported across module, CI, Docker, and developer docs, leaving future standard-library fixes unavailable. | Resolved with consistent Go 1.27 baseline and full Go validation. |
| Medium | Dependency vulnerability | `pgx/v5` 5.7.5 matched GO-2026-5004 and reached a query symbol. Exploitation requires attacker-controlled SQL construction; Atlas uses fixed parameterized SQL, so no exploit path was demonstrated. | Resolved at 5.9.2; `govulncheck ./...` reports zero symbol/package vulnerabilities. |
| Medium | Dependency vulnerability | `x/text` 0.31.0 matched GO-2026-5970; its normalization path was reachable through pgx and malformed input could cause an infinite loop. Relevant connection strings are operator-controlled. | Resolved at 0.39.0; `govulncheck ./...` reports zero symbol/package vulnerabilities. |
| Low | Dependency exposure | `x/crypto` 0.45.0 carried advisories in unused SSH packages. | Resolved at 0.52.0 as compatible defense in depth. |
| Informational | Not applicable | GO-2026-5932 reports that `x/crypto/openpgp` is unmaintained and has no fixed version. Atlas neither imports nor calls `openpgp`; `x/crypto` is required for Argon2id. | Accepted unreachable module-only advisory; verbose `govulncheck` proves zero imported-package/symbol exposure. |
| Low | Supply-chain weakness | Maintained-but-old action pins used deprecated embedded Node generations and CI inherited repository-default token permissions. | Resolved with current SHA pins and explicit read-only CI permission. |
| Low | Least-privilege weakness | Runtime application role owns the database to run migrations and could alter audit/revision rows after full application/DB compromise. | Accepted for v1.0.2; split migrator/runtime roles is future architecture work. |
| Low | Accepted design risk | Admin-selected node and webhook hosts can reach private services; node DNS answers are not pinned and no port/CIDR allowlist exists. | Accepted because private node access is core functionality; schemes, TLS, redirects, proxies, time, and size are constrained. |
| Low | Abuse-resistance weakness | Login throttling is process-local and source IP is the direct peer, so restart/HA can reset counts and a reverse proxy can aggregate clients. | Accepted for the current single-controller product; edge rate limiting is recommended. |
| Informational | Proxy boundary | HSTS and public TLS termination are not enforced by Atlas itself. | By design; configure HTTPS/HSTS at the trusted reverse proxy and set `PUBLIC_BASE_URL` correctly. |

## Security regression evidence

- `TestProtectedRouteInventoryRequiresAuthentication` derives all 87 protected
  routes from the real router and sends direct unauthenticated HTTP requests.
- `TestSessionBoundaryRejectsMissingTamperedAndExpiredTokens` covers server-side
  opaque-session failures; JWT-specific tests are not applicable.
- `TestMutationBoundaryRejectsCSRFAndMassAssignment` proves missing CSRF is 403
  and privileged body fields cannot establish identity/role.
- Existing administrator-route tests prove an authenticated non-administrator is
  rejected with 403 even though the supported database role set is admin-only.
- `TestNormaliseNodeURL`, `TestProbeRejectsRedirects`, and
  `TestProbeRejectsOversizedStatusResponse` cover malicious schemes, userinfo,
  private IPv4/IPv6 policy, redirect refusal, and response bounds.
- Integration `TestRelease01OperatorWorkflow` proves setup closes, node GET/audit
  omit credentials, ciphertext lacks plaintext, and server-side cookies/API
  behavior works through PostgreSQL.
- Authentication, credential cipher, backup envelope, webhook redaction,
  revision lifecycle, target validation, sequential deployment/read-back,
  drift, and safe audit properties remain covered by their package and release
  integration suites.

## Validation performed

- Checksum-verified official Go 1.27.0: `go mod verify`, `go vet ./...`,
  `go test ./...`, and `go test -race ./...` passed.
- `govulncheck@v1.7.0 ./...` and verbose triage reported zero affected symbols
  and zero imported packages; the single module-only OpenPGP advisory is
  documented above.
- A disposable local PostgreSQL 17 cluster ran
  `go test -count=1 ./tests/integration`; all integration tests passed and the
  database was stopped afterward.
- Checksum-verified Node 22.23.2 with npm 10.9.8 ran a clean temporary `npm ci`,
  type-check, lint, 251 frontend tests, production build, full `npm audit`, and
  production-only audit. Both audits reported zero vulnerabilities.
- `npm outdated` identified only the deliberately deferred build/test major
  releases in the dependency table. Clean install also reported deprecated
  transitive `whatwg-encoding` through test-only jsdom.
- The release-artifact script passed under Go 1.27/Node 22 and produced checksums
  for Linux amd64 and arm64 controller, migration, and backup archives.
- Documentation-link validation, Go/frontend formatting, workflow/Compose YAML
  parsing, `git diff --check`, active Go-1.24 reference search, unsafe HTML/TLS
  pattern search, and final changed-file secret-pattern review passed.

Docker is not installed in the audit environment, so `docker compose config`,
the production container build, image execution, SBOM generation, and an image
vulnerability scan were not executed locally. `syft`, `trivy`, `grype`,
`gitleaks`, `actionlint`, and `shellcheck` were also unavailable. Compose/workflow
YAML was parsed and manually reviewed, native multi-architecture release inputs
were built, and the hosted workflows retain Compose validation plus the
multi-platform BuildKit build with provenance/SBOM. A hosted image scan remains
a manual release check.

## Remaining risks and hardening opportunities

Accepted for the supported self-hosted model:

- all local accounts are administrators; no fine-grained RBAC, OIDC, tenant
  isolation, or RLS exists;
- the controller and database are single-instance services, and login throttling
  is not distributed;
- administrator-selected private destinations and DNS rebinding remain possible
  within the intended management-network capability;
- a portable backup is sufficient to recover credentials once its passphrase is
  known, and the runtime DB owner can change evidence after host/DB compromise;
- floating major image tags obtain security patches at rebuild time but require
  registry/container scanning outside this local environment.

Recommended future hardening:

- separate schema-migration ownership from a restricted runtime database role;
- offer optional outbound CIDR/port allowlists and resolution logging/pinning for
  environments with tighter egress policy;
- add reverse-proxy/distributed login throttling and formally define trusted
  proxy handling before controller HA;
- add signed release artifacts and verify container/base-image scan policy in the
  hosted release environment;
- move Node 22 to Node 24 LTS in a dedicated build-toolchain change before Node
  22 reaches end of life.

No material applicable OWASP area was omitted from this review. File upload is
limited to authenticated restore preflight with a bounded archive reader; JWT,
OAuth/OIDC, multi-tenant ownership, GraphQL, WebSocket, and server-side HTML
templating controls are not applicable to the current implementation.

## Release conclusion

Atlas DNS Controller is sufficiently secure for the documented self-hosted,
trusted-administrator deployment model, provided operators treat the controller,
PostgreSQL, runtime secrets, management network, and backups as privileged DNS
infrastructure. The reusable release gate is documented in
[security-release-gate.md](security-release-gate.md).

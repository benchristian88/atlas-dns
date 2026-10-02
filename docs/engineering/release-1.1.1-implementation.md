# Atlas DNS Controller 1.1.1 implementation and validation

Recorded 3 October 2026 (Pacific/Auckland). This records repository evidence,
not publication or real-node/container release qualification.

## Root causes and resulting behavior

The adapter and Query Log duplicated numeric version parsing and restricted
compatibility to product minor `0.107`. Splitting on dots rejected prereleases
and conflated product version with API generation. Listener validation treated
all status addresses as IPs, and document conversion copied encrypted endpoint
URIs into the IP-only node override.

`internal/adguardcompat` now owns the policy and avoids the existing
AdGuard → Query Log import cycle. `APIGeneration` is `unknown` or
`legacy_control`; there is no new adapter or persisted/API field.

- Stable v0.107.78 and later compatible 0.x and all valid v1.x are eligible for
  the existing `/control/*` adapter. There is no upper minor/patch limit.
- Versions below the stable minimum are unsupported, including its prereleases.
  Future majors (2.x+) and malformed versions remain unknown.
- An anchored SemVer grammar accepts optional `v`, prerelease and build
  identifiers and rejects incomplete versions, extra components and leading
  zeros where forbidden. Canonical decimal string comparisons avoid overflow.
  The minimum stable prerelease distinction is explicit; suffixes are not
  discarded before applying that boundary.
- Existing release-tested legacy patches keep their classification; other
  eligible versions receive a general provisional-compatibility warning.
- Query Log wrappers delegate to the shared policy. Exact Statistics inherit
  the same eligibility; statistics jobs distinguish known old unsupported
  versions from unknown versions instead of checking the `0.107` minor.
- Listener validation recognizes parsed HTTPS/TLS/QUIC URI metadata with valid
  authority/optional port structure, accepts bare IPv4/IPv6, and rejects garbage,
  unknown schemes, invalid ports, empty lists and missing plain identity.
  Existing protection checks and domain errors remain.
- Only trimmed canonical IPs enter `NodeSpecific.BindHosts`. Extraction preserves
  source order; the existing canonicalizer still sorts/deduplicates. URI
  metadata is never fetched or copied into bind hosts. TLS remains inventory-only.

There are no new migrations, canonical/public API schema changes, live DNS
proxying, or changes to credential/TLS/redirect/timeout/size protections.
Development metadata follows the existing process (`1.1.1-dev`); release builds
inject `1.1.1`. The environment/install examples pin `1.1.1`.

## Regression evidence

Tests cover IPv4/IPv6, each/all encrypted protocols, whitespace/case, IPv6 URI
hosts, URI-only/empty/garbage/unknown scheme responses, malformed authority and
ports, protection failures, helper canonicalization/input preservation, and
configuration/desired-document validation. The real probe → reader → inventory
observation → snapshot → audited draft import flow passes for legacy v0.107.79,
v1 beta and stable v1, with HTTP/storage boundaries faked.

Version tests cover the minimum, later 0.x betas/stable, v1 alpha/beta/RC/stable,
later minors/patches including large numeric components, invalid syntax and
future majors. Query Log, Statistics and onboarding jobs accept eligible v1.
V1 response failures still reject authentication errors, HTTP errors, malformed
JSON, wrong types, missing semantics and oversized payloads.

Thirty-two representative v1 beta/stable JSON fixtures cover the nine requested
core endpoints plus rewrite settings, safety, DHCP, Query Log and Statistics
data. They are synthetic fixtures checked against the official
[tagged v1 beta contract](https://github.com/AdguardTeam/AdGuardHome/blob/v1.0.0-b.1/openapi/openapi.yaml),
not captured stable-v1/real-device qualification.

## Exact verification commands and results

Environment: Go 1.27.0, Node.js 24.19.0, PostgreSQL 17.11 (Homebrew), macOS arm64.
Go commands used `GOCACHE=/tmp/atlas-dns-go-cache` to stay within writable
filesystem roots; commands below retain that prefix where applicable.

| Command | Result |
|---|---|
| `GOCACHE=/tmp/atlas-dns-go-cache go test ./internal/adguardcompat ./internal/adguard ./internal/querylog ./internal/jobs ./internal/onboarding` | PASS after fixture-server path correction |
| `make fmt-check` | PASS; Go formatting and 126 frontend files |
| `make docs-check` | PASS; local Markdown links and anchors |
| `GOCACHE=/tmp/atlas-dns-go-cache make test` | PASS; `go test ./...` plus 54 frontend test files / 310 tests |
| `GOCACHE=/tmp/atlas-dns-go-cache go test ./...` | PASS after final parser/failure-regression changes; PostgreSQL cases skip without database environment |
| `GOCACHE=/tmp/atlas-dns-go-cache make lint` | PASS; `go vet ./...` plus frontend Biome |
| `GOCACHE=/tmp/atlas-dns-go-cache go vet ./...` | PASS after final parser changes |
| `GOCACHE=/tmp/atlas-dns-go-cache make test-race` | PASS |
| `GOCACHE=/tmp/atlas-dns-go-cache go test -race ./...` | PASS after final changes; PostgreSQL cases skip without database environment |
| `GOCACHE=/tmp/atlas-dns-go-cache TEST_DATABASE_URL='postgres://atlas_test@127.0.0.1:56111/atlas_dns_test?sslmode=disable' make test-integration` | PASS against a disposable local cluster, including the final rerun; `go test -count=1 ./tests/integration` |
| `npm --prefix web run typecheck` | PASS |
| `npm --prefix web run test:assets` | PASS |
| `GOCACHE=/tmp/atlas-dns-go-cache make build` | PASS; four binaries plus frontend |
| `GOCACHE=/tmp/atlas-dns-go-cache make VERSION=1.1.1 build` | PASS after final changes; stable version injected |
| `GOCACHE=/tmp/atlas-dns-go-cache WANT_BUILD_VERSION=1.1.1 go test -ldflags '-X github.com/benchristian88/atlas-dns/internal/version.Version=1.1.1' ./internal/version` | PASS |
| `GOBIN=/tmp/atlas-dns-111-tools GOCACHE=/tmp/atlas-dns-go-cache go install golang.org/x/vuln/cmd/govulncheck@v1.7.0` | PASS; repository-pinned checker installed outside the repository |
| `GOCACHE=/tmp/atlas-dns-go-cache /tmp/atlas-dns-111-tools/govulncheck ./...` | PASS; no reachable/imported-package vulnerabilities; four advisories in required modules not called by this code |
| `npm --prefix web audit --omit=dev` | PASS; zero production dependency vulnerabilities |
| `bash -n scripts/*.sh` | PASS |
| `git diff --check` | PASS |

Initial sandbox attempts could not write Go's default cache, bind HTTP fixture
listeners, or allocate PostgreSQL shared memory. A temporary cache and approved
local test execution resolved these environment restrictions. One new fixture
path mapping failed and was corrected. The first injected-version test command
omitted `WANT_BUILD_VERSION` and failed its expected development-version check;
the documented command above passed with the intended expectation. These were
not counted as passing attempts. The disposable PostgreSQL server was stopped
after the final integration pass.

## Remaining release gates and scope differences

Docker is unavailable (`docker --version` returned command not found), so Compose
configuration/build and container installation were not validated. Real AdGuard
nodes, Debian/systemd installs, container/GHCR publication and external browser
qualification remain manual release gates. The frontend build emits its existing
chunk-size warning; this patch does not change frontend behavior or bundling.
Uncalled module advisories remain follow-up dependency review, not a new
compatibility feature.

There are no intended behavior deviations from the request. The neutral shared
package, stable-minimum prerelease rejection, informational URI handling and
existing canonical sorting follow its allowed architecture/semantics. No
AdGuard `/api/v1` migration is attempted; a future adapter needs explicit
endpoint/schema/capability review and independent regression qualification.

## Changed files

- `.env.example`
- `CHANGELOG.md`
- `Dockerfile`
- `Makefile`
- `README.md`
- `compose.dev.yaml`
- `docs/README.md`
- `docs/api/controller-api.md`
- `docs/api/node-api.md`
- `docs/architecture/architecture.md`
- `docs/architecture/configuration-model.md`
- `docs/backend/operational-health.md`
- `docs/backend/query-ingestion.md`
- `docs/backend/statistics-aggregation.md`
- `docs/decisions/ADR-0025-version-configuration-schema-and-guard-dhcp-handoffs.md`
- `docs/decisions/ADR-0028-aggregate-exact-node-statistics-in-the-controller.md`
- `docs/development/release-process.md`
- `docs/getting-started/docker.md`
- `docs/getting-started/native-systemd.md`
- `docs/getting-started/onboarding.md`
- `docs/getting-started/portainer.md`
- `docs/operations/compatibility-matrix.md`
- `docs/operations/release-1.1.1.md`
- `docs/product/support-and-deprecation-policy.md`
- `internal/adguard/client.go`
- `internal/adguard/client_test.go`
- `internal/adguard/compatibility_flow_test.go`
- `internal/adguard/configuration.go`
- `internal/adguard/listeners_test.go`
- `internal/adguard/querylog_test.go`
- `internal/adguard/statistics_test.go`
- `internal/adguard/testdata/README.md`
- `internal/adguardcompat/compatibility.go`
- `internal/adguardcompat/compatibility_test.go`
- `internal/jobs/querylog_test.go`
- `internal/jobs/statistics.go`
- `internal/jobs/statistics_test.go`
- `internal/onboarding/service_test.go`
- `internal/querylog/models.go`
- `internal/querylog/models_test.go`
- `internal/version/version.go`
- `internal/version/version_test.go`
- `web/package-lock.json`
- `web/package.json`
- `internal/adguard/testdata/v1.0.0-b.1/*.json` (16 endpoint/data fixtures)
- `internal/adguard/testdata/v1.0.0/*.json` (16 endpoint/data fixtures)
- `docs/engineering/release-1.1.1-implementation.md` (this report)

# Atlas DNS Controller 1.1.1 implementation and validation

Recorded 3 October 2026 (Pacific/Auckland). This records repository evidence,
not publication or real-node/container release qualification.

The initial compatibility checks below used Go 1.27.0. The subsequent
[Go 1.27.1 maintenance validation](#go-1271-toolchain-update) records the updated
compiler and rerun gates separately.

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

## Go 1.27.1 toolchain update

The follow-up maintenance/security/runtime change keeps `go 1.27.0` as the
module minimum and adds `toolchain go1.27.1`, following the official
[toolchain conventions](https://go.dev/doc/toolchain). This is a patch compiler,
standard-library and runtime update, with no new language features. Both CI
workflows use exact `go-version: 1.27.1`, and the Docker builder pins
`golang:1.27.1-bookworm`. Makefile, Compose and release scripts have no separate
Go version pin; they inherit the module-aware command/builder. No development
container or other toolchain override was found. Historical audit evidence and
AdGuard product versions such as `v1.27.12` were intentionally left unchanged.

Changed files for this follow-up: `go.mod`, `Dockerfile`, both `.github/workflows`
files, `CHANGELOG.md`, local development/release-process docs, v1.1.1 release
notes and this report. CI now also builds the production Dockerfile, starts its
image against the disposable PostgreSQL service, waits at most 30 attempts for
readiness/health, and removes the test container on exit. Static workflow
validation passed; actual execution remains pending CI.

`go version` returned `go1.27.1 darwin/arm64`. `go mod tidy` produced no changes
relative to the already-added toolchain directive, and `go.sum` is byte-for-byte
unchanged. The only module diff is the two-line toolchain declaration; no
required dependency version or checksum changed. `go mod verify` returned
`all modules verified`.

All Go checks below used `GOCACHE=/tmp/atlas-dns-go-cache`. Formatting additionally
used the selected 1.27.1 toolchain's `bin` directory at the front of `PATH`, so
`make fmt-check` ran its patched `gofmt` as well as the frontend formatter.

| Command/check | Result under Go 1.27.1 |
|---|---|
| `go version` | PASS; `go1.27.1 darwin/arm64` |
| `go mod tidy` | PASS; no additional module/checksum changes |
| `go mod verify` | PASS; all modules verified |
| `go test ./...` | PASS; database cases skip without explicit DB environment |
| `go vet ./...` | PASS |
| `make test` | PASS; full Go suite and 54 frontend files / 310 tests |
| `make test-race` | PASS; Go race suite |
| `TEST_DATABASE_URL='postgres://atlas_test@127.0.0.1:56111/atlas_dns_test?sslmode=disable' make test-integration` | PASS; actual PostgreSQL 17.11 suite |
| `make lint` | PASS; Go vet and frontend Biome |
| `make fmt-check` | PASS |
| `make docs-check` | PASS |
| `npm --prefix web run typecheck` | PASS |
| `npm --prefix web run test:assets` | PASS |
| `make VERSION=1.1.1 build` | PASS; four native commands and frontend |
| `go version -m bin/atlas-dns` | PASS; compiler/runtime metadata says `go1.27.1` |
| Native production startup against fresh `atlas_dns_go1271_startup` database | PASS; fresh migrations, `/ready` 200, `/health` reports `status: ok` / `version: 1.1.1`, frontend 200; process stopped afterwards |
| `ATLAS_DNS_VERSION=1.1.1-rc.go1271 scripts/release-artifacts.sh` | PASS; candidate Linux amd64 and arm64 archives; sandbox module stat-cache warnings did not prevent successful builds |
| `shasum -a 256 -c checksums.txt` in the candidate output directory | PASS; all artifacts valid |
| `go version -m` on each extracted archive binary | PASS; all eight binaries report `go1.27.1` |
| `/tmp/atlas-dns-go1271-tools/actionlint -color .github/workflows/ci.yml .github/workflows/release.yml` | PASS |
| `/tmp/atlas-dns-111-tools/govulncheck ./...` | PASS; no reachable vulnerabilities; same four uncalled required-module advisories |
| `npm --prefix web audit --omit=dev` | PASS; zero production vulnerabilities |
| `bash -n scripts/*.sh` and `git diff --check` | PASS |
| Official Docker registry manifest for `golang:1.27.1-bookworm` | PASS; Linux amd64 and arm64 present; metadata verification only |
| `docker build --build-arg VERSION=1.1.1 --tag atlas-dns:1.1.1-go1.27.1 .` | BLOCKED; shell exit 127, `docker: command not found` |

The disposable PostgreSQL server was stopped after these checks. The native
binaries were rebuilt with stable `VERSION=1.1.1` after candidate artifact
assembly.

The production Docker image was not built or started locally. No Docker CLI,
Docker Desktop/Colima/Podman installation, or active local Docker socket is
available; the leftover desktop context points at a missing socket. A native
production startup proves the Go 1.27.1 application starts, but does not count
as container qualification. The new CI image startup gate must pass on a
Docker-equipped host before this requirement can be marked complete.

Artifacts are local unpublished candidate outputs under
`dist/release/1.1.1-rc.go1271`; no tag or image was published. Real-device,
container and supported installation qualification remain external gates.

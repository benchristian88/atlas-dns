# Security Release Gate

Run this gate for every Atlas DNS Controller release. A release fails when an
automated gate fails, required evidence is missing, or a confirmed Critical/High
finding or defined release-blocking security property remains unresolved.

## Automated release gates

- [x] Go formatting and documentation links are valid.
  Evidence: `make fmt-check`, `make docs-check`.
- [x] Backend, migration, frontend, and race tests pass.
  Evidence: `make test`, `make test-race`, and `make test-integration` with a
  disposable PostgreSQL database.
- [x] Go code and dependency graph pass static/vulnerability checks.
  Evidence: `go vet ./...`, `go mod verify`, and pinned
  `govulncheck@v1.7.0 ./...` in CI and release workflows.
- [x] Production frontend dependencies have no known npm vulnerability.
  Evidence: `npm audit --omit=dev` in CI and release workflows.
- [x] Every protected API route rejects an unauthenticated direct HTTP request.
  Evidence: `TestProtectedRouteInventoryRequiresAuthentication`.
- [x] Missing, tampered, and expired sessions are rejected.
  Evidence: `TestSessionBoundaryRejectsMissingTamperedAndExpiredTokens`.
- [x] Protected mutations require CSRF and reject privileged over-posting.
  Evidence: `TestMutationBoundaryRejectsCSRFAndMassAssignment` and feature HTTP
  handler suites.
- [x] Administrator-only endpoints use server-derived roles.
  Evidence: `TestUserAdministrationRequiresServerSideAdministratorAndCSRF` and
  `TestControlPlaneLifecycleRoutesRequireAdministratorAndCSRF`.
- [x] Setup is one-time and credentials never appear in normal node/audit API
  responses or database ciphertext.
  Evidence: integration `TestRelease01OperatorWorkflow`,
  `TestCredentialCipherRoundTripAndNodeBinding`, and
  `TestProductisationRoutesRejectUnauthenticatedRequests`.
- [x] Node URLs are HTTP(S)-only, plaintext HTTP is explicit, TLS remains
  verified, redirects are refused, and status bodies are bounded.
  Evidence: `TestNormaliseNodeURL`, `TestProbeSeparatesAuthenticationAndTLSFailures`,
  `TestProbeSupportsCustomCA`, `TestProbeRejectsRedirects`, and
  `TestProbeRejectsOversizedStatusResponse`.
- [x] Published configuration and deployment use server-side revision/target
  state, validate all nodes before mutation, and activate only after read-back.
  Evidence: `TestExecutorValidatesEveryTargetBeforeMutation`,
  `TestExecutorDeploysSequentiallyAndActivatesAfterReadBack`, control-plane
  lifecycle tests, and integration `TestRelease03AuthoritativeConfigurationWorkflow`.
- [x] Drift adoption is explicit and deployment/operational actions retain
  actor, revision/target, per-node results, and safe audit evidence.
  Evidence: control-plane, reconciliation, operation, and integration suites.
- [x] Production frontend type-check, lint, test, and build pass.
  Evidence: `npm run typecheck`, `npm run lint`, `npm test`, `npm run build`.
- [x] Native controller, migration, and backup commands build for Linux amd64
  and arm64; release archive creation/checksums pass.
  Evidence: release workflow `make release-artifacts` plus local cross-builds.

The checkboxes describe required controls, not permanent claims about a future
commit. Review the command output for the exact release candidate before signing
off.

## Manual release checks

- [ ] Review `git diff` and the final commit range for secrets, debug endpoints,
  disabled TLS verification, new public routes, broad permissions, or test-only
  settings in production.
- [ ] Confirm every GitHub Action still points to a reviewed immutable commit in
  its official repository and does not use a deprecated runner runtime.
- [ ] Confirm CI remains `contents: read`; release-only write permissions remain
  scoped to trusted tag/manual triggers and no pull-request secrets are exposed.
- [ ] Confirm current supported status for Go, Node, PostgreSQL, Debian images,
  npm dependencies, and container images using authoritative upstream sources.
- [ ] Review Go release notes between toolchain baselines and triage `npm
  outdated`; do not bundle unrelated major upgrades only for version currency.
- [ ] Build and scan the production multi-platform image in the hosted release
  environment, review its SBOM/provenance, and inspect base-image findings.
- [ ] Run integration tests against disposable PostgreSQL and fake/controlled
  AdGuard Home versions in the documented compatibility range. Never use live
  DNS infrastructure for adversarial tests.
- [ ] Verify `PUBLIC_BASE_URL` is HTTPS for production, cookies carry `Secure`,
  the trusted proxy sets HSTS where policy requires it, and CORS remains absent
  unless a reviewed same-origin policy changes.
- [ ] Verify a preflighted encrypted backup exists and its passphrase/key files
  have restricted ownership and permissions.
- [ ] Recheck migration checksums and confirm released migrations remain
  append-only.
- [ ] Confirm the release contains no unrelated feature work, especially webhook
  notification history or Operational History pagination in a security-only
  workstream.

## Recommended hardening

- Split migration ownership from a least-privilege runtime PostgreSQL role.
- Add optional operator-configured node/webhook egress CIDR and port policy.
- Add distributed/edge authentication throttling before controller HA.
- Sign native release artifacts and enforce hosted container scanning policy.
- Plan supported LTS/toolchain changes before, rather than at, upstream EOL.
- Periodically restore an encrypted backup into an isolated empty database and
  verify the recovered credential-key file remains protected.

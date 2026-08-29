# Release Process

This is the repeatable process for stable 1.x releases. Feature work stops
before release preparation; only blocker fixes, compatibility work,
documentation, and release engineering enter the candidate.

## Version identities

Release tags include the `v` prefix, while build versions and release output
directories do not. The reusable forms are `vX.Y.Z-rc.N` / `X.Y.Z-rc.N` for a
release candidate and `vX.Y.Z` / `X.Y.Z` for a final release. For the current
release, development builds report `1.1.0-dev`, candidates use
`v1.1.0-rc.N`, and the final release is `v1.1.0`.

## Candidate gate

1. Select a semantic candidate version such as `X.Y.Z-rc.1` and freeze scope.
2. Update the changelog, compatibility matrix, support policy, upgrade notes,
   and any schema/API documentation affected by the release.
3. Run formatting, lint, full Go tests and race tests, frontend unit/Axe/assets/
   type/lint/build checks, integration tests, production dependency audit, and
   documentation link/identity checks.
4. Review migrations as append-only, replay the full chain into a clean empty
   database, and exercise every supported prior-release upgrade baseline.
5. Create and preflight a branded backup, restore it offline into a new empty
   database, and verify the recovery checklist.
6. Exercise authentication, CSRF, authorization, last-admin protection, secrets,
   webhook/lifecycle operations, audit, error/log redaction, and update
   boundaries.
7. Validate desktop/mobile Light/Dark/System behavior, keyboard/focus/dialog
   access, representative supported browsers, and PWA/iOS install metadata.
8. Generate the candidate artifacts and install them on clean Docker and Debian
   targets without Go, Node.js, npm, or a source checkout.
9. Record real AdGuard Home compatibility, database/runtime versions,
   performance observations, known limitations, and all unavailable external
   gates without reporting them as passes.

## Local artifact assembly

Use an unused versioned output directory:

```bash
ATLAS_DNS_VERSION=X.Y.Z-rc.1 scripts/release-artifacts.sh
```

The script produces self-contained Linux amd64 and arm64 archives, production
Compose and environment inputs, the native installer, BUSL-1.1 terms, and
`checksums.txt`. Each native archive contains all four commands, the exact
frontend assets, systemd unit, README, and license. If `syft` is available, an
SPDX JSON SBOM is added. Existing output is never overwritten.

## GitHub Actions

`.github/workflows/release.yml` accepts `v*` tags and manual version builds. A
tag supplies the version and always publishes; a manual run uses its version
input and explicit publish choice. The workflow removes an optional leading
`v`, validates the version, and injects that resolved version, the Git commit,
and the UTC build time into native binaries and the OCI image. It reruns its
required repository gates, assembles and uploads native archives and checksums,
and builds the same `linux/amd64` and `linux/arm64` image. A publish-enabled run
uses the repository `GITHUB_TOKEN` to create `vX.Y.Z` or `vX.Y.Z-rc.N` as the
GitHub Release and publish the linked GHCR package; no personal access token is
required.

Stable `vX.Y.Z` publishes image tags `X.Y.Z`, `X.Y`, `X`, and `latest`.
Prereleases such as `vX.Y.Z-rc.N` publish only their exact version tag and never
move `latest`. The workflow refuses to replace an existing exact GitHub Release
or image tag. Operators should pin exact versions.

Third-party actions are pinned to commit SHAs. Workflow permissions are
read-only by default and elevated only in the publishing job to `contents: write`
and `packages: write`. Buildx provenance and SBOM attestations are enabled for
published images.

## External release validation

Before final `vX.Y.Z`, publish and install at least one `vX.Y.Z-rc.N` release
candidate. Verify:

- both native archive checksums and runtime contents;
- anonymous pulls and the amd64/arm64 manifest from GHCR;
- production `compose.yaml` pulls instead of builds;
- Docker Compose and Portainer Stack health/persistence/redeploy behavior;
- the Debian/systemd installer downloads and verifies the archive;
- version/build metadata and update awareness;
- clean install, restart, backup, restore, and supported upgrade behavior.

GitHub may require the repository owner to make the first GHCR package public
and confirm its repository association. That one-time setting is an explicit
external gate; anonymous pull must be retested afterwards.

## Current v1.1 qualification

Before final `v1.1.0`, the candidate evidence must include repository regression
and PostgreSQL-backed integration; a fresh database migration through `000019`;
the supported `v1.0.2 → v1.1.0` upgrade applying pending migrations
`000016` through `000019`; encrypted backup/preflight/empty-database restore;
security, authentication, and MFA regression; and native archive, checksum,
version metadata, and image validation. Retained schema-1 records must remain a
read-only, non-deployable conversion boundary; all authoring and deployment
remain schema v2.

Manual qualification remains required on the supported LXC/device or
production-like host paths, including real AdGuard Home v0.107.78 and v0.107.79
nodes, representative browser/PWA devices, webhook delivery, and relevant
failure/outage behavior. Record unavailable external gates as unavailable, not
as passes.

## Final release

Promote the reviewed candidate commit without additional feature changes.
Create the final tag only when every required automated and external gate is
recorded. After publication, verify the GitHub Release assets and checksums,
public GHCR visibility/manifest/tags, production Compose pull, installer
download, source/docs links, and update-awareness response.

Database migrations are forward-only. A binary-only downgrade after migration
is unsupported unless that release's notes explicitly prove otherwise. Always
create and preflight a backup before updating; recovery restores into a new
empty database.

Post-release work is issue triage, regression/security fixes, and documented
patch guidance. Support scope is defined by the current compatibility and
support policies, with no implied SLA.

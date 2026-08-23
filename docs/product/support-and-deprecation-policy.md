# Support and Deprecation Policy

Atlas DNS Controller 1.0 is the first stable supported release. Support is
community best-effort: no response-time, resolution-time, uptime, or commercial
SLA is offered.

## Where to ask for help

- Use GitHub Issues for reproducible defects and documentation problems.
- Use repository discussions, when enabled, for operator questions.
- Use GitHub's private security-advisory channel for vulnerabilities; follow
  [SECURITY.md](../../SECURITY.md).

Never publish credentials, sessions, database URLs, webhook destinations,
backup archives/passphrases, private Query Log data, or raw node responses.

## Supported versions and environments

- The latest stable 1.x release receives normal defect and documentation work.
- A superseded 1.x release may receive upgrade guidance, but fixes normally
  require moving to the latest stable 1.x release.
- Only environments marked **Supported** or **Tested** in the
  [compatibility matrix](../operations/compatibility-matrix.md) are release
  commitments. Best-effort and unsupported environments carry no compatibility
  promise.
- AdGuard Home v0.107 patches newer than the latest release-tested patch are
  provisionally compatible: Atlas attempts its normal typed capability checks
  and permits operations only when those contracts validate. Other API
  generations are unknown and affected managed writes remain blocked.

## Upgrade and deprecation policy

- Create and preflight an Atlas backup before every upgrade and preserve runtime
  configuration separately.
- Supported 1.x upgrades follow release notes, ordered append-only migrations,
  and the documented installation method.
- Database downgrade is unsupported unless a release explicitly supplies a
  compatible rollback procedure.
- Stable environment variables, configuration semantics, `/api/v1` contracts,
  and backup-format changes are announced in release notes. A safe migration
  path and at least one minor-release deprecation window are provided where
  practical; urgent security removals may use a shorter window.
- Database internals, browser-local state, logs, and undocumented implementation
  details are not public APIs.
- Pre-1.0 installations and backups are not a supported upgrade source. Rebuild
  and install Atlas DNS Controller 1.0 fresh.

## Unsupported cases

Custom forks, source builds, incompatible or unknown AdGuard Home API generations, unsupported
PostgreSQL majors, modified release archives/images, exposed management APIs,
and deployments that bypass documented secret or migration controls are
best-effort or unsupported. Atlas support does not cover AdGuard Home defects,
host networking, DNS client configuration, reverse proxies, PostgreSQL
administration, Docker, or Portainer themselves.

## Licence boundary

Atlas DNS Controller is source-available under BUSL-1.1, not open source while
that licence governs a version. The Additional Use Grant permits
non-commercial personal and homelab use and prohibits commercial hosting or
resale. Support policy does not expand the rights in [LICENSE](../../LICENSE).

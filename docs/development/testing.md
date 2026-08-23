# Testing Strategy

Tests follow product boundaries: desired versus observed state, immutable
history, server-authoritative mutation safety, secret redaction, node
attribution, and DNS independence. Point-in-time release evidence lives in the
[pre-1.0 archive](../archive/pre-1.0/README.md), not in this strategy.

## Standard checks

```bash
make fmt-check
make docs-check
make test
make test-race
make lint
make build
```

Frontend-specific checks are available under `web/`:

```bash
npm test
npm run typecheck
npm run lint
npm run test:assets
npm run build
```

Run `git diff --check` before handoff. Shell/package/Compose changes also require
syntax/config validation. Network/package audits and external browser/container
checks must record their environment and date.

## Unit and domain tests

Cover canonicalization/hashing, desired/effective projection, structured diff,
capability/version validation, configuration/DHCP invariants, reconciliation,
deployment state transitions, retry/error classification, aggregation,
retention bounds, backup envelope validation, and secret redaction. Failure-path
tests should prove what remains unchanged and which evidence is retained.

## API and contract tests

- Authentication, administrator authorization, CSRF, request validation, stable
  errors, optimistic versions, and idempotency where applicable.
- No credential, destination, node-response, private TLS, query-content, stack,
  or internal schema leakage.
- Exact supported AdGuard Home read/write method/path/payload fixtures across the
  compatibility boundary.
- Unknown/partial capability and timeouts remain safe and node-attributed.
- Archive/delete eligibility is recalculated by the server and conflicting
  references return safe errors.

## Frontend tests

Use React DOM tests for loading, empty, partial, stale, error, and success states;
keyboard/focus/dialog behavior; responsive semantic structure; theme/navigation;
typed API handoffs; and exact destructive confirmation. Run Axe checks on
critical layouts. jsdom is not evidence for browser rendering, touch, actual
media queries, color contrast, favicon/PWA behavior, or horizontal overflow;
those require packaged browser checks.

## PostgreSQL integration tests

Set `TEST_DATABASE_URL` to a disposable PostgreSQL 17 database:

```bash
TEST_DATABASE_URL='postgres://…' make test-integration
```

Every migration should exercise up behavior and preservation from the previous
schema; down behavior is development-only and may require disposable state.
Integration tests cover transactions, locks/concurrency, foreign-key conflicts,
audit persistence, worker claims/restarts, backup inclusion policy, and retained
history.

The release 0.9.2 integration case specifically proves active/referenced revision
safety, archive visibility/restore, unused revision deletion, terminal deployment
archive, started deployment preservation, unstarted deployment deletion, and
webhook delivery/event survival after channel deletion.

The release 1.0.2 integration case seeds healthy DNS, drives failed/stable-
failed/recovered probe states, verifies exactly one failure and one recovery
webhook at a TLS fake receiver, records a Test result, rejects repeated degraded
spam, checks redaction, and walks every combined Operational History cursor
page.

## AdGuard Home integration

Use real or isolated stateful fixtures for authentication, status, configuration
reads/writes/read-back, filtering, DHCP, Statistics, Query Log, DNS probes, TLS
metadata, and version capability behavior. A critical control-plane workflow is:

1. Create cluster and two nodes.
2. Import and save desired configuration.
3. Publish an immutable revision.
4. Deploy sequentially and verify both nodes.
5. Change a node directly and detect drift.
6. Restore/Enforce through a verified deployment.
7. Publish another revision and perform deployment-based rollback.

No test may imply the controller answered DNS; DNS service must remain node-owned
when the controller is stopped.

## Backup and recovery tests

Test Standard/Full component policy, passphrase and checksum failure, future
format/schema rejection, bounded upload/extraction, protected input files,
empty-database enforcement, one-transaction restore, credential-key handling,
session/cache exclusion, archived-record restoration, and absence of records
hard-deleted before backup. A release gate includes a timed clean-database
restore followed by login, node decryption/observation, revision/deployment/drift,
and collector verification.

## External release matrix

Automated checks do not replace:

- Current Chromium, Firefox, Safari/iOS desktop/tablet/mobile interaction and
  accessibility checks.
- Docker Compose clean install, upgrade, restart, backup, restore, and rollback.
- Debian 13/systemd clean install, upgrade, restart, backup, restore, rollback,
  and hardened service verification.
- Real PostgreSQL migration/lock/query-plan/retention behavior.
- Supported real AdGuard Home version and DNS failure/maintenance/upgrade cases.

If an environment is unavailable, report the gate as pending; never convert a
compiled/skipped or jsdom result into external evidence.

# Tests

Run Go unit tests and the frontend suite with `make test`. The PostgreSQL-backed
integration suite is organized by the release boundary that introduced the
behavior:

- `release_0_1_test.go`, `release_0_3_test.go`, and `release_0_4_1_test.go`
  cover bootstrap/API operation, authoritative deployment/drift/rollback, and
  durable node-attributed operational commands.
- `release_0_6_test.go` through `release_0_9_2_test.go` cover Query Log schema
  and failure scanning, HA lifecycle evidence, user/settings persistence, and
  lifecycle/webhook-history cleanup.
- `release_1_0_1_test.go` and
  `release_1_0_2_notification_history_test.go` cover migration-ledger
  stability, node cleanup, notification-history upgrade, DNS transition
  delivery, pagination, and mixed destination results.
- `release_1_1_onboarding_test.go`, `release_1_1_runtime_test.go`,
  `release_1_1_audit_evidence_test.go`, and `release_1_1_mfa_test.go` cover
  onboarding/runtime/notification persistence, v1.0.2 runtime upgrade,
  Operational History isolation, Audit Log keyset/redaction behavior, and the
  MFA enrollment/challenge/recovery/host-reset flow.

The suite requires PostgreSQL 17 and an explicitly supplied connection URL:

```bash
TEST_DATABASE_URL='postgres://user:password@127.0.0.1:5432/atlas_dns_test?sslmode=disable' make test-integration
```

Each integration test creates and later drops an isolated schema, applies the
embedded migration chain, exercises rollback/reapply setup, and uses in-process
HTTP servers where AdGuard Home behavior is needed. Optional `TEST_NODE_A_URL`
and `TEST_NODE_B_URL` values can target external compatible nodes when supplied
together.

The direct `go test ./tests/integration` command skips PostgreSQL cases when
`TEST_DATABASE_URL` is unset. Those skips confirm only that the suite compiles;
they are not a release-qualification pass. `make test-integration` fails early
when the variable is absent. Full PostgreSQL fresh-install and supported
v1.0.2-upgrade validation remains required before the v1.1 release candidate.

# Operations and Troubleshooting Runbook

This runbook covers the supported Atlas DNS Controller 1.x operating model. It
assumes a production installation from the published Compose inputs or verified
Debian/systemd release archive.

## Start, stop, and verify

Docker Compose:

```bash
docker compose up -d
docker compose ps
docker compose logs --tail=100 atlas-dns
curl --fail http://127.0.0.1:8080/health
curl --fail http://127.0.0.1:8080/ready
```

Native systemd:

```bash
systemctl --no-pager --full status atlas-dns
journalctl -u atlas-dns --since '15 minutes ago'
curl --fail http://127.0.0.1:8080/health
curl --fail http://127.0.0.1:8080/ready
```

`/health` proves only that the process can respond. `/ready` also checks the
database and startup/migration state. Do not send browser traffic to an instance
that is not ready.

## Startup failures

Check, without printing secret values:

1. PostgreSQL 17 is reachable from the controller network namespace.
2. `DATABASE_URL`, `PUBLIC_BASE_URL`, `SESSION_SECRET`, and
   `CREDENTIAL_ENCRYPTION_KEY` are set.
3. `PUBLIC_BASE_URL` exactly matches the browser-visible scheme, host, and port.
4. the version-matched frontend exists at `WEB_DIST_DIR`;
5. migration checksum and ordering errors in the service log;
6. write access only to the documented temporary/state directory.

Never paste the environment file, connection URL, node responses, session
cookies, or credential key into an issue. Preserve the request ID and safe error
code instead.

## Login, CSRF, and browser symptoms

- A Secure cookie will not work over plain HTTP. Use HTTPS in production and
  ensure `PUBLIC_BASE_URL` is correct.
- A CSRF failure usually means a stale page/cookie pair or origin mismatch.
  Reload once, then inspect the configured origin and reverse-proxy headers.
- Disabling an administrator or resetting its password revokes its sessions.
- An MFA password success intentionally sets no session cookie until the TOTP or
  recovery challenge succeeds. Repeated factor failures consume the challenge
  after five attempts; start a new password login rather than refreshing an old
  challenge.
- The final enabled administrator cannot be disabled. Use another enabled
  administrator for account recovery.
- Clear only `atlas-dns.theme` to reset browser theme preference. Do not clear
  controller cookies as a theme workaround.

### Lost authenticator and recovery codes

There is no browser administrator bypass. An owner with host/container shell
access can deliberately reset MFA for one local login. The command deletes the
encrypted seed and recovery hashes, consumes outstanding challenges, revokes all
sessions for that user, and records `user.mfa.reset_from_host`; it prints only
the normalized email and revoked-session count.

Docker Compose:

```bash
docker compose exec atlas-dns \
  atlas-dns-admin reset-mfa --email operator@example.test
```

Native systemd (the protected environment file supplies the existing database,
session, and credential-key configuration without printing it):

```bash
sudo sh -c 'set -a; . /etc/atlas-dns/atlas-dns.env; set +a; \
  exec /usr/local/bin/atlas-dns-admin reset-mfa --email operator@example.test'
```

Use exact local login email and trusted local shell history policy. On success,
the user signs in with the normal password and may enroll MFA again. A missing
user or database/configuration failure changes nothing and returns non-zero.

## Node connection and compatibility

For an unreachable or rejected node, verify the stored base URL, DNS-running
state, management-network route, request timeout, and selected TLS trust policy.
Atlas does not follow redirects or use ambient HTTP proxy settings for node
credentials.

An authentication failure is distinct from TLS and network failure. Replace
credentials through the node form; they are write-only and cannot be read back.
An unknown or unsupported AdGuard Home contract remains observable where safe,
but managed writes are blocked. Do not bypass capability validation; update the
compatibility fixtures and policy in a reviewed release.

Deleting a node preserves its historical observations, revisions, deployment
results, health evidence, and audit attribution, but removes that UUID from the
mutable draft. Re-adding the same URL creates a new node UUID and does not
inherit maintenance state or node-specific desired values by URL. Refresh the
replacement's observation, explicitly import it into the draft, validate, and
publish a new immutable revision before previewing deployment. A preview of a
pre-replacement revision correctly fails with guidance that the new identity is
absent; do not edit or delete historical revisions to bypass it.

## Desired configuration, deployment, and drift

The safe operating sequence is:

```text
refresh observation
→ edit draft
→ validate and publish immutable revision
→ review deployment preview
→ deploy sequentially
→ verify every node
→ activate revision
```

Publication does not change a node. A failed deployment stops at the first
failed node and preserves per-node evidence; it does not silently roll back.
Repair the cause, refresh every affected node, and deliberately redeploy the
desired or a reviewed historical revision.

Direct node changes produce drift. In Enforce mode Atlas may restore desired
state only through the normal verified deployment path. In Alert or Manual mode
the operator chooses restore or adopt. Maintenance suppresses automatic
reconciliation but does not erase drift evidence.

Return-to-service runs fresh live checks. Existing drift is reported as a
warning and reconciliation resumes after maintenance is cleared. A required
API, observation/capability, DNS, configuration-state, DHCP-safety, applicable
TLS, or configured-collector failure leaves the node in Maintenance Mode. The
page reports safe check detail and a Request ID; controller logs retain the safe
check name/status/code.

The `tls` return check reads redacted state from the fresh AdGuard Home
`GET /control/tls/status` observation. It is not a connection test to port 443
or 853. If encryption is disabled, `not_applicable` is expected even when the
node retains old certificate metadata. If encryption is enabled, repair the
reported certificate time, certificate/chain validity, key validity, or
certificate/key pairing issue in AdGuard Home, refresh the node, and retry.
`unknown` means the fresh observation could not establish TLS applicability and
fails closed. For an HTTPS administration URL, transport trust and hostname
failures instead appear in the separate required `api` check as
`NODE_TLS_FAILED`; Atlas never disables certificate verification.

In certificate expiry views, **Not applicable** means TLS is deliberately
disabled, **Unknown** means enabled/expected certificate expiry could not be
determined, **Invalid** is a blocking enabled-TLS validation result rather than
an expiry date, and **Expired** is reserved for a present, applicable certificate
whose parsed expiry is in the past. Disabled TLS does not create certificate
Attention, operational alert/recovery events, or webhook deliveries.

Archive hides terminal historical records without making them mutable. Hard
deletion is restricted to unreferenced unused revisions and never-started,
effect-free deployments, requires strong confirmation, and is audited.

## DNS health, DHCP, and guided upgrades

Atlas probes the configured DNS listener over UDP and TCP independently from
the AdGuard Home administration API. If API health is good but DNS is not,
check listener bindings, port, firewall, probe name/address, and controller
network reachability. Nodes continue serving DNS if Atlas is unavailable.

Before enabling DHCP, run the active-server check and review every reported
IPv4/IPv6 protocol result. Only one designated node may own enabled DHCP in a
cluster. Planned handoff requires disable, verify, then enable; partial or
unavailable checks fail closed.

Guided AdGuard Home upgrades coordinate maintenance and capacity but never run
remote shell or package commands. Preflight requires sufficient verified DNS
capacity. Return to service requires authenticated API access, compatible
version, DNS probes, a fresh observation, and convergence.

## Statistics, Query Log, and Operational Status

Statistics and Query Log are asynchronously collected and always retain node
attribution. Missing coverage is not zero traffic. Check worker status, node
eligibility/retention, last success, gaps, and the configured polling interval
before treating a partial result as a DNS problem.

Query Log may contain sensitive client/domain data. Keep retention bounded,
avoid copying raw rows into support material, and disable collection where
policy requires it. A node clear/reset or retention rollover can create a known
gap because AdGuard Home supplies no stable event ID.

Operational Status distinguishes worker failure, database readiness, API
health, DNS health, convergence, maintenance, certificate/version warnings,
and data freshness. Use the dimension's code and node attribution rather than a
single colour or summary badge when troubleshooting.

## Webhooks and notifications

Webhook destinations must be HTTPS and cannot include userinfo or fragments.
Tests and deliveries are bounded and do not follow redirects. The UI/API returns
only scheme/host summaries and safe codes. Editing preserves the secret unless
replacement is explicit; deletion retains historical event/delivery identity.

If delivery fails, test DNS/TLS/network reachability from the controller,
destination policy, and the safe Operational History reason/status. `Pending`
may retry up to five total attempts; `Failed` is terminal. A channel created
after degradation does not replay that existing state, so validate with a
controlled healthy → degraded transition and restore it afterward. Never log
or publish the stored URL, token, response body, or request payload.

## Backup and disaster recovery

Before upgrades or material lifecycle changes, create a Standard backup (or
Full when retained operational history is required), store the passphrase
separately, and run preflight. Restore is offline into a new empty PostgreSQL 17
database; never target the live database.

Follow the [backup and restore guide](backup-and-restore.md). Keep the original
database, archive, and runtime configuration until administrator login, node
credential decryption, desired/active revisions, deployments, drift, workers,
HA state, per-user MFA login, and expected history are verified. MFA seeds stay
encrypted and recovery codes stay hashed in the dump; transient challenges and
all browser sessions are excluded. The established format-v1 archive also
contains the credential key only inside its passphrase-encrypted authenticated
payload, so protect archive and passphrase separately. Pre-1.0 archives are
unsupported.

## Controller updates

Review release notes, the [compatibility matrix](compatibility-matrix.md), and
[upgrade policy](upgrade-policy.md). Create and preflight a backup first.

Docker operators set an exact `ATLAS_DNS_VERSION`, then run:

```bash
docker compose pull atlas-dns
docker compose up -d atlas-dns
```

Native operators download the new release's installer and checksums, verify the
installer, then rerun it with the exact version. The installer verifies the
matching prebuilt archive and preserves the environment file. After either
path, check readiness, About version/schema, login, nodes, convergence,
collectors, and HA state.

Migrations are forward-only. Replacing only the image/binary with an older
version after a schema change is unsupported unless release notes explicitly
permit it. Recovery normally restores the pre-upgrade backup into a new empty
database with the matching exact artifact.

## Incident evidence

Preserve version/commit/schema, UTC timestamps, request IDs, safe error codes,
audit actions, deployment/delivery IDs, node attribution, and relevant health
transitions. Exclude secrets and private Query Log data. For suspected security
issues follow [SECURITY.md](../../SECURITY.md), not a public issue.

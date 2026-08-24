# Backup and Restore

Portable backups protect controller state; they do not replace node-independent
DNS resilience. Create and preflight a backup before every controller upgrade or
material configuration/lifecycle change.

## What to preserve

- A Standard or Full `.atlasdnsbackup` archive.
- Runtime environment configuration (`.env` or
  `/etc/atlas-dns/atlas-dns.env`).
- Database/runtime TLS material and deployment configuration.
- Source application version, restore target, and the preflight result.

The portable archive contains the PostgreSQL dump and credential-encryption key
inside one passphrase-encrypted authenticated envelope. Runtime database URL,
session secret, public origin, and TLS are target configuration and remain
separate.

## Standard versus Full

- **Standard** contains the control plane: users/password hashes/enabled state,
  nodes/encrypted credentials, drafts, immutable observations/revisions,
  revision/deployment archive metadata, deployments/results, drift, audit,
  lifecycle/upgrades, notification channels/category subscriptions, onboarding
  completion/acknowledgements, and persisted runtime system settings.
- **Full** includes the same control plane plus retained Statistics, Query Log,
  DNS probes, HA events, and notification delivery history.

Sessions and controller-release cache data are excluded from restore. Safely
hard-deleted unused revisions/deployments are absent from backups created after
deletion and do not reappear. Archive status restores as control-plane state.

## Create and preflight in the UI

System → Backup & Restore creates a Standard or Full download with a new
passphrase of at least 16 characters. Store the download away from controller
temporary workspace. Upload it with the passphrase to **Restore preflight**;
preflight validates the envelope, manifests, checksums, source/schema
compatibility, type, components, time, and size without mutating PostgreSQL.

Do not regard an archive as usable until preflight succeeds and a periodic
clean-database restore exercise has completed.

## CLI create and preflight

Passphrases must come from a bounded `0600` regular file that is not a symlink:

```bash
atlas-dns-backup create --type standard --output controller.atlasdnsbackup \
  --passphrase-file /run/atlas-dns-backup-passphrase

atlas-dns-backup preflight --archive controller.atlasdnsbackup \
  --passphrase-file /run/atlas-dns-backup-passphrase
```

Use `--type full` only when operational history is required and storage/privacy
policy permits it. Never place the passphrase or target database URL directly in
process arguments.

## Restore safety contract

Restore is intentionally offline and requires:

1. The controller is stopped for the entire operation.
2. A separately created PostgreSQL database with no public tables.
3. A protected passphrase file and protected file containing the target database
   URL.
4. An explicit `RESTORE_TO_EMPTY_DATABASE` confirmation.

```bash
atlas-dns-backup restore --archive controller.atlasdnsbackup \
  --passphrase-file /run/atlas-dns-backup-passphrase \
  --target-database-url-file /run/atlas-dns-restore-database-url \
  --credential-key-output /etc/atlas-dns/restored-credential.key \
  --confirm RESTORE_TO_EMPTY_DATABASE
```

Restore uses `pg_restore` in one transaction. Failure leaves the target empty and
removes newly written key output. Configure the recovered value as
`CREDENTIAL_ENCRYPTION_KEY`, point `DATABASE_URL` to the restored database, and
preserve the target's session secret/public URL/TLS.

## Docker clean-database recovery

Place the archive and protected files in a root-owned `0700` `recovery`
directory. Stop only the controller and create a new database alongside the old
one:

```bash
docker compose stop atlas-dns
docker compose exec postgres createdb -U atlas_dns atlas_dns_restored
docker compose run --rm --no-deps --user 0:0 \
  --entrypoint atlas-dns-backup \
  --volume "$PWD/recovery:/recovery" atlas-dns restore \
  --archive /recovery/controller.atlasdnsbackup \
  --passphrase-file /recovery/passphrase \
  --target-database-url-file /recovery/database-url \
  --credential-key-output /recovery/restored-credential.key \
  --confirm RESTORE_TO_EMPTY_DATABASE
```

The target database URL must use Compose host `postgres`. Put the restored
database name and recovered key into protected `.env`, then recreate only the
`atlas-dns` service. The temporary root user is limited to this stopped-controller
command; normal runtime remains unprivileged and must not mount the Docker
socket.

The command above assumes the default `POSTGRES_USER=atlas_dns`. Substitute the
configured role and use a URL-encoded password in the protected target URL when
the deployment differs.

## Post-restore verification

Keep the old database and archive unchanged until all checks pass:

1. `/ready`, About version/schema, and administrator login.
2. Disabled accounts and session invalidation expectations.
3. Node credential decryption, connection test, and fresh observation.
4. Draft, active/archived revisions, deployments, per-node results, and drift.
5. Lifecycle settings/upgrades, webhook summaries/categories, onboarding
   completion, runtime system settings, and audit.
6. Statistics/Query Log/HA/delivery history expected for the chosen backup type.
7. New collector records and no unexpected known gaps.

Keep Enforce disabled until desired and observed state are reviewed. Record
duration, archive type/size, excluded history, source/target versions, and any
freshness gap. The [backup format](backup-format.md) defines the archive contract.

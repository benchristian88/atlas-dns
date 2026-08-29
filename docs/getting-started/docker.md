# Install with Docker Compose

This is the supported Docker Compose installation, update, rollback, and
uninstall guide for Atlas DNS Controller 1.x. It pulls the published GHCR image;
it does not compile source.

## Requirements

- Docker Engine with the Compose v2 plugin.
- OpenSSL for secret generation.
- An HTTPS origin for normal browser use, usually through a trusted reverse
  proxy.
- Network access from the container to each AdGuard Home administration API.

Go, Node.js, npm, a repository checkout, privileged mode, a Docker socket mount,
and DNS port exposure are not required.

## Install 1.1.0

Create a deployment directory and download the versioned production inputs from
the GitHub Release:

```bash
mkdir atlas-dns && cd atlas-dns
curl -fsSLO https://github.com/benchristian88/atlas-dns/releases/download/v1.1.0/compose.yaml
curl -fsSLo .env https://github.com/benchristian88/atlas-dns/releases/download/v1.1.0/atlas-dns.env.example
```

Generate three independent values:

```bash
openssl rand -hex 24
openssl rand -base64 48
openssl rand -base64 32
```

Edit `.env`. Set `POSTGRES_PASSWORD`, `SESSION_SECRET`, and
`CREDENTIAL_ENCRYPTION_KEY` to those values, set `PUBLIC_BASE_URL` to the
browser-visible origin, and keep `ATLAS_DNS_VERSION=1.1.0`. The database
password must use URL-safe characters because Compose interpolates it into
`DATABASE_URL`.

Validate, pull, and start the published image:

```bash
docker compose config
docker compose pull
docker compose up -d
docker compose ps
docker compose logs --tail=100 atlas-dns
```

Open `PUBLIC_BASE_URL`, create the initial administrator, and follow Setup
Guide. The named PostgreSQL volume is authoritative persistent state;
`atlas-dns-work` is restricted temporary workspace.

## Verify

```bash
curl --fail http://127.0.0.1:8080/health
curl --fail http://127.0.0.1:8080/ready
docker image inspect ghcr.io/benchristian88/atlas-dns:1.1.0
```

Then verify login, About version metadata, one node connection, Operational
Status, and collector recovery. `/ready` includes PostgreSQL; `/health` is only
process liveness.

## Back up and restore

Create and preflight a `.atlasdnsbackup` through System → Backup & Restore and
copy it to protected storage outside Docker volumes. Preserve `.env`
separately. Offline disaster recovery uses a new empty PostgreSQL database and
the packaged `atlas-dns-backup` command as described in the
[backup and restore guide](../operations/backup-and-restore.md).

## Update within 1.x

1. Review the release notes and compatibility matrix.
2. Create and preflight a backup; retain the current `.env` and Compose file.
3. Set `ATLAS_DNS_VERSION` to the exact reviewed 1.x version.
4. Pull and recreate, then verify readiness and application metadata.

```bash
docker compose pull
docker compose up -d
docker compose ps
docker compose logs --tail=100 atlas-dns
```

Exact version tags are the supported production pin. Major/minor and `latest`
tags are convenience pointers, not reproducible deployment identities.

For a v1.0.x to v1.1.0 update, preserve the old `.env` through the first 1.1
startup so deprecated runtime values can seed PostgreSQL once. Verify System
Settings, then remove those legacy variables from normal environment management.
Data remains in `postgres-data`; do not recreate the volume.

## Rollback boundary

Database migrations are forward-only unless a release explicitly documents a
compatible rollback. Reverting only the image after a schema migration is not a
supported rollback. Preserve the previous Compose input and restore a verified
backup into a new database when release notes require recovery.

## Uninstall or rebuild

`docker compose down` removes containers and retains named volumes. A fresh
rebuild is:

```text
verified backup → docker compose down → new deployment → restore to new database
```

Removing `postgres-data` destroys controller state and is not routine
uninstallation. Verify a recoverable backup before deleting any volume.

## Development builds

Source builds are contributor workflows only. From a checkout, developers may
run:

```bash
docker compose -f compose.yaml -f compose.dev.yaml build
docker compose -f compose.yaml -f compose.dev.yaml up -d
```

Those commands are not the supported production installation path.

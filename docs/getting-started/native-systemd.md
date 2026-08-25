# Install on Debian 13 with systemd

This is the supported native installation and update guide. The reference
target is Debian 13 on amd64 or arm64 with systemd. Installation consumes a
prebuilt GitHub Release archive and never compiles Atlas DNS Controller.

## Requirements

- Root access on Debian 13 or a compatible unprivileged LXC.
- PostgreSQL 17 server and client tools, including matching `pg_dump` and
  `pg_restore` major versions.
- `curl`, `sha256sum`, `tar`, OpenSSL, and systemd.
- An HTTPS browser-visible origin and controller-to-node administration API
  access.

Go, Node.js, npm, Make, Git, and a repository checkout are not required.

## Download and verify the installer

```bash
mkdir atlas-dns-install && cd atlas-dns-install
curl -fsSLO https://github.com/benchristian88/atlas-dns/releases/download/v1.1.0/install-systemd.sh
curl -fsSLO https://github.com/benchristian88/atlas-dns/releases/download/v1.1.0/checksums.txt
grep ' install-systemd.sh$' checksums.txt | sha256sum --check
chmod 0755 install-systemd.sh
```

Review the verified script, then run it with an exact version:

```bash
sudo ATLAS_DNS_VERSION=1.1.0 \
  PUBLIC_BASE_URL=https://controller.example.test \
  ./install-systemd.sh
```

The installer detects amd64/arm64, downloads the matching release archive over
TLS, obtains `checksums.txt`, verifies the archive before extraction, checks all
required runtime files, and then installs them. It creates the unprivileged
`atlas-dns` account and local PostgreSQL database, generates protected runtime
secrets, installs `atlas-dns.service`, starts it, and waits for `/ready`.

Omitting `ATLAS_DNS_VERSION` resolves GitHub's latest stable release. Exact
version selection is recommended for reproducibility. Prereleases must be
selected explicitly.

Terminate TLS with a trusted reverse proxy. `PUBLIC_BASE_URL` must exactly match
the browser-visible HTTPS origin so Secure cookies and CSRF origin validation
work correctly.

## Verify

```bash
systemctl --no-pager --full status atlas-dns
journalctl -u atlas-dns --since '10 minutes ago'
curl --fail http://127.0.0.1:8080/health
curl --fail http://127.0.0.1:8080/ready
/usr/local/bin/atlas-dns --version
```

Open `PUBLIC_BASE_URL`, create the initial administrator, add a node, and check
Operational Status. Atlas DNS Controller does not listen on DNS ports.

## Installed files

- `/usr/local/bin/atlas-dns` — API, frontend, and workers.
- `/usr/local/bin/atlas-dns-backup` — backup/preflight/offline restore CLI.
- `/usr/local/bin/atlas-dns-migrate` — explicit migration utility.
- `/usr/local/share/atlas-dns/web` — version-matched frontend.
- `/usr/local/share/atlas-dns/LICENSE` — BUSL-1.1 terms.
- `/etc/atlas-dns/atlas-dns.env` — root-owned `0600` runtime configuration.
- `/var/lib/atlas-dns` — service-owned restricted working state.
- `/etc/systemd/system/atlas-dns.service` — hardened service unit.

Never copy the environment file into logs or issue reports.

## Back up, update, and recover

Create and preflight a backup before every update. Preserve the environment file
separately. To update within supported 1.x, download and verify the new installer
and checksums, review the release notes, then rerun with the exact new version.
The installer preserves the environment file and restarts the service with the
version-matched API and frontend.

Downgrading a binary after a forward schema migration is unsupported unless the
release notes explicitly say otherwise. Recover by stopping the service and
restoring a verified backup into a new empty database; follow the
[backup and restore guide](../operations/backup-and-restore.md).

For v1.0.x to v1.1.0, rerun the verified installer. It preserves the old
environment through first startup so deprecated runtime values seed PostgreSQL
once. Verify System Settings before removing those deprecated entries. Fresh
1.1 installs write only bootstrap, networking, secret, migration, and identity
values to the protected environment file.

## Uninstall or rebuild

Stop and disable the service before removing the installed runtime:

```bash
sudo systemctl disable --now atlas-dns.service
sudo rm -f /etc/systemd/system/atlas-dns.service
sudo rm -f /usr/local/bin/atlas-dns /usr/local/bin/atlas-dns-backup /usr/local/bin/atlas-dns-migrate
sudo systemctl daemon-reload
```

These commands intentionally retain `/etc/atlas-dns/atlas-dns.env`,
`/usr/local/share/atlas-dns`, `/var/lib/atlas-dns`, PostgreSQL state, and the
service account. Keep them and a verified backup until rebuild/recovery is
confirmed; remove retained data only under a separate explicit destruction
decision. Reinstall by downloading and verifying the chosen release's installer
again. The supported transition from any pre-1.0 installation is a fresh Atlas
DNS Controller 1.0 install, not an in-place service/path migration.

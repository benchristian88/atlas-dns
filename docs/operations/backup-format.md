# Portable Backup Format

Atlas DNS Controller 1.x archives use the `.atlasdnsbackup` extension and
format version 1. They are intended for Atlas DNS Controller recovery, not as a
general PostgreSQL interchange format.

## Envelope

The bounded outer envelope contains:

1. ASCII magic `ATLASDNSBACKUP` followed by a newline.
2. A four-byte big-endian unsigned manifest length, limited to 64 KiB.
3. A UTF-8 JSON envelope with the non-secret manifest, encrypted-payload size,
   and SHA-256 checksum.
4. One `age` scrypt-recipient encrypted payload.

The encrypted payload is a deterministic-name tar stream with exactly:

- `manifest.json`: authenticated copy of the manifest;
- `database.dump`: PostgreSQL custom-format dump;
- `credential.key`: base64 credential-encryption key.

Only regular files with those exact basename-only paths are accepted. Duplicate
entries, links, devices, traversal, absolute paths, unknown entries, excess
entries, oversized metadata/key/archive data, checksum differences, trailing
payload data, malformed JSON/tar/age data, and unsupported format versions are
rejected before restore.

## Manifest

The manifest records application identity `atlas-dns`, format, application
version, build identifier, database schema version, UTC creation time, Standard
or Full type, included/excluded
components, entry SHA-256 checksums, passphrase requirement, and the fact that
sessions are not restored. The outer and authenticated inner manifests must be
structurally identical.

Pre-1.0 archives using `.aghhabackup`, `AGHHABACKUP`, or the old application
identity are rejected. No compatibility shim or automatic conversion is
provided; retain a matching pre-1.0 environment if historical recovery is
required. Format v1 is the supported baseline for 1.x. Compatible readers may
accept older 1.x schema versions only when the release notes and migration tests
explicitly say so.

## Standard and Full

Standard Backup includes database schema and required control-plane rows. It
includes database-backed System Settings and notification policy. It
excludes table data for sessions, controller/upstream release caches,
Statistics, Query Log ingestion/events, DNS probes, HA operational events, and
notification deliveries. The table schema is retained so a restored database
is immediately valid.

Full Backup retains the same recovery state plus all retained operational
history. Neither type restores browser sessions.

## Security and temporary data

The passphrase is never stored. `age` performs the passphrase KDF and
authenticated encryption. Temporary directories and files use `0700`/`0600`,
are fixed-name and bounded, and are removed after download/preflight. Database
passwords are removed from `pg_dump`/`pg_restore` command arguments and passed
through the child process environment. The restore CLI reads both its
passphrase and target database URL from protected bounded files, never process
arguments. Tool output containing URL-like material is not returned.

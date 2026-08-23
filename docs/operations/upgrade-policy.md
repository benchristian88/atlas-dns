# Atlas DNS Controller 1.x Upgrade and Migration Policy

Release 1.0.0 is the clean stable baseline. There is no supported in-place
upgrade or backup-restore guarantee from the pre-1.0 product.

## Database migrations

- The immutable `000001`–`000014` chain shipped in v1.0.0 is the physical clean
  bootstrap and checksum ledger for the supported baseline. It is retained even
  though its filenames describe pre-1.0 development milestones.
- Released migrations are immutable and append-only.
- The embedded runner applies pending migrations in numeric order inside the
  migration's defined transaction boundary and records name, checksum, and UTC
  application time.
- Startup fails readiness rather than serving a partially migrated schema.
- A migration checksum mismatch fails closed.
- Schema downgrade is not generally supported. A down migration in source is a
  development aid unless release notes explicitly authorize it for rollback.
- Schema-neutral patch releases do not add empty migrations. Release 1.0.1 uses
  the v1.0.0 schema unchanged; Release 1.0.2 appends `000015` for notification
  delivery diagnostics and history indexing.

## Supported 1.x upgrade flow

```text
review release notes
→ create and preflight compatible backup
→ preserve runtime environment
→ install/pull exact new artefact
→ apply ordered migrations
→ verify readiness, version, nodes and workers
```

Release notes identify database, configuration, environment, API, and backup
compatibility changes. Operators must not skip a required intermediate version
when release notes specify one.

## Configuration and API stability

Documented runtime variables and `/api/v1` are stable 1.x interfaces. A rename
or removal normally receives a documented replacement and at least one
minor-release deprecation window where practical. Secrets never gain insecure
compatibility aliases. Database identifiers and undocumented browser-local
state are not public interfaces.

## Backup compatibility

Atlas backup format v1 is accepted only when its Atlas application identity,
format, application version, and database schema are compatible with the target.
A later 1.x release must either restore a supported earlier 1.x backup or state
the required intermediate recovery path before release. Future application or
schema backups fail closed on older targets.

Rollback after a migration normally means restoring the pre-upgrade backup into
a new empty database and reinstalling the matching exact artefact. Replacing
only the binary/image against a newer schema is unsupported.

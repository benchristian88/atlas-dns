# Administration Guide

All current users are local administrators. The server authorizes every
mutation, applies CSRF protection to browser requests, and records security- and
control-plane-sensitive actions in Audit.

## Initial setup and Setup Guide

When no user exists, the application offers **Create your administrator**. The
setup transaction creates the first administrator and session; subsequent setup
attempts are rejected. Create a cluster, register nodes, test connectivity,
import desired configuration, publish a revision, and verify the first
deployment. Setup Guide derives its steps from actual controller state and can
be revisited at any time.

A cluster with no nodes is a valid first-run state. Setup Guide renders its
checklist, marks node-dependent work incomplete, and links to Add first node;
loading and genuine controller/API failures remain separate states.

## Users

System → Users creates additional local administrators, disables/re-enables an
account, and resets credentials. Disabling or resetting revokes existing
sessions. The controller prevents self-disable and prevents disabling the final
enabled administrator. User hard deletion and additional roles are not
available, preserving audit attribution and the current authorization boundary.

## Webhooks and notifications

HA → HA Operations owns the single notification-channel subsystem.

- **Add webhook** accepts a unique name, enabled state, and HTTPS destination.
- **Edit webhook** can rename or change enabled state while retaining the hidden
  destination. Select **Replace destination secret** only for a deliberate
  replacement.
- **Disable/Enable webhook** pauses/resumes new notifications without discarding
  configuration.
- **Test webhook** sends one bounded synthetic event directly, follows no
  redirects, reports a safe result, is audited, and adds a clearly labelled
  Test delivery row to Operational History. It does not alter HA state, notify
  other channels, or reveal the destination.
- **Delete webhook** requires exact-name confirmation. HA events remain intact;
  delivery rows retain their safe channel-name snapshot even after their channel
  reference is cleared.

Destinations must use HTTPS and cannot contain userinfo or fragments. API/UI
responses expose only scheme and host; path and query token components remain
encrypted and hidden. Avoid putting secrets in channel names.

Operational History shows one separate delivery row per webhook destination.
`Delivered` means the endpoint returned 2xx. `Failed` is terminal after the
existing five-attempt retry policy, `Pending` includes queued or retrying work,
and `Suppressed` means an expected maintenance DNS failure was deliberately not
sent. Previous/Next reads are server-paginated at 50 rows. New channels receive
future state transitions only; toggle the underlying condition healthy and
degraded again in a safe test environment when validating a real alert.

## Revision and deployment lifecycle

Archive is the normal cleanup mechanism. It removes historical records from the
default list without changing immutable content or severing relationships.
Archived state is audited, restorable, and included in both Standard and Full
portable control-plane recovery.

Hard deletion is exceptional:

- A revision must be inactive, never deployed, and unreferenced by clusters,
  nodes, drafts, deployments, rollback links, or drift records.
- A deployment must be queued, never started, have no touched node task, and be
  unreferenced by drift.

The UI shows eligibility returned by the server, but the mutation locks and
rechecks every reference transactionally. Exact confirmation phrases are
required. Audit events survive because they describe the action without a
database foreign key to the deleted record.

## Backup and Restore

System → Backup & Restore creates Standard or Full passphrase-encrypted portable
archives and performs non-mutating preflight. Standard includes the control
plane, including archive status; Full also includes retained operational
history. Sessions and release caches are excluded.

Actual restore is offline through `atlas-dns-backup restore`, with the controller
stopped and a new empty database. See the [backup procedure](../operations/backup-and-restore.md)
and [format reference](../operations/backup-format.md). Store runtime settings
such as session secret, database URL, public origin, and TLS separately.

## Updates

System → Updates uses cached stable GitHub release metadata and provides
installation-specific instructions. It does not execute commands, perform an
automatic rollback, or access the Docker socket. Treat release text and links as
untrusted display data. Back up and preflight before following host instructions.

## System Settings

System → Settings controls supported persistent controller settings such as
release checks. Collector intervals, retention, database connectivity, secrets,
and listener settings remain deployment configuration unless the UI explicitly
states otherwise. Use Operational Status to verify effective worker behavior.

## About

System → About shows application version, commit, build time, environment,
schema compatibility, project attribution, documentation, and licensing status.
Use these values in a support report, but never include credentials, backup
passphrases, Query Log records, or raw node responses.

## Audit

System → Audit provides append-only action attribution with request IDs and safe
metadata. It covers authentication, users, nodes, configuration publication,
deployments/rollback, drift, maintenance/upgrades, webhook lifecycle/test,
backup/preflight, and lifecycle archive/delete actions. Audit is evidence, not a
substitute for deployment per-node results or webhook delivery history.

## Routine administrative checklist

1. Review Operational Status and unresolved drift.
2. Confirm at least two intended DNS-serving nodes before maintenance.
3. Review certificate/version warnings and collector gaps.
4. Preflight a recent portable backup.
5. Archive stale history deliberately; never treat archive as deletion.
6. Review administrators and webhook enabled state.
7. Apply controller/node upgrades through their host-native mechanism and verify
   return-to-service evidence.

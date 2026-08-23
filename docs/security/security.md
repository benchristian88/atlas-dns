# Security Guide

Atlas DNS Controller holds administrator sessions, node credentials, desired
configuration, query history, and recovery material. Deploy it as a privileged
management system even though it is outside the DNS request path.

## Authentication and authorization

The first-run flow creates the initial local administrator exactly once. All
current accounts have the administrator role. Passwords are hashed; successful
login creates a server-side session represented by a Secure, HTTP-only,
same-site cookie in production. Authentication endpoints are rate-limited.

Disabling an account or resetting credentials revokes its sessions. The server
prevents self-disable and removal of the final enabled administrator. Every
protected operation checks server-side identity and role; hiding a button is not
an authorization control.

## Browser and API protections

- Serve the browser origin over HTTPS and configure `PUBLIC_BASE_URL` exactly.
- State-changing browser requests require a matching, session-bound CSRF
  cookie/header pair in addition to the session. Both cookies use Strict
  SameSite policy; Atlas does not rely on an `Origin` header as its CSRF proof.
- API paths are versioned under `/api/v1`; request IDs support safe correlation.
- External input is validated at the API boundary and machine-readable error
  codes avoid returning internal schema or secret detail.
- Content security and other response headers should be reinforced by the
  trusted TLS reverse proxy where deployment policy requires it.

## Secrets

Node credentials and webhook destinations are encrypted at rest with
`CREDENTIAL_ENCRYPTION_KEY`. They are write-only through API/UI contracts and
must never be logged, echoed in responses, copied into audit metadata, or exposed
through diagnostics. Webhook summaries include only scheme and host; userinfo,
path, fragment, and query-token detail is excluded.

TLS observations retain only public status/certificate metadata. Private keys,
certificate chains where unsafe, file paths, node request bodies, and AdGuard
Home error bodies are discarded at the adapter boundary. Logs use structured
safe codes and controller-owned operation names.

Protect `.env` or `/etc/atlas-dns/atlas-dns.env`, database
credentials, TLS material, metric token, backup passphrases, and recovered
credential keys with least-privilege filesystem access. Rotate compromised
values and invalidate affected sessions/backups.

## Transport and network boundaries

Use HTTPS between administrators and the controller and preferably between the
controller and nodes. Restrict node administration APIs to trusted management
networks. DNS clients communicate directly with AdGuard Home nodes; the
controller does not listen on DNS ports.

The Docker deployment runs unprivileged with a read-only root filesystem and no
Docker socket. The native service uses a dedicated non-login account and a
hardened systemd unit. The controller has no remote shell or package-execution
facility.

## Webhook security

Webhook creation, edit, enable/disable, test, and deletion require an
authenticated administrator and CSRF. Destinations must be HTTPS, cannot contain
userinfo or fragments, and follow no redirects during a test. Editing preserves
the stored destination unless an explicit replacement is requested.

Delivery/test calls are time-bounded. Responses show safe status/error codes,
not destination values or response bodies. Delete uses exact-name confirmation,
is audited, and retains HA events and historical delivery identity.

## Configuration and lifecycle safety

Draft writes use optimistic concurrency. Revision publication and deployments
are distinct explicit actions. Deployment locks/revalidates targets, records
per-node results, and activates only after verified success. Direct changes are
durable drift rather than silently accepted.

Revision/deployment archive and hard deletion require administrator identity,
CSRF, explicit confirmation, audit, and transactional server-side referential
checks. Active/referenced/effectful history cannot be hard-deleted. Archive never
makes immutable records editable.

## Audit

Audit covers authentication, user management, node/credential changes,
configuration publication, deployment/rollback, drift resolution, maintenance,
guided upgrades, webhook lifecycle/test, backup/preflight, update settings, and
revision/deployment archive/delete actions. Events retain actor, action, safe
target metadata, request ID, and time. Secrets and raw Query Log records are not
audit fields.

Audit does not replace immutable revisions, per-node deployment results, drift
events, HA events, or delivery records; each preserves a different kind of
evidence.

## Query Log and Statistics privacy

Query Log can contain domains, client identities, answers, and policy outcomes.
Access is authenticated, source-node attribution is preserved, and central
retention is bounded independently from node-local policy. Anonymized values are
stored as received. Raw events are excluded from routine logs and support
bundles. Disable collection or shorten retention where policy requires it.

Statistics uses normalized aggregate counters and explicit coverage. It does not
derive or expose raw Query Log events.

## Backup and restore

Portable archives combine the database dump and credential key inside a
passphrase-encrypted authenticated envelope. Passphrases must be supplied from a
protected regular file to the CLI, never a command argument. Restore requires a
stopped controller and new empty database and emits the recovered key to an
explicit protected path.

Store archives away from controller temporary workspace, preserve runtime
configuration separately, and preflight backups. Sessions and release caches are
not restored. A Full archive includes sensitive operational history; Standard
still contains credentials and configuration and must be protected equivalently.

## Update boundary

The Updates page displays cached release metadata and host-guided instructions.
The controller neither executes metadata/instructions nor performs an automatic
upgrade. It never mounts the Docker socket. Treat remote release text as
untrusted, verify the selected version independently, and create a preflighted
backup first.

## Incident response

1. Preserve logs, request IDs, audit evidence, affected deployment/delivery
   records, and version metadata without copying secrets or Query Log content.
2. Revoke compromised sessions/accounts and rotate exposed runtime credentials.
3. Disable affected webhooks or place unsafe nodes in maintenance as appropriate.
4. Verify desired versus observed state before enabling Enforce.
5. Restore only from a preflighted archive into a separate empty database.
6. Report suspected vulnerabilities using [SECURITY.md](../../SECURITY.md).

## Known security boundaries

The stable 1.x product is local-administrator-only and does not provide OIDC,
fine-grained RBAC, distributed login throttling, automatic secret rotation,
signed release artifacts, or controller HA. Do not infer those controls from the
current UI.

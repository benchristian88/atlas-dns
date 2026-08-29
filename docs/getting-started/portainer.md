# Install as a Portainer Stack

Portainer uses the same production `compose.yaml` and public GHCR image as the
Docker Compose CLI. There is no Portainer-specific image and no source build.

## Git repository workflow

1. In Portainer, open **Stacks → Add stack → Git repository**.
2. Use `https://github.com/benchristian88/atlas-dns.git`.
3. Select the reviewed release tag, such as `v1.1.0`, and Compose path
   `compose.yaml`.
4. Add every value from `atlas-dns.env.example` in Portainer's environment
   editor. Generate unique `POSTGRES_PASSWORD`, `SESSION_SECRET`, and
   `CREDENTIAL_ENCRYPTION_KEY` values and set `ATLAS_DNS_VERSION=1.1.0`.
5. Deploy the stack and wait for both PostgreSQL and Atlas DNS Controller to be
   healthy.

Portainer Git mode does not automatically consume an uncommitted local `.env`;
enter required variables in the stack environment. Do not put secrets in the
repository or Compose file.

## Web Editor or upload workflow

Download the release's `compose.yaml`, paste or upload it into a new stack, add
the same environment values, and deploy. Variable substitution and named-volume
behavior are otherwise the same as the CLI workflow.

## Verify and persist

Open the published port, check `/health` and `/ready`, create the initial
administrator, and inspect the container health in Portainer. The
`postgres-data` and `atlas-dns-work` named volumes survive ordinary stack
redeploys. Do not enable a Docker socket mount or privileged mode.

## Update

Create and preflight a backup, review release notes, update the Git reference and
`ATLAS_DNS_VERSION` to the same exact release, enable image re-pull, and redeploy.
Confirm the image digest/version in About and verify readiness, nodes, and
collectors. Database rollback follows the same forward-only boundary as the
[Docker guide](docker.md).

For v1.0.x to v1.1.0, preserve the stack environment through the first startup
so deprecated runtime values seed PostgreSQL once. Verify System Settings, then
remove only the deprecated runtime entries. Do not delete or recreate
`postgres-data`, `atlas-dns-work`, or secrets.

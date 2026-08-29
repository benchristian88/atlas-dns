# Runtime settings and environment ownership

Atlas DNS Controller 1.1 stores normal operational policy in PostgreSQL and
manages it through **System → System Settings**. These values are audited,
validated, included in Standard and Full backups, and adopted without a process
restart.

| Setting | Recommended | Valid range | Restart semantics |
|---|---:|---:|---|
| Session duration | 12 hours | 15 minutes–30 days | New sessions only; existing expiry timestamps are unchanged. |
| Node health interval | 30 seconds | 5 seconds–1 hour | Worker timer resets immediately. |
| Node request timeout | 10 seconds | 1 second–2 minutes | New node requests only. |
| Statistics poll interval | 1 hour | 1 minute–24 hours | Worker timer resets immediately. |
| Query Log collection | Enabled | Boolean | Collection starts/stops immediately; retained rows remain. |
| Query Log poll interval | 30 seconds | 5 seconds–1 hour | Worker timer resets immediately. |
| Query Log retention | 7 days | 1 hour–90 days | Next cleanup pass uses the new value. |
| Log level | Info | debug/info/warn/error | Logger threshold changes immediately. |
| Operational History retention | 90 days | 7/14/30/90/180/365 days | Next bounded cleanup pass uses the new value. |

During the first 1.1 startup only, an uninitialized database imports the legacy
v1.0.x variables `SESSION_DURATION`, `NODE_HEALTH_INTERVAL`,
`NODE_REQUEST_TIMEOUT`, `STATISTICS_POLL_INTERVAL`,
`QUERY_LOG_COLLECTION_ENABLED`, `QUERY_LOG_POLL_INTERVAL`,
`QUERY_LOG_RETENTION`, and `LOG_LEVEL`. Invalid values use recommended defaults.
After initialization PostgreSQL is authoritative; changing, removing, or
leaving stale legacy variables has no effect.

## Complete environment classification

This inventory covers every Atlas-defined runtime, deployment, build, backup,
and integration-test environment interface. Toolchain scratch variables set
internally by the Dockerfile are included so they are not mistaken for product
settings.

| Variable | Current purpose | Secret? | Needed before DB? | v1.1 destination | Runtime mutable? | Migration/deprecation |
|---|---|---:|---:|---|---:|---|
| `APP_ENV` | Process mode | No | Yes | External bootstrap | No | Retained |
| `HTTP_ADDR` | HTTP listener binding | No | Yes | External infrastructure | No | Retained |
| `DATABASE_URL` | Controller/migrator PostgreSQL connection | Yes | Yes | External bootstrap | No | Retained |
| `PUBLIC_BASE_URL` | Trusted browser origin and cookie/CSRF boundary | No | Yes | External security bootstrap | No | Retained; restart to change |
| `SESSION_SECRET` | Session signing | Yes | Yes | External secret | No | Retained; excluded from DB backups |
| `CREDENTIAL_ENCRYPTION_KEY` | Node-credential envelope encryption | Yes | Yes | External secret | No | Retained; only encrypted backup envelope handling applies |
| `AUTO_MIGRATE` | Startup migration policy | No | Yes | External bootstrap | No | Retained |
| `METRICS_BEARER_TOKEN` | Optional metrics authentication | Yes | Yes | External secret | No | Retained |
| `INSTALLATION_TYPE` | Deployment identity/update guidance | No | Yes | External deployment metadata | No | Retained |
| `WEB_DIST_DIR` | Packaged frontend location | No | Yes | External infrastructure | No | Retained |
| `PG_DUMP_PATH` | Backup tool location | No | No | External host tooling | No | Retained |
| `PG_RESTORE_PATH` | Restore tool location | No | No | External host tooling | No | Retained |
| `SESSION_DURATION` | Legacy session lifetime | No | No | DB System Setting | Yes, through UI/API | First-uninitialized-DB seed only in 1.1; then ignored |
| `NODE_HEALTH_INTERVAL` | Legacy health poll interval | No | No | DB System Setting | Yes, through UI/API | First-uninitialized-DB seed only in 1.1; then ignored |
| `NODE_REQUEST_TIMEOUT` | Legacy AdGuard request timeout | No | No | DB System Setting | Yes, through UI/API | First-uninitialized-DB seed only in 1.1; then ignored |
| `STATISTICS_POLL_INTERVAL` | Legacy statistics schedule | No | No | DB System Setting | Yes, through UI/API | First-uninitialized-DB seed only in 1.1; then ignored |
| `QUERY_LOG_COLLECTION_ENABLED` | Legacy query-log switch | No | No | DB System Setting | Yes, through UI/API | First-uninitialized-DB seed only in 1.1; then ignored |
| `QUERY_LOG_POLL_INTERVAL` | Legacy query-log schedule | No | No | DB System Setting | Yes, through UI/API | First-uninitialized-DB seed only in 1.1; then ignored |
| `QUERY_LOG_RETENTION` | Legacy query-log retention | No | No | DB System Setting | Yes, through UI/API | First-uninitialized-DB seed only in 1.1; then ignored |
| `LOG_LEVEL` | Legacy logging threshold | No | No | DB System Setting | Yes, through UI/API | First-uninitialized-DB seed only in 1.1; then ignored |
| `POSTGRES_DB` | Managed PostgreSQL database name | No | Yes | Compose/installer infrastructure | No | Retained |
| `POSTGRES_USER` | Managed PostgreSQL role | No | Yes | Compose/installer infrastructure | No | Retained |
| `POSTGRES_PASSWORD` | Managed PostgreSQL role password | Yes | Yes | External deployment secret | No | Retained |
| `ATLAS_DNS_VERSION` | Exact image/release/artifact version | No | Yes | External deployment/release input | No | Retained |
| `ATLAS_DNS_BIND_ADDRESS` | Published container bind address | No | Yes | External infrastructure | No | Retained |
| `ATLAS_DNS_PORT` | Published container port | No | Yes | External infrastructure | No | Retained |
| `ATLAS_DNS_COMMIT` | Release-artifact commit identity | No | Build only | Release pipeline input | No | Retained |
| `ATLAS_DNS_BUILT_AT` | Release-artifact build timestamp | No | Build only | Release pipeline input | No | Retained |
| `VERSION` | Make/Docker version metadata | No | Build only | Build input | No | Retained |
| `COMMIT` | Make/Docker commit metadata | No | Build only | Build input | No | Retained |
| `BUILT_AT` | Make/Docker build timestamp | No | Build only | Build input | No | Retained |
| `GO` | Go executable selected by Make/release scripts | No | Build only | Build tooling | No | Retained |
| `CGO_ENABLED` | Static release-build control | No | Build only | Build tooling | No | Internally set for release builds |
| `GOOS` | Release target operating system | No | Build only | Build tooling | No | Internally set for release builds |
| `GOARCH` | Release target architecture | No | Build only | Build tooling | No | Internally set for release builds |
| `GOCACHE` | Docker build cache location | No | Build only | Build scratch state | No | Dockerfile-internal |
| `GOTMPDIR` | Docker Go temporary directory | No | Build only | Build scratch state | No | Dockerfile-internal |
| `TMPDIR` | Runtime temporary-work directory | No | Yes | Container infrastructure | No | Image default retained |
| `PGPASSWORD` | Password passed only to a `pg_dump`/`pg_restore` child | Yes | No | Ephemeral child-process environment | No | Rebuilt from parsed DB URL; stale inherited values removed |
| `TEST_DATABASE_URL` | PostgreSQL integration-test connection | Yes | Test only | Test infrastructure | No | Retained; never runtime configuration |
| `TEST_NODE_A_URL` | Optional live AdGuard test node A | No | Test only | Test infrastructure | No | Retained |
| `TEST_NODE_B_URL` | Optional live AdGuard test node B | No | Test only | Test infrastructure | No | Retained |
| `TEST_NODE_USERNAME` | Optional live AdGuard test username | Yes | Test only | Test secret | No | Retained |
| `TEST_NODE_PASSWORD` | Optional live AdGuard test password | Yes | Test only | Test secret | No | Retained |

`PUBLIC_BASE_URL` cannot move into System Settings: it establishes the trusted
origin and cookie/CSRF posture required to authenticate the very request that
would edit it. Changing an external value requires a process/container restart.

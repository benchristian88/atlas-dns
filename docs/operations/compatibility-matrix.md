# Compatibility Matrix

**Tested** means recorded automated or operator release evidence. **Supported**
means part of the 1.x contract with release-gate coverage. **Best effort** is not
a compatibility commitment. **Unsupported** must not be advertised as working.

| Area | Version/platform | Status | Boundary |
|---|---|---|---|
| AdGuard Home | Earlier than v0.107.78 | Unsupported | Status can be identified, but onboarding, configuration inventory, and every managed write are blocked. |
| AdGuard Home | v0.107.78 and v0.107.79 | Tested and supported | Schema v2; node observation, Query Log, Statistics, TLS applicability, configuration adapters, and rolling mixed-patch operation are contract-tested. |
| AdGuard Home | Later compatible 0.x and valid v1.x, including prereleases | Provisionally compatible | Atlas attempts normal typed capability/API validation and operates when it succeeds; capability-specific incompatibility fails safely. |
| AdGuard Home | Future major (2.x+) or malformed version | Unknown | Inventory and managed writes are blocked pending review; unknown is not reported as unsupported evidence. |
| PostgreSQL | 17 | Tested and supported | Matching PostgreSQL 17 `pg_dump`/`pg_restore` required. |
| PostgreSQL | Other majors | Unsupported | No schema or backup claim. |
| Native | Debian 13 with systemd, amd64/arm64 | Supported release target; RC gate pending | Prebuilt release archive; no build toolchain. Do not mark 1.0 final until clean-host evidence is recorded. |
| Container | Linux amd64/arm64 | Supported release target; publication pending | One public multi-platform GHCR image; anonymous pull and manifest are final external gates. |
| Docker | Maintained Docker Engine with Compose v2 | Supported release target; RC gate pending | Production Compose pulls; no socket or privileged mode. |
| Portainer | Stack using repository `compose.yaml` | Supported release target; RC gate pending | Same GHCR image/configuration; environment entered in Portainer. |
| Browser | Current Chromium desktop/mobile | Tested and supported | Automated accessibility/DOM and packaged responsive baseline. |
| Browser | Current Firefox and Safari/iOS | Supported release target; external gate pending | Automated DOM checks do not substitute for packaged browser evidence. |
| PWA | Current Chromium/Android and Safari/iOS Add to Home Screen | Supported metadata; external gate pending | Standalone metadata/icons; no offline service worker. |
| Upgrade | 1.0.x → later documented 1.x | Supported policy | Backup first; ordered forward migrations and release notes. |
| Upgrade | 1.0.0 → 1.0.1 | Supported, schema-neutral | Preserve environment/volumes; the v1.0.0 migration ledger is accepted unchanged. |
| Upgrade | 1.0.1 → 1.0.2 | Supported, forward migration | Back up first; startup applies `000015` for notification delivery diagnostics/history indexing. |
| Upgrade | 1.0.2 → 1.1.0 | Supported, forward migration | Back up first; startup applies `000016` through `000019`, imports legacy runtime values once, retains immutable schema-1 records through a non-deployable read conversion, preserves webhook channels, adds stable Audit Log keyset paging, and adds optional per-user MFA state. |
| Upgrade | 1.1.0 → 1.1.1 | Supported, schema-neutral | No new migrations or canonical schema changes; refresh/import encrypted-DNS nodes after updating. |
| Upgrade | Any pre-1.0 installation → 1.0 | Unsupported in place | Destroy/rebuild and fresh Atlas installation. |
| Backup | Atlas backup format v1 within compatible 1.x schema | Supported | Newer application/schema inputs fail closed. |
| Backup | Pre-1.0 `.aghhabackup`/`AGHHABACKUP` | Unsupported | Not interpreted as Atlas backup v1. |
| Manual release archive | Published Linux amd64/arm64 bundle | Supported advanced path | Operator owns process supervision/configuration. |
| Source/custom build | Any | Best effort | Contributor workflow, not a production installation promise. |

This matrix describes Atlas DNS Controller compatibility, not endorsement or
support by AdGuard Software, Docker, Portainer, PostgreSQL, or browser vendors.

Atlas uses AdGuard Home's legacy `/control` API generation. It accepts stable
v0.107.78 and later compatible 0.x releases and all syntactically valid v1.x
versions, subject to normal API response and capability validation. A prerelease
of the minimum `v0.107.78` remains below that stable minimum. v0.107.78 and
v0.107.79 are release-tested; representative v1 beta/stable fixtures add
contract regression coverage, not real-device qualification. There is no upper
minor/patch limit within 1.x. Future major (2.x+) or malformed versions remain
unknown and fail closed. The unfinished AdGuard `/api/v1` API is not used.

For Atlas v1.1.1, live validation on AdGuard Home v0.107.79 confirmed observation
and configuration import with plain DNS on port 53 and DoH/DoT/DoQ enabled.
The node used HTTPS hostname management with a valid wildcard TLS certificate,
while a second node used a direct IP-based management address; both were
simultaneously healthy and manageable in the same controller.
`NodeSpecific.BindHosts` retained only plain listener addresses and excluded
encrypted endpoint URIs from `/control/status.dns_addresses`. See the
[sanitised live validation evidence](release-1.1.1.md#live-encrypted-dns-validation).
This real-world test used v0.107.79, not AdGuard Home v1.x.

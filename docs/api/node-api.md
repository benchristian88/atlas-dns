# AdGuard Home Node API Adapter

## Query Log reads

For stable AdGuard Home v0.107.78 and later compatible 0.x releases and valid v1.x,
Atlas DNS Controller reads
`GET /control/querylog` with `limit` (maximum 500), empty `search`,
`response_status=all`, and the previous response's `oldest` value as
`older_than`. Results are newest-first. Offset exists in portions of the
upstream contract but is intentionally not used because offsets shift while a
live log receives new records. The source supplies timestamps and a timestamp
cursor, not a stable event ID.

The adapter accepts `question.name`/`question.host`, `client`, `client_id`, optional `client_info.name`,
`client_proto`, `elapsedMs` number/string, `status`, `reason`, upstream, answer,
rule/rules/filter ID, service, cache, and DNSSEC fields. It bounds each record
to 64 KiB and normalizes only controller-domain values.
`GET /control/querylog/config` supplies enabled/anonymisation state.

The source can repeat records across overlapping windows, discard history due
to node policy or clear, reset after restart, and cannot distinguish completely
identical events with an ID. Atlas DNS Controller documents and exposes those limitations via
checkpoint/gap coverage; it does not log raw payloads or attempt to reverse
client anonymisation.

## Adapter purpose

The adapter is the only package that consumes raw AdGuard Home HTTP payloads.
Its base discovery operation is a read-only status probe at:

```text
GET {baseUrl}/control/status
```

The adapter uses HTTP Basic authentication, requires a bounded request timeout, caps response bodies at 1 MiB, rejects redirects, and returns only the stable domain result:

```go
type NodeProbeResult struct {
    Version                      string
    Compatibility                Compatibility
    Running                      bool
    ProtectionEnabled            bool
    ProtectionDisabledDurationMS int64
    LatencyMS                    int
}
```

Raw payloads and authentication values do not cross the adapter boundary.

## Transport trust

Each node selects one explicit policy:

- `system`: HTTPS using the host system trust store;
- `custom_ca`: HTTPS using system roots plus a stored node-specific private CA;
- `insecure_http`: plaintext HTTP, visibly discouraged and valid only for an `http` URL.

There is no option that skips TLS certificate or hostname verification. Node requests connect directly and do not use ambient proxy configuration, avoiding accidental credential forwarding to an HTTP proxy.

## Compatibility

The minimum managed version is stable v0.107.78. Compatible later 0.x and all
valid v1.x product versions, including prereleases, use the known
`legacy_control` API generation. The central `internal/adguardcompat` policy
validates semantic versions (optional `v`, prerelease and build metadata), keeps
prereleases of the stable minimum below it, and leaves malformed or future-major
(2.x+) versions unknown. There is no maximum 1.x minor/patch. v0.107.78 and
v0.107.79 remain release-tested; later versions are provisionally compatible and
must pass normal typed endpoint and semantic checks. AdGuard's draft `/api/v1`
adapter is not implemented.

Contract fixtures cover the existing legacy versions and representative
synthetic v1 beta/stable status, DNS, filtering, clients, rewrites, blocked
services, Query Log/Statistics policy and TLS responses. The full-flow regression
also exercises observation, snapshot and audited import; this is not real-node
release qualification.

## Configuration contract

Configuration compatibility begins at v0.107.78 and uses schema v2 only. The adapter reads status, DNS, filtering, persistent clients, rewrites, blocked services, safety/Safe Search, query-log policy, statistics policy, TLS status, and optional DHCP status. Feature flags record each successfully observed area; missing required features block preview before mutation.

| AdGuard Home version | Configuration schema | Managed boundary |
|---|---:|---|
| Earlier than v0.107.78 | Unsupported | Status remains identifiable, but onboarding, configuration inventory, and deployment are blocked with the required-version message |
| v0.107.78–v0.107.79 | 2 | Supported and explicitly release-tested |
| Later compatible 0.x or valid v1.x | 2 | Provisionally compatible after complete typed observation; a failed required contract blocks writes |
| Future major (2.x+) or malformed version | Unknown | Inventory/deployment blocked pending explicit review |

The compatibility boundary is contract-tested at the legacy minimum, higher
0.x, v1 alpha/beta/RC/stable/later minor releases, and unknown future majors.
A node can report DHCP unavailable
(including platforms where AdGuard Home returns its documented not-implemented
status); the capability remains false and DHCP cannot enter that node's desired
override.

`GET /control/status` is decoded with the runtime field
`protection_disabled_duration`; `GET /control/dns_info` separately uses
`protection_disabled_until`. Official v0.107.78 and v0.107.79 source emit the
same runtime names; the v0.107.79 changelog corrected OpenAPI documentation.
Atlas validates non-negative, non-contradictory pause state and keeps the pause
deadline observed-only while preserving effective protection as managed state.

The writer uses the documented `/control/dns_config`, `/control/filtering/*`, `/control/clients/*`, `/control/rewrite/*`, `/control/blocked_services/update`, safety enable/disable, `/control/safesearch/settings`, `/control/querylog/config/update`, `/control/stats/config/update`, and `/control/dhcp/*` endpoints. Query-log/statistics updates use `PUT`; existing collections are reconciled rather than blindly duplicated. `/control/filtering/refresh` is exposed as a separate audited operation.

`GET /control/blocked_services/all` supplies observed catalogue metadata.
The supported contract returns `blocked_services` entries with `id`, `name`,
`rules`, Base64 `icon_svg`, and optional `group_id` and
the response adds `groups: [{"id": "..."}]`. The adapter accepts both
contracts, validates stable IDs/names/groups, and returns only ID, name, and
optional group ID. It does not retain rules or icon data, and it never uses
deprecated `/blocked_services/services`, `/list`, or `/set` endpoints.

Two node-specific safety reads support DHCP workflows. Interface discovery
uses `GET /control/dhcp/interfaces`. The v0.107.78 and v0.107.79 response is an object keyed
by interface name whose values contain `name`, `hardware_address`,
`ipv4_addresses`, `ipv6_addresses`, `gateway_ip`, and pipe-delimited `flags`.
The adapter returns only those safe values and derives availability in the
controller; the metadata never enters configuration or drift.

Active-DHCP detection uses `POST /control/dhcp/find_active_dhcp` with the exact
body `{ "interface": "eth0" }`. The response contains
`v4.other_server.found`, `v4.static_ip.static`, optional
`v4.static_ip.ip`, and `v6.other_server.found`; protocol result values are
`yes`, `no`, or `error`. Missing protocol data is reported as unavailable.
AdGuard `error` strings and response bodies are never returned, logged, or
stored. Although AdGuard exposes this read-only check as POST, the controller
does not treat it as a configuration mutation.

## Listener identity

`/control/status.dns_addresses` may mix bare IPs and `https://`, `tls://` or
`quic://` endpoints. The adapter trims and parses entries, requires a valid DNS
port and at least one plain IP, and rejects malformed or unknown metadata.
Only canonical bare IPs enter `NodeSpecific.BindHosts`; encrypted URIs are
informational and never fetched, stored as bind hosts, or made editable.
Existing protection semantics remain mandatory. The extraction preserves source
order; normal canonicalization still sorts/deduplicates bind hosts. TLS mutation
remains inventory-only and no schema/database expansion is introduced.

## Statistics contract

The adapter reads `GET /control/stats/config` and
`GET /control/stats?recent={milliseconds}` for eligible versions of the legacy
control API, including later compatible 0.x and valid v1.x. It requests
whole-hour fixed ranges for 24 hours, 7 days, and 30 days only when they do not
exceed that node's configured interval. Versions below stable v0.107.78 are
unsupported; unknown versions do not claim exact-range support. No range is
approximated. A fixed range beyond node retention maps to
`STATISTICS_RANGE_EXCEEDS_NODE_RETENTION` without making the eligible collector
pass fail.

The response boundary accepts `hours` or `days`, non-negative additive totals,
a finite non-negative average processing time, up to 1,000 equal-length
non-negative series points, and up to 100 one-key ranked entries per panel.
Invalid, mismatched, oversized, negative, empty-key, NaN, or infinite data maps
to a safe node-response error. The adapter returns a normalized typed snapshot;
raw JSON and authentication data do not cross into storage.

TLS parsing deliberately has no fields for `certificate_chain`, `private_key`, `certificate_path`, or `private_key_path`. Only public status, subject/issuer, validity, DNS names, ports, and safe warning text cross the adapter boundary. Certificate applicability uses the capability contract rather than an exact patch allowlist: both tested v0.107.78 and v0.107.79 responses use `enabled=false` for intentionally unused TLS, and their zero/default certificate timestamps are retained only as observation metadata, never interpreted as an expiry. DHCP dynamic leases are observed-only; configuration/static leases are node-specific managed state.

## Error mapping

The adapter distinguishes:

- `NODE_UNREACHABLE` for bounded network and timeout failures;
- `NODE_TLS_FAILED` for certificate, hostname, or TLS handshake failures;
- `NODE_AUTHENTICATION_FAILED` for HTTP 401 or 403;
- `CAPABILITY_ERROR` for a proven missing/not-implemented capability endpoint;
- `NODE_INVALID_RESPONSE` for other status codes, oversized bodies, malformed
  payloads, or missing/contradictory required semantics.

Safe diagnostics include node ID, reported AdGuard version, method, endpoint,
HTTP status/content type, and decode/semantic detail; request handling adds the
controller request ID. Messages never include credentials or a node response
body.

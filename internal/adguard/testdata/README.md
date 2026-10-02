# AdGuard Home contract fixtures

The v0.107.78/v0.107.79 fixtures preserve the existing reviewed control
contracts. `v1.0.0-b.1` and `v1.0.0` contain representative synthetic responses,
checked against the official
[v1.0.0-b.1 OpenAPI contract](https://github.com/AdguardTeam/AdGuardHome/blob/v1.0.0-b.1/openapi/openapi.yaml).
The stable v1 directory is forward compatibility regression coverage, not a
capture from a stable v1 device. No live-node qualification is implied.

The tagged specification still declares `/control` as its server URL. These
fixtures cover status, DNS info, filtering, clients, rewrites/settings, blocked
services, safety, Query Log configuration/data, Statistics configuration/data,
TLS status and DHCP. Status includes the reported plain IP plus DoH/DoT/DoQ
endpoints. TLS secret placeholders test redaction; they are not credentials.

The full flow test uses the real probe/reader/inventory service with HTTP and
storage test boundaries and proves that observation and audited import retain
only the plain listener identity. Existing transport and semantic failure
tests remain applicable to these eligible product generations.

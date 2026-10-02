# Atlas DNS Controller 1.1.1

This focused compatibility/correctness patch follows v1.1.0. It adds no database
migration, public Atlas endpoint change, or canonical configuration schema
change. The controller remains outside the live DNS request path.

## Maintenance / runtime update

The preferred build compiler/runtime is now Go 1.27.1. This maintenance/security
runtime update is independent of the AdGuard compatibility fixes. Go 1.27.1
contains compiler/runtime and standard-library corrections; see the official
[Go release history](https://go.dev/doc/devel/release#go1.27.1).

The module retains `go 1.27.0` as its minimum and adds `toolchain go1.27.1`.
This selects the patched toolchain for development under normal Go automatic
selection without changing the language family or introducing new language
features. CI uses `go-version: 1.27.1`; the Docker builder uses
`golang:1.27.1-bookworm`. Native build/release scripts inherit the module's
selected toolchain. Older historical audit records retain their recorded
versions. Module cleanup and verification results are recorded in the
[implementation report](../engineering/release-1.1.1-implementation.md).

## Fixed

AdGuard Home nodes using DNS-over-HTTPS, DNS-over-TLS, or DNS-over-QUIC can now
be observed and imported when `/control/status.dns_addresses` contains encrypted
endpoint URIs. Atlas recognizes `https`, `tls`, and `quic` URIs as informational
metadata. It never follows or fetches those URLs. Only trimmed, canonical bare
IPv4/IPv6 addresses enter node-specific `bindHosts`, in the source order before
the existing canonical sorting/deduplication.

Invalid ports, empty lists, missing plain IP identity, malformed/unknown address
entries, and missing/contradictory protection state still fail with
`NODE_INVALID_RESPONSE`. TLS mutation remains unsupported/inventory-only;
this patch adds no editable DoH/DoT/DoQ configuration fields or endpoint storage.

## Compatibility

Atlas uses AdGuard Home's legacy `/control` API generation. It supports stable
v0.107.78 and later compatible 0.x releases and the AdGuard Home v1.x generation,
including valid alpha/beta/RC versions, subject to normal API response and
capability validation. There is no upper 1.x minor/patch bound. Future major
versions (2.x+) and malformed versions remain compatibility unknown; versions
below the stable minimum remain unsupported, including `v0.107.78-rc.1`.

Central semantic-version parsing accepts an optional `v`, validates numeric
components, prerelease identifiers and build metadata, and rejects malformed
identifiers, leading zeros and extra components. Canonical decimal comparisons
avoid integer overflow and an artificial minor/patch ceiling. Prereleases
of a higher accepted product release are eligible; prereleases of the stable
minimum are below it. Query Log and exact recent Statistics share this policy,
so 0.108.x and 1.x no longer produce unknown capability solely from their version.

Product version and API generation are distinct in `internal/adguardcompat`:
`unknown` or `legacy_control`. Existing typed JSON/semantic/HTTP checks, Basic
Auth, TLS trust, redirect restrictions, timeouts and response bounds remain
active. The unfinished AdGuard `/api/v1` API is not used or implemented.

## Upgrade

Create/preflight a backup, then update from v1.1.0 using the documented native
or container workflow, pinned to `1.1.1`. Preserve runtime configuration and
database/volumes. There are no new migrations; canonical schema remains v2.
Refresh affected nodes and import their latest successful snapshot after the
update. Existing revisions remain immutable; a newly imported draft still
requires publication and an explicit deployment to change managed nodes.

Upgrades from v1.0.x retain the migration/runtime requirements in the
[v1.1.0 upgrade notes](release-1.1.0.md).

## Validation boundary

Regression coverage includes the existing plain-DNS legacy fixtures, v1 beta
and stable status/core configuration fixtures, encrypted listener filtering,
invalid response rejection, capability/jobs/onboarding gates, and the real
adapter → observation → snapshot → audited import path with storage faked.
The v1 fixtures are representative synthetic responses based on the reviewed
tagged upstream contract; they do not claim real-device release qualification.

Before publication, exercise real AdGuard v1 beta/stable and legacy encrypted
listeners, packaged install/upgrade and container workflows using the
[release process](../development/release-process.md). A future AdGuard API
generation requires contract review and a separate adapter decision.

Repository commands, results and scope are recorded in the
[implementation report](../engineering/release-1.1.1-implementation.md).

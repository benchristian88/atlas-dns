# Configuration Model

## Objective

Represent AdGuard Home configuration in a stable, revisioned, version-aware model that separates shared cluster policy from node-specific infrastructure details.

## Configuration layers

### Shared cluster configuration

Examples:

- Upstream DNS servers.
- Bootstrap DNS.
- Filtering settings.
- Filter subscriptions.
- Custom filtering rules.
- Allow lists.
- Persistent clients.
- DNS rewrites.
- Blocked services.
- Safe browsing.
- Parental controls.
- Safe search.
- Query-log policy.
- Statistics policy.

### Node-specific overrides

Examples:

- Bind addresses.
- Web administration address.
- TLS certificate and key references.
- Hostnames.
- Interface selection.
- DHCP role and interface.
- Node labels.
- Site metadata.
- Maintenance state.

## Model shape

```yaml
schemaVersion: 1
shared:
  dns:
    upstreamDns:
      - https://dns.quad9.net/dns-query
  filtering:
    enabled: true
  queryLog:
    enabled: true
nodeOverrides:
  node-a-id:
    bindHosts:
      - 192.168.3.10
  node-b-id:
    bindHosts:
      - 192.168.3.11
```

Release 0.2 froze the per-node observed `Document` shape. Release 0.3 added a distinct authoritative `DesiredDocument`: shared DNS/filtering values, `nodeOverrides`, and explicit unsupported areas. Observed-only product version never enters desired state. Schema v1 manages DNS upstream, bootstrap, fallback, and private reverse resolvers plus filtering enablement, interval, blocklist subscription URLs, and custom rules. Bind hosts and DNS port are required node overrides but are verification-only because AdGuard Home exposes no supported writer for them.

Release 0.4 introduces schema v2 without changing schema v1. Shared v2 state adds protection, rate limiting, blocking response behavior, EDNS Client Subnet, DNSSEC, cache and resolver modes; blocklist and allowlist URLs; persistent-client identity/policy/schedules/upstream cache; rewrites; blocked-service schedules; Safe Browsing, parental control, Safe Search; and node-local query-log/statistics policy. Node overrides add DHCP configuration and static leases. Redacted TLS status and dynamic DHCP leases are observed-only. TLS mutation is listed as unsupported.

v0.107.52 observations remain schema v1. v0.107.53 and later patches in the
v0.107 API generation use schema v2. v0.107.78 and v0.107.79 are explicitly
release-tested; a newer v0.107 patch is provisionally compatible only when all
typed endpoint and semantic checks used by the observation succeed. Other API
generations remain unknown. Patch-level capabilities cover upstream timeout
(v0.107.57), cache enablement (v0.107.64), rewrite enablement (v0.107.68),
ignore-list activation (v0.107.72), and arbitrary filter intervals
(v0.107.78). `ProjectDocument` narrows a current observation to a historical
revision schema, and convergence ignores observed-only/unsupported metadata.
The database retains each document's original version and canonical JSON.

AdGuard protection pause is normalized explicitly: `/control/status` supplies
the remaining `protection_disabled_duration`, while `/control/dns_info`
supplies `protection_disabled_until`. The latter is observed-only explanation;
the effective protection state remains the managed semantic value used for
comparison.

The AdGuard adapter reads bind addresses and DNS port from `/control/status` (`dns_addresses` and `dns_port`). `/control/dns_info` supplies the shared DNS parameters and does not own listener identity. An observation with an absent, out-of-range, or malformed listener identity fails, and imports defensively reject older incomplete snapshots.

## Revision lifecycle

- Draft: mutable operator workspace.
- Validated revision: immutable configuration snapshot.
- Deployment: attempt to apply a revision to selected nodes.
- Applied revision: revision verified on a node.
- Superseded revision: previously active revision.
- Rolled-back revision: a previous revision redeployed as a new deployment.

Revisions are never edited after creation.

## Canonicalisation

Canonicalisation must:

- Sort unordered collections.
- Preserve ordered collections where order changes behaviour.
- Strip API-generated timestamps and counters.
- Normalise null and empty representations.
- Normalise IP addresses and CIDR notation.
- Normalise domain casing where semantics allow.
- Preserve comments and operator labels separately from deployable state.
- Produce deterministic serialisation.

Schema v1 preserves order for upstream resolvers, fallback resolvers, and custom filtering rules. It treats bootstrap resolvers, private reverse resolvers, bind hosts, and enabled filter URLs as unordered sets. Runtime cache counters, filter IDs, filter display names, rule counts, and last-update timestamps are discarded at the adapter boundary.

Schema v2 additionally treats allowlists, clients, rewrites, blocked-service IDs, ignore lists, TLS DNS names, and DHCP static leases as sets with deterministic ordering. Upstream/fallback/client-upstream lists and custom rules retain order. Certificate/key material, filesystem paths, dynamic lease expiry state, and filter refresh results never enter desired state.

## Configuration ownership

Each field should eventually be classified as:

- Controller-managed.
- Node-specific managed.
- Observed only.
- Unsupported.
- Ignored.
- Secret reference.

## Import behaviour

On first node onboarding:

1. Read supported configuration.
2. Normalise it.
3. Present import summary.
4. Require explicit operator acceptance.
5. Create or update the cluster's optimistic configuration draft.
6. Compare additional nodes against that draft or source snapshot.

Release 0.3 validates and publishes the draft as an immutable numbered revision. Publication does not activate it. A revision becomes the cluster's `activeRevisionId` only after its durable deployment verifies every target. Draft updates and publication both use optimistic draft versions.

## Release 0.3 deployment boundary

The configuration adapter writes only the supported `/control/dns_config`, `/control/filtering/config`, `/control/filtering/add_url`, `/control/filtering/set_url`, and `/control/filtering/set_rules` contracts. It reads the result back through the observation adapter and compares canonical semantic values. All targets are freshly observed and capability-validated before the first write. A listener override difference blocks the whole deployment rather than editing node files.

The controller must not overwrite a newly added node before showing the differences.

## Release 0.4 deployment boundary

Schema-v2 deployment extends the supported API writer through existing sequential tasks. Preview requires `dns`, `filtering`, `clients`, `rewrites`, `blocked_services`, `safety`, `query_log`, `statistics`, and `tls` capability flags, plus `dhcp` where a node override models DHCP. Safe Search with Ecosia requires its explicit version capability. The writer reconciles set-like collections, applies shared policy, writes DNS, applies DHCP last, and verifies only managed semantic fields from a fresh observation.

`ValidateDesired` requires a listener override for every enabled node, validates schema-v2 ranges and schedules, and permits DHCP enablement on at most one node. A role handoff orders desired-disabled DHCP nodes before the desired-enabled node. DNS listener identity and TLS remain non-writable.

`nodeOverrides` is therefore a required per-enabled-node desired-state map, not
a sparse list of deviations from cluster defaults. An empty override is not a
valid substitute because bind hosts and DNS port are node identity used during
read-back verification. A newly added or newly enabled node must first have a
successful capability/configuration observation explicitly imported into the
mutable draft. Disabling a node retains its override so the same identity can
be re-enabled deliberately. Deleting a node atomically removes only its mutable
draft key and increments the draft version; immutable revisions and historical
node attribution keep the deleted UUID. Re-adding the same endpoint creates a
new UUID and requires refresh, import, validation, and publication of a new
revision before deployment. Atlas never copies an old override by URL.

## Adoption behaviour

When drift is detected, manual adoption should:

1. Show a structured difference.
2. Identify whether the changed field is shared or node-specific.
3. Validate the changed value.
4. Create a new draft.
5. Require an operator to save a revision.
6. Never mutate desired state silently.

# Guided Onboarding

After the first local administrator signs in, Atlas offers guided onboarding
when the selected cluster has not completed core setup. The wizard is also
available at `/onboarding`; leaving it does not roll back valid work, and the
Setup Guide links back to the same current state.

## State and sequence

Onboarding derives progress from the normal cluster, node, observation,
capability, draft, revision, System Settings, and notification records. It does
not keep wizard copies of credentials or configuration. The only onboarding
record contains deliberate acknowledgements (single-node operation,
monitoring reviewed, notifications skipped) and the audited completion event.

The sequence is:

1. welcome and controller/cluster identity review;
2. connection-test and registration of the first compatible AdGuard Home node;
3. a second node, or an explicit acknowledgement that one node is not redundant;
4. topology, API/DNS, capability, and schema-v2 observation validation;
5. explicit selection of the initial configuration source;
6. monitoring settings review;
7. encrypted webhook configuration and test, or deliberate skip;
8. review, audited completion, and an initial dashboard collection pass.

Atlas resumes at the first unmet requirement. Existing nodes are shown instead
of recreated, an existing schema-v2 revision satisfies the authoritative-state
step, and configured webhooks are represented only by safe metadata. Once
completion is audited, transient health or collector failures remain visible as
operational issues but do not trap the administrator back in first-run setup.

## Node compatibility

Guided onboarding requires AdGuard Home v0.107.78 or later in the v0.107 API
generation. v0.107.78 and v0.107.79 are explicitly tested. Newer v0.107 patches
must pass the same typed endpoint, semantic, capability, and schema checks.
Older already-managed nodes retain the broader Atlas compatibility contract;
the stricter floor applies to establishing a new v1.1 installation.

Node candidate validation applies the normal URL/SSRF, TLS trust, credential,
bounded-response, and status validation before credentials are encrypted and
stored. Unsupported, unknown, unreachable, unauthenticated, or non-serving
nodes cannot satisfy the onboarding topology.

## Establishing authority

Onboarding never chooses the first node automatically. Select **Import from**
the intended node. When two configurations are available, **Review differences
first** is required before publication even when the normalized documents are
equal.

The selected observation enters the normal workflow:

```text
schema-v2 observation → desired-state draft → capability validation
→ immutable revision
```

Publishing the initial revision establishes authoritative configuration but
does not deploy it. Review and deploy that exact revision separately through
Configuration Control and Deployments.

## Monitoring and notifications

System Settings persist and audit node-health cadence, Statistics cadence,
Query Log collection, Query Log cadence, and Query Log retention. Existing
environment values initialize the database once during a v1.1 upgrade; later
UI changes are adopted by collector scheduling without a controller restart.
Recommended defaults are 30 seconds, one hour, enabled, 30 seconds, and seven
days respectively.

Finishing onboarding immediately runs the normal node-health, DNS-health,
Statistics, and Query Log collectors for the selected cluster. The collectors
run concurrently with a 20-second overall limit before the completed screen is
shown, so the Dashboard starts with current durable evidence instead of waiting
for the next configured interval. A failed or disabled source is recorded using
its normal operational state; it does not undo the audited completion, and the
scheduled collectors continue from the configured cadence.

Webhook destinations remain encrypted and write-only. Choose one or more of
DNS, redundancy, certificate, version, maintenance, and upgrade event
categories, then send the bounded Test Webhook. Notifications are optional and
may be skipped.

## Failure and security behavior

Every onboarding API is administrator-only. Browser mutations require the
normal session-bound CSRF token and optimistic record version; progress and
completion are audited. Initial collection is read-only against the nodes and
cannot change their DNS or managed configuration. Node credentials and webhook
URL path/query values are never returned. A failed later step leaves prior
cluster, node, observation, draft, or revision work intact so the operator can
exit, correct the issue, and resume safely.

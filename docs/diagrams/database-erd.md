# Database ERD

This is a compact relationship view of the implemented stable schema. The
[database design](../database/database-design.md) and append-only migrations are
authoritative for columns, constraints, and retention detail.

```mermaid
erDiagram
    USERS ||--o{ SESSIONS : has
    USERS ||--o| USER_MFA : secures
    USERS ||--o{ USER_MFA_RECOVERY_CODES : recovers_with
    USERS ||--o{ MFA_CHALLENGES : verifies
    USERS ||--o{ CONFIGURATION_REVISIONS : creates
    USERS ||--o{ AUDIT_EVENTS : acts_in
    CLUSTERS ||--o{ NODES : contains
    CLUSTERS ||--o| CONFIGURATION_DRAFTS : owns
    CLUSTERS ||--o{ CONFIGURATION_REVISIONS : owns
    CLUSTERS ||--o{ DEPLOYMENTS : runs
    CLUSTERS ||--o{ QUERY_EVENTS : retains
    CLUSTERS ||--o{ STATISTICS_SNAPSHOTS : aggregates
    CLUSTERS ||--o{ HA_OPERATIONAL_EVENTS : records
    CLUSTERS ||--o{ NOTIFICATION_CHANNELS : configures
    CONFIGURATION_REVISIONS ||--o{ DEPLOYMENTS : deployed_as
    DEPLOYMENTS ||--o{ DEPLOYMENT_NODES : targets
    NODES ||--o{ DEPLOYMENT_NODES : receives
    NODES ||--o{ OBSERVED_SNAPSHOTS : produces
    NODES ||--o| NODE_CAPABILITY_PROFILES : has
    NODES ||--o{ DRIFT_EVENTS : experiences
    NODES ||--o{ QUERY_EVENTS : sources
    NODES ||--o{ STATISTICS_SNAPSHOTS : sources
    NODES ||--o{ DNS_PROBE_RESULTS : probes
    CONFIGURATION_REVISIONS ||--o{ DRIFT_EVENTS : desired_for
    OBSERVED_SNAPSHOTS ||--o{ DRIFT_EVENTS : detected_from
    NOTIFICATION_CHANNELS ||--o{ NOTIFICATION_DELIVERIES : delivers
```

Immutable desired revisions, mutable drafts, observed snapshots, per-node
deployment results, drift, and operational data remain distinct. Released
migrations are never renamed for cosmetic product changes.

-- Release 1.1 adds state-aware onboarding acknowledgements without copying
-- canonical node, observation, draft, or revision state.
CREATE TABLE cluster_onboarding_state (
    cluster_id uuid PRIMARY KEY REFERENCES clusters(id) ON DELETE CASCADE,
    redundancy_skipped_at timestamptz,
    monitoring_reviewed_at timestamptz,
    notifications_skipped_at timestamptz,
    completed_at timestamptz,
    completed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    record_version integer NOT NULL DEFAULT 1 CHECK (record_version > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (completed_at IS NOT NULL OR completed_by IS NULL)
);

-- Established clusters must not be forced through first-run onboarding after
-- upgrade.  Current facts are still re-evaluated by the service at read time.
INSERT INTO cluster_onboarding_state (cluster_id, completed_at, record_version, updated_at)
SELECT c.id, now(), 1, now()
FROM clusters c
WHERE EXISTS (
        SELECT 1 FROM nodes n
        WHERE n.cluster_id = c.id AND n.enabled AND n.deleted_at IS NULL
    )
  AND EXISTS (
        SELECT 1 FROM configuration_revisions r
        WHERE r.cluster_id = c.id AND r.archived_at IS NULL
    );

-- Runtime monitoring values are initialised once from the effective pre-v1.1
-- environment by controller startup.  Nullable columns distinguish an
-- upgrade awaiting that initialisation from a deliberate persisted value.
ALTER TABLE system_settings
    ADD COLUMN runtime_settings_initialized boolean NOT NULL DEFAULT false,
    ADD COLUMN node_health_interval_seconds integer
        CHECK (node_health_interval_seconds BETWEEN 5 AND 3600),
    ADD COLUMN statistics_poll_interval_seconds integer
        CHECK (statistics_poll_interval_seconds BETWEEN 60 AND 86400),
    ADD COLUMN query_log_collection_enabled boolean,
    ADD COLUMN query_log_poll_interval_seconds integer
        CHECK (query_log_poll_interval_seconds BETWEEN 5 AND 3600),
    ADD COLUMN query_log_retention_seconds integer
        CHECK (query_log_retention_seconds BETWEEN 3600 AND 7776000);

-- Notification subscriptions become explicit server-owned categories.  The
-- backfill preserves the v1.0 behavior where every HA transition was sent.
ALTER TABLE notification_channels
    ADD COLUMN subscribed_categories text[] NOT NULL DEFAULT ARRAY[
        'dns', 'redundancy', 'certificates', 'versions', 'maintenance', 'upgrades'
    ]::text[],
    ADD CONSTRAINT notification_channels_subscribed_categories_check CHECK (
        cardinality(subscribed_categories) BETWEEN 1 AND 6
        AND subscribed_categories <@ ARRAY[
            'dns', 'redundancy', 'certificates', 'versions', 'maintenance', 'upgrades'
        ]::text[]
    );

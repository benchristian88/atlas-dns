ALTER TABLE notification_channels
    DROP CONSTRAINT IF EXISTS notification_channels_subscribed_categories_check,
    DROP COLUMN IF EXISTS subscribed_categories;

ALTER TABLE system_settings
    DROP COLUMN IF EXISTS query_log_retention_seconds,
    DROP COLUMN IF EXISTS query_log_poll_interval_seconds,
    DROP COLUMN IF EXISTS query_log_collection_enabled,
    DROP COLUMN IF EXISTS statistics_poll_interval_seconds,
    DROP COLUMN IF EXISTS node_health_interval_seconds,
    DROP COLUMN IF EXISTS runtime_settings_initialized;

DROP TABLE IF EXISTS cluster_onboarding_state;

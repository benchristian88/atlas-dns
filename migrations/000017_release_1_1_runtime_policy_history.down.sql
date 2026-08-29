DROP TABLE notification_policy;

ALTER TABLE system_settings
    DROP COLUMN operational_history_retention_days,
    DROP COLUMN log_level,
    DROP COLUMN node_request_timeout_seconds,
    DROP COLUMN session_duration_seconds;

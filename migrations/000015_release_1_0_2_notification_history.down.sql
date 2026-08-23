DROP INDEX notification_deliveries_event_history_idx;

ALTER TABLE notification_deliveries
    DROP COLUMN error_summary,
    DROP COLUMN http_status;

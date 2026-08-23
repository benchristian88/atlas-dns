ALTER TABLE notification_deliveries
    ADD COLUMN http_status integer
        CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    ADD COLUMN error_summary text NOT NULL DEFAULT ''
        CHECK (char_length(error_summary) <= 200);

CREATE INDEX notification_deliveries_event_history_idx
    ON notification_deliveries (event_id, created_at DESC, id DESC);

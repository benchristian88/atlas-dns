CREATE INDEX audit_events_created_at_id_idx
    ON audit_events (created_at DESC, id DESC);

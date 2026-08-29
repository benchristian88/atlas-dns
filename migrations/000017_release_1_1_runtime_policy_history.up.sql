-- Release 1.1 completes the database-backed runtime boundary, adds the
-- controller-wide notification policy, and makes Operational History
-- retention explicit.  Existing nullable monitoring columns from 000016 are
-- still initialized once by startup so v1.0.x environment values are not lost.
ALTER TABLE system_settings
    ADD COLUMN session_duration_seconds integer
        CHECK (session_duration_seconds BETWEEN 900 AND 2592000),
    ADD COLUMN node_request_timeout_seconds integer
        CHECK (node_request_timeout_seconds BETWEEN 1 AND 120),
    ADD COLUMN log_level text
        CHECK (log_level IN ('debug', 'info', 'warn', 'error')),
    ADD COLUMN operational_history_retention_days integer NOT NULL DEFAULT 90
        CHECK (operational_history_retention_days IN (7, 14, 30, 90, 180, 365));

CREATE TABLE notification_policy (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    enabled_event_types text[] NOT NULL DEFAULT ARRAY[
        'dns.failed',
        'dns.recovered',
        'redundancy.degraded',
        'redundancy.at_risk',
        'redundancy.restored',
        'maintenance.return_validation_failed',
        'upgrade.validation_failed'
    ]::text[],
    record_version integer NOT NULL DEFAULT 1 CHECK (record_version > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    CHECK (enabled_event_types <@ ARRAY[
        'dns.failed',
        'dns.recovered',
        'redundancy.degraded',
        'redundancy.at_risk',
        'redundancy.restored',
        'certificate.warning',
        'certificate.critical',
        'certificate.expired',
        'certificate.recovered',
        'version.update_available',
        'version.current',
        'maintenance.started',
        'maintenance.return_validation_failed',
        'maintenance.ended',
        'upgrade.started',
        'upgrade.succeeded',
        'upgrade.validation_failed'
    ]::text[])
);

INSERT INTO notification_policy (singleton) VALUES (true);


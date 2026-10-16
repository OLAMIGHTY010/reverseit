-- Quarantined events keep what was refused so it can be replayed once the cause is
-- fixed (docs/backend/quarantine-replay.md), and a capability for doing so.

ALTER TABLE quarantined_events
    ADD COLUMN payload         jsonb,
    ADD COLUMN status          text        NOT NULL DEFAULT 'open',
    ADD COLUMN attempts        integer     NOT NULL DEFAULT 0,
    ADD COLUMN last_attempt_at timestamptz,
    ADD COLUMN last_reason     text        NOT NULL DEFAULT '',
    ADD COLUMN resolved_at     timestamptz,
    ADD CONSTRAINT quarantined_events_status CHECK (status IN ('open', 'replayed', 'discarded'));
CREATE INDEX quarantined_events_open_idx ON quarantined_events (tenant_id, created_at) WHERE status = 'open';

GRANT UPDATE ON quarantined_events TO sr_app;

INSERT INTO capabilities (capability, description, requires_mfa) VALUES
    ('manage_ingestion', 'Inspect and replay quarantined events and manage source feeds', true);
INSERT INTO permission_grants (role, capability, scope) VALUES
    ('ADMIN_INTEGRATION', 'manage_ingestion', 'full'),
    ('HEAD_OPS', 'manage_ingestion', 'metadata');

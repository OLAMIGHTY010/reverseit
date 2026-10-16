-- Internal alerts (docs/backend/alerts-and-incidents.md): one row per live condition,
-- not one per occurrence, routed to a role and the people on call for it.

CREATE TABLE alerts (
    tenant_id        uuid        NOT NULL REFERENCES institutions (id),
    id               uuid        NOT NULL DEFAULT gen_random_uuid(),
    kind             text        NOT NULL,
    severity         text        NOT NULL,
    dedupe_key       text        NOT NULL,
    subject_type     text        NOT NULL DEFAULT '',
    subject_ref      text        NOT NULL DEFAULT '',
    body             text        NOT NULL,
    target_role      text        NOT NULL,
    routed_to        uuid[]      NOT NULL DEFAULT '{}',
    unrouted         boolean     NOT NULL DEFAULT false,
    status           text        NOT NULL DEFAULT 'open',
    suppressed_by    uuid,
    occurrences      integer     NOT NULL DEFAULT 1,
    escalation_level integer     NOT NULL DEFAULT 0,
    escalated_role   text        NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    last_raised_at   timestamptz NOT NULL DEFAULT now(),
    acknowledged_by  uuid,
    acknowledged_at  timestamptz,
    escalated_at     timestamptz,
    resolved_at      timestamptz,
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT alerts_kind CHECK (kind IN ('critical_exception', 'high_value_unresolved', 'sla_approaching', 'sla_breached', 'reversal_failure',
        'widespread_incident', 'integration_outage', 'approval_request', 'unusual_spike', 'approval_expired')),
    CONSTRAINT alerts_severity CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    CONSTRAINT alerts_status CHECK (status IN ('open', 'acknowledged', 'suppressed', 'resolved')),
    CONSTRAINT alerts_level CHECK (escalation_level BETWEEN 0 AND 3)
);
-- One live alert per condition. A resolved one is history and a new raise starts afresh.
CREATE UNIQUE INDEX alerts_one_live ON alerts (tenant_id, dedupe_key) WHERE status <> 'resolved';
CREATE INDEX alerts_open_idx ON alerts (tenant_id, status, created_at);

ALTER TABLE alerts ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON alerts USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE ON alerts TO sr_app;

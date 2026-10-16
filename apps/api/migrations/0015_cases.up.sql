-- The shared exception queue (B4). Bot-detected and customer-reported cases use
-- the same tables, states, history and evidence.

CREATE SEQUENCE case_number_seq;

CREATE TABLE cases (
    tenant_id         uuid        NOT NULL REFERENCES institutions (id),
    id                uuid        NOT NULL DEFAULT gen_random_uuid(),
    case_ref          text        NOT NULL,
    classification    text        NOT NULL,
    status            text        NOT NULL DEFAULT 'DETECTED',
    priority          text        NOT NULL,
    risk_grade        text        NOT NULL DEFAULT '',
    source            text        NOT NULL,
    txn_id            uuid,
    exposure_minor    bigint,
    currency          text,
    root_cause        text        NOT NULL DEFAULT '',
    owner_id          uuid,
    queue             text        NOT NULL DEFAULT 'OPERATIONS',
    rule_id           text        NOT NULL DEFAULT '',
    rule_version      text        NOT NULL DEFAULT '',
    regulatory_clock  text        NOT NULL DEFAULT '',
    opened_at         timestamptz NOT NULL,
    sla_due_at        timestamptz,
    resolved_at       timestamptz,
    closed_reason     text        NOT NULL DEFAULT '',
    complaint         jsonb,
    version           integer     NOT NULL DEFAULT 1,
    updated_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, case_ref),
    FOREIGN KEY (tenant_id, txn_id) REFERENCES transactions (tenant_id, id),
    CONSTRAINT cases_classification CHECK (classification ~ '^[A-Z]{3,4}[0-9]{3}$'),
    CONSTRAINT cases_source CHECK (source IN ('bot', 'customer', 'manual')),
    CONSTRAINT cases_priority CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    CONSTRAINT cases_status CHECK (status IN (
        'DETECTED', 'AWAITING_EVIDENCE', 'UNDER_INVESTIGATION', 'AWAITING_APPROVAL', 'REMEDIATION_SUBMITTED',
        'OUTCOME_VERIFICATION', 'RESOLVED', 'CLOSED', 'REJECTED_ACTION', 'REMEDIATION_FAILED', 'EXTERNAL_DEPENDENCY',
        'SUPPRESSED', 'CLOSED_NO_EXCEPTION', 'CLOSED_CUSTOMER_ERROR', 'REOPENED'))
);
-- One bot-detected case per transaction and classification, however many times
-- the detector sees the transaction.
CREATE UNIQUE INDEX cases_bot_once ON cases (tenant_id, txn_id, classification) WHERE source = 'bot';
CREATE INDEX cases_queue_idx ON cases (tenant_id, status, opened_at DESC);
CREATE INDEX cases_txn_idx ON cases (tenant_id, txn_id);

CREATE TABLE case_events (
    tenant_id   uuid        NOT NULL,
    case_id     uuid        NOT NULL,
    seq         integer     NOT NULL,
    at          timestamptz NOT NULL,
    actor_type  text        NOT NULL,
    actor_id    uuid,
    kind        text        NOT NULL,
    from_status text        NOT NULL DEFAULT '',
    to_status   text        NOT NULL DEFAULT '',
    reason_code text        NOT NULL DEFAULT '',
    detail      jsonb       NOT NULL DEFAULT '{}',
    PRIMARY KEY (tenant_id, case_id, seq),
    FOREIGN KEY (tenant_id, case_id) REFERENCES cases (tenant_id, id),
    CONSTRAINT case_events_actor CHECK (actor_type IN ('user', 'service', 'system'))
);

CREATE TABLE case_evidence (
    tenant_id     uuid        NOT NULL,
    case_id       uuid        NOT NULL,
    id            uuid        NOT NULL DEFAULT gen_random_uuid(),
    kind          text        NOT NULL,
    ref           text        NOT NULL,
    source_system text        NOT NULL DEFAULT '',
    fetched_at    timestamptz NOT NULL,
    hash          text        NOT NULL DEFAULT '',
    is_fact       boolean     NOT NULL DEFAULT true,
    detail        jsonb       NOT NULL DEFAULT '{}',
    PRIMARY KEY (tenant_id, case_id, id),
    FOREIGN KEY (tenant_id, case_id) REFERENCES cases (tenant_id, id)
);

-- Messages the system decided to send. In this build nothing is delivered: rows
-- are recorded, labelled simulated, for the notification mock and the screens.
CREATE TABLE notifications (
    tenant_id    uuid        NOT NULL REFERENCES institutions (id),
    id           uuid        NOT NULL DEFAULT gen_random_uuid(),
    case_id      uuid        NOT NULL,
    audience     text        NOT NULL,
    stage        text        NOT NULL,
    template_id  text        NOT NULL,
    body         text        NOT NULL,
    dedupe_key   text        NOT NULL,
    status       text        NOT NULL DEFAULT 'recorded',
    simulated    boolean     NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, dedupe_key),
    FOREIGN KEY (tenant_id, case_id) REFERENCES cases (tenant_id, id),
    CONSTRAINT notifications_audience CHECK (audience IN ('customer', 'operations')),
    CONSTRAINT notifications_stage CHECK (stage IN ('detected', 'verified', 'alert'))
);

ALTER TABLE cases         ENABLE ROW LEVEL SECURITY;
ALTER TABLE case_events   ENABLE ROW LEVEL SECURITY;
ALTER TABLE case_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cases
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON case_events
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON case_evidence
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON notifications
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE ON cases TO sr_app;
GRANT SELECT, INSERT ON case_events, case_evidence, notifications TO sr_app;
GRANT USAGE ON SEQUENCE case_number_seq TO sr_app;

CREATE FUNCTION case_history_reject_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = 'restrict_violation';
END
$$;
CREATE TRIGGER case_events_no_change BEFORE UPDATE OR DELETE ON case_events
    FOR EACH ROW EXECUTE FUNCTION case_history_reject_change();
CREATE TRIGGER case_evidence_no_change BEFORE UPDATE OR DELETE ON case_evidence
    FOR EACH ROW EXECUTE FUNCTION case_history_reject_change();
CREATE TRIGGER notifications_no_change BEFORE DELETE ON notifications
    FOR EACH ROW EXECUTE FUNCTION case_history_reject_change();

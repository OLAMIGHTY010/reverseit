-- Customer feedback, and labelled examples from cases the detector missed
-- (docs/backend/customer-backend.md).

CREATE TABLE customer_feedback (
    tenant_id       uuid        NOT NULL REFERENCES institutions (id),
    id              uuid        NOT NULL DEFAULT gen_random_uuid(),
    report_id       uuid        NOT NULL,
    customer_id     uuid        NOT NULL,
    idempotency_key text        NOT NULL,
    helpful         boolean,
    comment         text        NOT NULL DEFAULT '',
    disputes_outcome boolean    NOT NULL DEFAULT false,
    reopened        boolean     NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, customer_id, idempotency_key),
    FOREIGN KEY (tenant_id, report_id) REFERENCES customer_reports (tenant_id, id),
    CONSTRAINT customer_feedback_comment CHECK (length(comment) <= 1000)
);

-- A case a customer reported and the detector had not raised becomes a labelled example
-- in the pending state. Only confirmed examples may enter training or rule tuning.
-- Decisions are new versions, never edits, so the label store has a history.
CREATE TABLE labeled_examples (
    tenant_id  uuid        NOT NULL REFERENCES institutions (id),
    id         uuid        NOT NULL DEFAULT gen_random_uuid(),
    case_id    uuid        NOT NULL,
    txn_id     uuid        NOT NULL,
    version    integer     NOT NULL DEFAULT 1,
    label      jsonb       NOT NULL,
    status     text        NOT NULL DEFAULT 'pending',
    decided_by uuid,
    decided_at timestamptz,
    note       text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, case_id) REFERENCES cases (tenant_id, id),
    CONSTRAINT labeled_examples_status CHECK (status IN ('pending', 'confirmed', 'rejected', 'superseded'))
);
-- One live version per case: pending, confirmed or rejected. Older ones are superseded.
CREATE UNIQUE INDEX labeled_examples_live ON labeled_examples (tenant_id, case_id) WHERE status <> 'superseded';

ALTER TABLE customer_feedback ENABLE ROW LEVEL SECURITY;
ALTER TABLE labeled_examples  ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON customer_feedback USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON labeled_examples USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT ON customer_feedback TO sr_app;
GRANT SELECT, INSERT, UPDATE ON labeled_examples TO sr_app;

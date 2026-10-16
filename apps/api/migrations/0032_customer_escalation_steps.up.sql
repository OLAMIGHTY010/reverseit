-- The escalation ladder shown to a customer whose case is overdue and unresolved
-- (docs/backend/customer-backend.md). Contact details and wording are the bank's to
-- supply and Compliance's to review: a step is shown only once someone other than its
-- author has approved it. Nothing is seeded.

CREATE TABLE customer_escalation_steps (
    tenant_id     uuid        NOT NULL REFERENCES institutions (id),
    step_no       integer     NOT NULL,
    label         text        NOT NULL,
    contact_text  text        NOT NULL,
    wording       text        NOT NULL,
    review_status text        NOT NULL DEFAULT 'draft',
    authored_by   uuid        NOT NULL,
    reviewed_by   uuid,
    reviewed_at   timestamptz,
    review_note   text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, step_no),
    CONSTRAINT customer_escalation_steps_no CHECK (step_no BETWEEN 1 AND 10),
    CONSTRAINT customer_escalation_steps_status CHECK (review_status IN ('draft', 'approved', 'rejected')),
    CONSTRAINT customer_escalation_steps_four_eyes CHECK (reviewed_by IS NULL OR reviewed_by <> authored_by)
);
ALTER TABLE customer_escalation_steps ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON customer_escalation_steps USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE ON customer_escalation_steps TO sr_app;

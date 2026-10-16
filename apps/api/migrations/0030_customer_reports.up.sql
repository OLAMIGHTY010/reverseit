-- Reports a ReverseIt customer makes, and the contacts they add to cases
-- (docs/backend/customer-backend.md).

ALTER TABLE customer_accounts ADD COLUMN tail text NOT NULL DEFAULT '', ADD CONSTRAINT customer_accounts_tail CHECK (tail = '' OR tail ~ '^[0-9]{4}$');

CREATE TABLE customer_reports (
    tenant_id        uuid        NOT NULL REFERENCES institutions (id),
    id               uuid        NOT NULL DEFAULT gen_random_uuid(),
    customer_id      uuid        NOT NULL,
    idempotency_key  text        NOT NULL,
    status           text        NOT NULL DEFAULT 'created',
    consent_at       timestamptz NOT NULL,
    consent_version  text        NOT NULL,
    -- The alert as pasted is never stored. Only the redacted text and a hash of the original.
    alert_redacted   text        NOT NULL DEFAULT '',
    alert_hash       text        NOT NULL DEFAULT '',
    image_ref        text        NOT NULL DEFAULT '',
    parser           text        NOT NULL DEFAULT '',
    parser_status    text        NOT NULL DEFAULT '',
    parsed           jsonb       NOT NULL DEFAULT '{}',
    confirmed        jsonb       NOT NULL DEFAULT '{}',
    candidates       jsonb       NOT NULL DEFAULT '[]',
    match_status     text        NOT NULL DEFAULT '',
    txn_id           uuid,
    case_id          uuid,
    dispute_type     text        NOT NULL DEFAULT '',
    draft            text        NOT NULL DEFAULT '',
    edited           text        NOT NULL DEFAULT '',
    submitted_at     timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    version          integer     NOT NULL DEFAULT 1,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, customer_id, idempotency_key),
    FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id),
    FOREIGN KEY (tenant_id, txn_id) REFERENCES transactions (tenant_id, id),
    CONSTRAINT customer_reports_status CHECK (status IN ('created', 'confirmed', 'submitted')),
    CONSTRAINT customer_reports_match CHECK (match_status IN ('', 'candidates', 'matched_existing_case', 'matched_transaction', 'verified_successful',
        'already_reversed', 'unmatched', 'duplicate_report', 'other_bank'))
);
CREATE INDEX customer_reports_customer_idx ON customer_reports (tenant_id, customer_id, created_at DESC);
CREATE INDEX customer_reports_txn_idx ON customer_reports (tenant_id, txn_id) WHERE txn_id IS NOT NULL;

-- Each report that attaches to a case is one contact, so repeat contacts are counted.
CREATE TABLE case_contacts (
    tenant_id   uuid        NOT NULL REFERENCES institutions (id),
    case_id     uuid        NOT NULL,
    report_id   uuid        NOT NULL,
    customer_id uuid        NOT NULL,
    at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, case_id, report_id),
    FOREIGN KEY (tenant_id, case_id) REFERENCES cases (tenant_id, id),
    FOREIGN KEY (tenant_id, report_id) REFERENCES customer_reports (tenant_id, id)
);

ALTER TABLE customer_reports ENABLE ROW LEVEL SECURITY;
ALTER TABLE case_contacts    ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON customer_reports USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON case_contacts USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE ON customer_reports TO sr_app;
GRANT SELECT, INSERT ON case_contacts TO sr_app;

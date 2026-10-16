-- External references seen for a transaction, used to correlate evidence from other
-- sources (docs/backend/correlation.md). A reference is evidence of a link, never a
-- key: the same RRN can be reused by a scheme on another day, so a match is judged
-- together with amount and time.

CREATE TABLE transaction_refs (
    tenant_id     uuid        NOT NULL REFERENCES institutions (id),
    txn_id        uuid        NOT NULL,
    ref_type      text        NOT NULL,
    ref_value     text        NOT NULL,
    source_system text        NOT NULL,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, txn_id, ref_type, ref_value),
    FOREIGN KEY (tenant_id, txn_id) REFERENCES transactions (tenant_id, id),
    CONSTRAINT transaction_refs_type CHECK (ref_type IN ('RRN', 'STAN', 'SESSION_ID', 'LEDGER_ID', 'GATEWAY_REF'))
);
CREATE INDEX transaction_refs_lookup_idx ON transaction_refs (tenant_id, ref_type, ref_value);

ALTER TABLE transaction_refs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON transaction_refs USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT ON transaction_refs TO sr_app;

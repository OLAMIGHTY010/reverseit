-- Findings held back because a source the transaction depends on is stale or down
-- (docs/backend/source-degradation.md). A hold is a reason not to raise a case yet,
-- not a case. It is released when the source recovers or the transaction resolves.

CREATE TABLE detection_holds (
    tenant_id   uuid        NOT NULL REFERENCES institutions (id),
    txn_id      uuid        NOT NULL,
    code        text        NOT NULL,
    reason      text        NOT NULL,
    held_at     timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz,
    PRIMARY KEY (tenant_id, txn_id, code),
    FOREIGN KEY (tenant_id, txn_id) REFERENCES transactions (tenant_id, id),
    CONSTRAINT detection_holds_code CHECK (code IN ('UNK001', 'DUP001'))
);
CREATE INDEX detection_holds_open_idx ON detection_holds (tenant_id, code) WHERE released_at IS NULL;

ALTER TABLE detection_holds ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON detection_holds USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE ON detection_holds TO sr_app;

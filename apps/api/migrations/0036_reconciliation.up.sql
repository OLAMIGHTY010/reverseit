-- Batch Reconciliation and Settlement Ingestion (B6).
--
-- Inbound settlement, statement and switch response files are stored unmodified,
-- hashed, scanned and parsed. Records are matched against transaction debits and
-- credits to detect breaks, ledger imbalances, and timing cutoff issues.

CREATE TABLE settlement_files (
    tenant_id       uuid        NOT NULL REFERENCES institutions (id),
    id              uuid        NOT NULL DEFAULT gen_random_uuid(),
    provider        text        NOT NULL,
    kind            text        NOT NULL,
    period_start    date        NOT NULL,
    period_end      date        NOT NULL,
    mapping_version text        NOT NULL DEFAULT '1.0',
    sha256          text        NOT NULL,
    filename        text        NOT NULL,
    record_count    integer     NOT NULL DEFAULT 0,
    matched_count   integer     NOT NULL DEFAULT 0,
    break_count     integer     NOT NULL DEFAULT 0,
    raw_content     text        NOT NULL DEFAULT '',
    uploaded_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, provider, period_start, period_end, sha256),
    CONSTRAINT settlement_files_kind CHECK (kind IN ('SETTLEMENT', 'LEDGER_STATEMENT', 'SWITCH_RESPONSE'))
);
CREATE INDEX settlement_files_provider_idx ON settlement_files (tenant_id, provider, period_start, period_end);

CREATE TABLE settlement_records (
    tenant_id       uuid        NOT NULL REFERENCES institutions (id),
    id              uuid        NOT NULL DEFAULT gen_random_uuid(),
    file_id         uuid        NOT NULL,
    provider        text        NOT NULL,
    instruction_ref text        NOT NULL,
    session_id      text        NOT NULL DEFAULT '',
    amount_minor    bigint      NOT NULL,
    currency        text        NOT NULL,
    settled_on      date        NOT NULL,
    status          text        NOT NULL,
    match_status    text        NOT NULL DEFAULT 'UNMATCHED',
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, file_id) REFERENCES settlement_files (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT settlement_records_amount_positive CHECK (amount_minor > 0),
    CONSTRAINT settlement_records_currency CHECK (currency ~ '^[A-Z]{3}$')
);
CREATE INDEX settlement_records_lookup ON settlement_records (tenant_id, instruction_ref);

CREATE TABLE reconciliation_breaks (
    tenant_id           uuid        NOT NULL REFERENCES institutions (id),
    id                  uuid        NOT NULL DEFAULT gen_random_uuid(),
    file_id             uuid,
    session_code        text        NOT NULL DEFAULT '',
    channel             text        NOT NULL DEFAULT 'NIP',
    break_type          text        NOT NULL,
    break_label         text        NOT NULL,
    severity            text        NOT NULL DEFAULT 'HIGH',
    status              text        NOT NULL DEFAULT 'UNMATCHED',
    instruction_ref     text        NOT NULL DEFAULT '',
    session_id          text        NOT NULL DEFAULT '',
    ledger_amount_minor bigint,
    switch_amount_minor bigint,
    amount_diff_minor   bigint      NOT NULL DEFAULT 0,
    time_diff_seconds   integer     NOT NULL DEFAULT 0,
    case_id             uuid,
    maker_notes         text        NOT NULL DEFAULT '',
    checker_signoff     jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, file_id) REFERENCES settlement_files (tenant_id, id) ON DELETE SET NULL,
    FOREIGN KEY (tenant_id, case_id) REFERENCES cases (tenant_id, id) ON DELETE SET NULL,
    CONSTRAINT recon_breaks_type CHECK (break_type IN ('LEDGER_DEBIT_NO_SWITCH', 'SWITCH_RECORD_NO_LEDGER', 'AMOUNT_MISMATCH', 'TIMING_CUTOFF_BREAK')),
    CONSTRAINT recon_breaks_status CHECK (status IN ('UNMATCHED', 'TOLERANCE_MATCHED', 'MANUALLY_PAIRED', 'ESCALATED_DISPUTE', 'SETTLED')),
    CONSTRAINT recon_breaks_severity CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL'))
);
CREATE INDEX recon_breaks_status_idx ON reconciliation_breaks (tenant_id, status, created_at DESC);
CREATE INDEX recon_breaks_case_idx ON reconciliation_breaks (tenant_id, case_id);

ALTER TABLE settlement_files       ENABLE ROW LEVEL SECURITY;
ALTER TABLE settlement_records     ENABLE ROW LEVEL SECURITY;
ALTER TABLE reconciliation_breaks  ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON settlement_files USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON settlement_records USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON reconciliation_breaks USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON settlement_files, settlement_records, reconciliation_breaks TO sr_app;

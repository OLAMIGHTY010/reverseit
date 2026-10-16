-- Transactions and their event history (B2).
--
-- Events are append-only evidence. A transaction row is a projection that is
-- recomputed from all of its events, so late and out-of-order events converge on
-- the same answer. Amounts are integer minor units.

CREATE TABLE transactions (
    tenant_id           uuid        NOT NULL REFERENCES institutions (id),
    id                  uuid        NOT NULL DEFAULT gen_random_uuid(),
    instruction_ref     text        NOT NULL,
    channel             text        NOT NULL,
    rail                text        NOT NULL,
    amount_minor        bigint      NOT NULL,
    currency            text        NOT NULL,
    debit_account_token text        NOT NULL,
    beneficiary_bank    text        NOT NULL DEFAULT '',
    beneficiary_token   text        NOT NULL DEFAULT '',
    session_id          text        NOT NULL DEFAULT '',
    lifecycle_state     text        NOT NULL,
    debit_status        text        NOT NULL,
    credit_status       text        NOT NULL,
    settlement_status   text        NOT NULL,
    reversal_status     text        NOT NULL DEFAULT 'NONE',
    first_debit_at      timestamptz,
    first_event_at      timestamptz NOT NULL,
    last_event_at       timestamptz NOT NULL,
    event_count         integer     NOT NULL DEFAULT 0,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, instruction_ref),
    CONSTRAINT transactions_amount_positive CHECK (amount_minor > 0),
    CONSTRAINT transactions_currency_format CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT transactions_state CHECK (lifecycle_state IN
        ('INITIATED', 'DEBITED', 'PENDING', 'CONFIRMED_SUCCESS', 'CONFIRMED_FAILURE', 'UNKNOWN', 'CONFLICTING', 'REVERSED'))
);
CREATE INDEX transactions_duplicate_idx ON transactions (tenant_id, debit_account_token, amount_minor, currency, first_debit_at);
CREATE INDEX transactions_state_idx ON transactions (tenant_id, lifecycle_state, last_event_at DESC);

CREATE TABLE transaction_events (
    tenant_id        uuid        NOT NULL,
    id               uuid        NOT NULL DEFAULT gen_random_uuid(),
    txn_id           uuid        NOT NULL,
    source_system    text        NOT NULL,
    source_event_ref text        NOT NULL,
    event_type       text        NOT NULL,
    source_ts        timestamptz NOT NULL,
    received_at      timestamptz NOT NULL DEFAULT now(),
    payload_hash     text        NOT NULL,
    payload          jsonb       NOT NULL,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, source_system, source_event_ref),
    FOREIGN KEY (tenant_id, txn_id) REFERENCES transactions (tenant_id, id),
    CONSTRAINT transaction_events_type CHECK (event_type IN
        ('DEBIT_POSTED', 'TRANSFER_SENT', 'CREDIT_CONFIRMED', 'CREDIT_FAILED', 'TRANSFER_RESPONSE_TIMEOUT',
         'SETTLEMENT_COMPLETED', 'REVERSAL_POSTED'))
);
CREATE INDEX transaction_events_txn_idx ON transaction_events (tenant_id, txn_id, source_ts);

-- Events that failed validation are kept for inspection and replay (FR-03).
CREATE TABLE quarantined_events (
    tenant_id     uuid        NOT NULL REFERENCES institutions (id),
    id            uuid        NOT NULL DEFAULT gen_random_uuid(),
    source_system text        NOT NULL DEFAULT '',
    reason        text        NOT NULL,
    payload_hash  text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id)
);

ALTER TABLE transactions        ENABLE ROW LEVEL SECURITY;
ALTER TABLE transaction_events  ENABLE ROW LEVEL SECURITY;
ALTER TABLE quarantined_events  ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON transactions
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON transaction_events
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON quarantined_events
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE ON transactions TO sr_app;
GRANT SELECT, INSERT ON transaction_events, quarantined_events TO sr_app;

CREATE FUNCTION transaction_events_reject_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'transaction_events is append-only' USING ERRCODE = 'restrict_violation';
END
$$;
CREATE TRIGGER transaction_events_no_change BEFORE UPDATE OR DELETE ON transaction_events
    FOR EACH ROW EXECUTE FUNCTION transaction_events_reject_change();

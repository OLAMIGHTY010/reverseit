-- Controlled reversal (B5) and the SIMULATED systems it talks to in Track H.
--
-- Nothing here moves real money. The sim_* tables stand in for the authoritative
-- status source, the core ledger and the reversal provider so that the whole
-- flow can be demonstrated and tested. Every row they hold is synthetic.

CREATE SEQUENCE reversal_number_seq;

CREATE TABLE reversals (
    tenant_id         uuid        NOT NULL REFERENCES institutions (id),
    id                uuid        NOT NULL DEFAULT gen_random_uuid(),
    reversal_ref      text        NOT NULL,
    case_id           uuid        NOT NULL,
    txn_id            uuid        NOT NULL,
    idempotency_key   text        NOT NULL,
    request_hash      text        NOT NULL,
    requested_by      uuid,
    requested_by_type text        NOT NULL,
    amount_minor      bigint      NOT NULL,
    currency          text        NOT NULL,
    reason_code       text        NOT NULL,
    approval_status   text        NOT NULL,
    accepted_status   text        NOT NULL DEFAULT 'PENDING',
    processing_status text        NOT NULL DEFAULT 'NOT_STARTED',
    financial_status  text        NOT NULL DEFAULT 'UNVERIFIED',
    gate_result       jsonb       NOT NULL,
    policy_version    text        NOT NULL DEFAULT '',
    attempt_no        integer     NOT NULL DEFAULT 1,
    provider_ref      text        NOT NULL DEFAULT '',
    failure_reason    text        NOT NULL DEFAULT '',
    submitted_at      timestamptz,
    posted_at         timestamptz,
    verified_at       timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    version           integer     NOT NULL DEFAULT 1,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, reversal_ref),
    UNIQUE (tenant_id, idempotency_key),
    FOREIGN KEY (tenant_id, case_id) REFERENCES cases (tenant_id, id),
    FOREIGN KEY (tenant_id, txn_id) REFERENCES transactions (tenant_id, id),
    CONSTRAINT reversals_amount_positive CHECK (amount_minor > 0),
    CONSTRAINT reversals_requester CHECK (requested_by_type IN ('user', 'system')),
    CONSTRAINT reversals_approval CHECK (approval_status IN ('NOT_REQUIRED', 'PENDING', 'APPROVED', 'REJECTED')),
    CONSTRAINT reversals_accepted CHECK (accepted_status IN ('PENDING', 'ACCEPTED', 'REJECTED', 'UNKNOWN')),
    CONSTRAINT reversals_processing CHECK (processing_status IN ('NOT_STARTED', 'IN_PROGRESS', 'COMPLETED', 'FAILED', 'UNKNOWN')),
    CONSTRAINT reversals_financial CHECK (financial_status IN ('UNVERIFIED', 'VERIFIED', 'FAILED'))
);

-- The remediation lock: at most one live reversal per transaction. A reversal
-- that was rejected, refused by approval or proven failed frees the lock.
CREATE UNIQUE INDEX reversals_one_live_per_txn ON reversals (tenant_id, txn_id)
    WHERE approval_status <> 'REJECTED' AND accepted_status <> 'REJECTED' AND financial_status <> 'FAILED';
CREATE INDEX reversals_case_idx ON reversals (tenant_id, case_id);

CREATE TABLE approvals (
    tenant_id     uuid        NOT NULL,
    id            uuid        NOT NULL DEFAULT gen_random_uuid(),
    reversal_id   uuid        NOT NULL,
    maker         uuid,
    maker_type    text        NOT NULL,
    reason        text        NOT NULL,
    status        text        NOT NULL DEFAULT 'PENDING',
    checker       uuid,
    decision_reason text      NOT NULL DEFAULT '',
    limit_applied bigint,
    created_at    timestamptz NOT NULL DEFAULT now(),
    decided_at    timestamptz,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, reversal_id),
    FOREIGN KEY (tenant_id, reversal_id) REFERENCES reversals (tenant_id, id),
    CONSTRAINT approvals_status CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED')),
    CONSTRAINT approvals_maker_type CHECK (maker_type IN ('user', 'system')),
    CONSTRAINT approvals_decided_has_checker CHECK (status = 'PENDING' OR checker IS NOT NULL)
);

-- The database refuses a checker who is also the maker.
CREATE FUNCTION approvals_check() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status <> 'PENDING' AND NEW.maker IS NOT NULL AND NEW.checker = NEW.maker THEN
        RAISE EXCEPTION 'the maker cannot also be the checker' USING ERRCODE = 'restrict_violation';
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.status <> 'PENDING' THEN
        RAISE EXCEPTION 'an approval is decided once' USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER approvals_maker_checker BEFORE INSERT OR UPDATE ON approvals
    FOR EACH ROW EXECUTE FUNCTION approvals_check();

-- ---- simulated systems (Track H) ------------------------------------------------

-- What the simulated authoritative status source will say for an instruction.
CREATE TABLE sim_status_script (
    tenant_id            uuid        NOT NULL REFERENCES institutions (id),
    instruction_ref      text        NOT NULL,
    outcome              text        NOT NULL,
    beneficiary_credited boolean     NOT NULL DEFAULT false,
    settlement_paid      boolean     NOT NULL DEFAULT false,
    age_seconds          integer     NOT NULL DEFAULT 0,
    set_at               timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, instruction_ref),
    CONSTRAINT sim_status_outcome CHECK (outcome IN ('CONFIRMED_SUCCESS', 'CONFIRMED_FAILURE', 'PENDING', 'UNKNOWN', 'CONFLICTING'))
);

-- The simulated core ledger.
CREATE TABLE sim_ledger_entries (
    tenant_id       uuid        NOT NULL REFERENCES institutions (id),
    id              uuid        NOT NULL DEFAULT gen_random_uuid(),
    instruction_ref text        NOT NULL,
    kind            text        NOT NULL,
    reversal_ref    text        NOT NULL DEFAULT '',
    amount_minor    bigint      NOT NULL,
    currency        text        NOT NULL,
    posted_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT sim_ledger_kind CHECK (kind IN ('DEBIT', 'REVERSAL_CREDIT'))
);
CREATE UNIQUE INDEX sim_ledger_one_debit ON sim_ledger_entries (tenant_id, instruction_ref) WHERE kind = 'DEBIT';
CREATE UNIQUE INDEX sim_ledger_one_reversal ON sim_ledger_entries (tenant_id, reversal_ref) WHERE kind = 'REVERSAL_CREDIT';

-- How the simulated reversal provider behaves for an instruction, and what it did.
CREATE TABLE sim_provider_script (
    tenant_id       uuid        NOT NULL REFERENCES institutions (id),
    instruction_ref text        NOT NULL,
    behaviour       text        NOT NULL DEFAULT 'SUCCESS',
    PRIMARY KEY (tenant_id, instruction_ref),
    CONSTRAINT sim_provider_behaviour CHECK (behaviour IN ('SUCCESS', 'REJECT', 'TIMEOUT', 'DELAYED', 'GHOST'))
);
CREATE TABLE sim_provider_requests (
    tenant_id       uuid        NOT NULL REFERENCES institutions (id),
    idempotency_key text        NOT NULL,
    provider_ref    text        NOT NULL,
    instruction_ref text        NOT NULL,
    amount_minor    bigint      NOT NULL,
    currency        text        NOT NULL,
    reversal_ref    text        NOT NULL,
    behaviour       text        NOT NULL,
    polls           integer     NOT NULL DEFAULT 0,
    state           text        NOT NULL DEFAULT 'ACCEPTED',
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, idempotency_key),
    CONSTRAINT sim_provider_state CHECK (state IN ('ACCEPTED', 'REJECTED', 'POSTED'))
);

ALTER TABLE reversals              ENABLE ROW LEVEL SECURITY;
ALTER TABLE approvals              ENABLE ROW LEVEL SECURITY;
ALTER TABLE sim_status_script      ENABLE ROW LEVEL SECURITY;
ALTER TABLE sim_ledger_entries     ENABLE ROW LEVEL SECURITY;
ALTER TABLE sim_provider_script    ENABLE ROW LEVEL SECURITY;
ALTER TABLE sim_provider_requests  ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON reversals USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON approvals USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON sim_status_script USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON sim_ledger_entries USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON sim_provider_script USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON sim_provider_requests USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE ON reversals, approvals, sim_provider_requests TO sr_app;
GRANT SELECT, INSERT, UPDATE ON sim_status_script, sim_provider_script TO sr_app;
GRANT SELECT, INSERT ON sim_ledger_entries TO sr_app;
GRANT USAGE ON SEQUENCE reversal_number_seq TO sr_app;

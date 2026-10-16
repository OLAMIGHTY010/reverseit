-- Advanced Reversal Gating, Dual-Control Overrides and Timeout Recovery (Prompt B5).

CREATE TABLE gate_overrides (
    tenant_id          uuid        NOT NULL REFERENCES institutions (id),
    id                 uuid        NOT NULL DEFAULT gen_random_uuid(),
    reversal_id        uuid        NOT NULL,
    proposer_id        uuid        NOT NULL,
    approver_id        uuid,
    justification      text        NOT NULL,
    status             text        NOT NULL DEFAULT 'PENDING',
    decision_reason    text        NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now(),
    decided_at         timestamptz,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, reversal_id),
    FOREIGN KEY (tenant_id, reversal_id) REFERENCES reversals (tenant_id, id),
    CONSTRAINT gate_overrides_status CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED')),
    CONSTRAINT gate_overrides_justification CHECK (btrim(justification) <> '')
);

CREATE FUNCTION gate_override_maker_checker() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status <> 'PENDING' AND NEW.approver_id IS NOT NULL AND NEW.approver_id = NEW.proposer_id THEN
        RAISE EXCEPTION 'the proposer cannot also be the approver of a gate override' USING ERRCODE = 'restrict_violation';
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.status <> 'PENDING' THEN
        RAISE EXCEPTION 'a gate override is decided once' USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER gate_override_check BEFORE INSERT OR UPDATE ON gate_overrides
    FOR EACH ROW EXECUTE FUNCTION gate_override_maker_checker();

CREATE TABLE recovery_subcases (
    tenant_id          uuid        NOT NULL REFERENCES institutions (id),
    id                 uuid        NOT NULL DEFAULT gen_random_uuid(),
    parent_case_id     uuid        NOT NULL,
    subcase_id         uuid        NOT NULL,
    reversal_id        uuid        NOT NULL,
    reason             text        NOT NULL,
    status             text        NOT NULL DEFAULT 'OPEN',
    created_at         timestamptz NOT NULL DEFAULT now(),
    resolved_at        timestamptz,
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, parent_case_id) REFERENCES cases (tenant_id, id),
    FOREIGN KEY (tenant_id, subcase_id) REFERENCES cases (tenant_id, id),
    FOREIGN KEY (tenant_id, reversal_id) REFERENCES reversals (tenant_id, id)
);

ALTER TABLE gate_overrides ENABLE ROW LEVEL SECURITY;
ALTER TABLE recovery_subcases ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON gate_overrides
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY tenant_isolation ON recovery_subcases
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE ON gate_overrides TO sr_app;
GRANT SELECT, INSERT, UPDATE ON recovery_subcases TO sr_app;

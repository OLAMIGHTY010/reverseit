-- Delegation, on-call and escalation routing (v2 B1, item 9).
--
-- These tables decide who a piece of work is routed to. They never decide who is
-- allowed to do it: authority always comes from roles and the permission
-- matrix, so a delegate who lacks the role is still refused.

CREATE TABLE delegations (
    tenant_id  uuid        NOT NULL REFERENCES institutions (id),
    id         uuid        NOT NULL DEFAULT gen_random_uuid(),
    delegator  uuid        NOT NULL,
    delegate   uuid        NOT NULL,
    starts_at  timestamptz NOT NULL,
    ends_at    timestamptz NOT NULL,
    reason     text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, delegator) REFERENCES users (tenant_id, id),
    FOREIGN KEY (tenant_id, delegate)  REFERENCES users (tenant_id, id),
    CONSTRAINT delegations_not_self CHECK (delegator <> delegate),
    CONSTRAINT delegations_window CHECK (ends_at > starts_at AND ends_at <= starts_at + interval '30 days'),
    CONSTRAINT delegations_reason CHECK (btrim(reason) <> '')
);

CREATE TABLE delegation_revocations (
    tenant_id     uuid        NOT NULL,
    delegation_id uuid        NOT NULL,
    revoked_by    uuid        NOT NULL,
    revoked_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, delegation_id),
    FOREIGN KEY (tenant_id, delegation_id) REFERENCES delegations (tenant_id, id)
);

CREATE TABLE on_call_shifts (
    tenant_id  uuid        NOT NULL REFERENCES institutions (id),
    id         uuid        NOT NULL DEFAULT gen_random_uuid(),
    role       text        NOT NULL REFERENCES roles (role),
    user_id    uuid        NOT NULL,
    starts_at  timestamptz NOT NULL,
    ends_at    timestamptz NOT NULL,
    created_by uuid        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id),
    CONSTRAINT on_call_window CHECK (ends_at > starts_at AND ends_at <= starts_at + interval '31 days')
);
CREATE INDEX on_call_shifts_lookup ON on_call_shifts (tenant_id, role, starts_at, ends_at);

-- If the role has no one on call, work escalates from one role to the next.
CREATE TABLE escalation_steps (
    tenant_id uuid        NOT NULL REFERENCES institutions (id),
    from_role text        NOT NULL REFERENCES roles (role),
    to_role   text        NOT NULL REFERENCES roles (role),
    set_by    uuid        NOT NULL,
    set_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, from_role),
    CONSTRAINT escalation_not_self CHECK (from_role <> to_role)
);

ALTER TABLE delegations            ENABLE ROW LEVEL SECURITY;
ALTER TABLE delegation_revocations ENABLE ROW LEVEL SECURITY;
ALTER TABLE on_call_shifts         ENABLE ROW LEVEL SECURITY;
ALTER TABLE escalation_steps       ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON delegations
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON delegation_revocations
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON on_call_shifts
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON escalation_steps
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON delegations, delegation_revocations, on_call_shifts TO sr_app;
GRANT SELECT, INSERT, UPDATE ON escalation_steps TO sr_app;

-- Delegations, revocations and shifts are history: never edited or removed.
CREATE FUNCTION routing_reject_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = 'restrict_violation';
END
$$;
CREATE TRIGGER delegations_no_change BEFORE UPDATE OR DELETE ON delegations
    FOR EACH ROW EXECUTE FUNCTION routing_reject_change();
CREATE TRIGGER delegation_revocations_no_change BEFORE UPDATE OR DELETE ON delegation_revocations
    FOR EACH ROW EXECUTE FUNCTION routing_reject_change();
CREATE TRIGGER on_call_shifts_no_change BEFORE UPDATE OR DELETE ON on_call_shifts
    FOR EACH ROW EXECUTE FUNCTION routing_reject_change();

-- Kill switch at four scopes: global, rail, exception type and policy (v2 S9).
--
-- Freezing is immediate and needs one person. Re-enabling needs two different
-- people: one requests, another approves. State is derived from this
-- append-only log, so who froze what, and who lifted it, is never lost.

CREATE TABLE kill_switch_events (
    seq         bigint      GENERATED ALWAYS AS IDENTITY,
    tenant_id   uuid        NOT NULL REFERENCES institutions (id),
    scope       text        NOT NULL,
    target      text        NOT NULL DEFAULT '',
    kind        text        NOT NULL,
    actor_id    uuid        NOT NULL,
    reason      text        NOT NULL,
    request_seq bigint,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, seq),
    CONSTRAINT kill_switch_scope CHECK (scope IN ('global', 'rail', 'exception_type', 'policy')),
    CONSTRAINT kill_switch_kind CHECK (kind IN ('freeze', 'unfreeze_request', 'unfreeze_approve', 'unfreeze_reject')),
    CONSTRAINT kill_switch_target_matches_scope CHECK (
        (scope = 'global' AND target = '') OR
        (scope = 'rail' AND target ~ '^[A-Z0-9_]{2,30}$') OR
        (scope = 'exception_type' AND target ~ '^[A-Z0-9_-]{3,30}$') OR
        (scope = 'policy' AND target ~ '^[A-Za-z0-9._-]{1,60}$')),
    CONSTRAINT kill_switch_reason_not_empty CHECK (btrim(reason) <> ''),
    CONSTRAINT kill_switch_decision_has_request CHECK (
        (kind IN ('unfreeze_approve', 'unfreeze_reject')) = (request_seq IS NOT NULL)),
    FOREIGN KEY (tenant_id, request_seq) REFERENCES kill_switch_events (tenant_id, seq)
);

CREATE INDEX kill_switch_events_state_idx ON kill_switch_events (tenant_id, scope, target, seq DESC);

ALTER TABLE kill_switch_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON kill_switch_events
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT ON kill_switch_events TO sr_app;
GRANT USAGE ON SEQUENCE kill_switch_events_seq_seq TO sr_app;

CREATE FUNCTION kill_switch_reject_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'kill_switch_events is append-only' USING ERRCODE = 'restrict_violation';
END
$$;
CREATE TRIGGER kill_switch_no_change BEFORE UPDATE OR DELETE ON kill_switch_events
    FOR EACH ROW EXECUTE FUNCTION kill_switch_reject_change();
CREATE TRIGGER kill_switch_no_truncate BEFORE TRUNCATE ON kill_switch_events
    FOR EACH STATEMENT EXECUTE FUNCTION kill_switch_reject_change();

-- The database refuses a re-enable that breaks dual control, so the rule holds
-- even if the application is wrong: a decision must refer to an open request
-- for the same scope, and must come from someone other than the requester.
CREATE FUNCTION kill_switch_check() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    req kill_switch_events%ROWTYPE;
BEGIN
    IF NEW.kind IN ('unfreeze_approve', 'unfreeze_reject') THEN
        SELECT * INTO req FROM kill_switch_events
         WHERE tenant_id = NEW.tenant_id AND seq = NEW.request_seq AND kind = 'unfreeze_request';
        IF NOT FOUND THEN
            RAISE EXCEPTION 'no such unfreeze request' USING ERRCODE = 'integrity_constraint_violation';
        END IF;
        IF req.scope <> NEW.scope OR req.target <> NEW.target THEN
            RAISE EXCEPTION 'the decision is for a different scope than the request' USING ERRCODE = 'integrity_constraint_violation';
        END IF;
        IF req.actor_id = NEW.actor_id THEN
            RAISE EXCEPTION 'the requester cannot decide their own unfreeze request' USING ERRCODE = 'restrict_violation';
        END IF;
        IF EXISTS (SELECT 1 FROM kill_switch_events
                    WHERE tenant_id = NEW.tenant_id AND request_seq = NEW.request_seq) THEN
            RAISE EXCEPTION 'this unfreeze request was already decided' USING ERRCODE = 'unique_violation';
        END IF;
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER kill_switch_dual_control BEFORE INSERT ON kill_switch_events
    FOR EACH ROW EXECUTE FUNCTION kill_switch_check();

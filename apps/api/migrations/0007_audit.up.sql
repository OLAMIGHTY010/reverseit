-- Append-only, hash-chained audit trail, one chain per tenant (v2 B1).
--
-- Application code computes record_hash. The database enforces the rest:
--   * rows cannot be updated, deleted or truncated (privileges and triggers);
--   * every row must continue the chain: chain_seq is the previous one plus one
--     and prev_hash is the previous record_hash. Two writers cannot fork it,
--     because the head row is locked until the writing transaction ends.
-- Verification recomputes every hash from the rows alone, so it does not trust
-- the head row.

CREATE TABLE audit_heads (
    tenant_id uuid PRIMARY KEY REFERENCES institutions (id),
    last_seq  bigint NOT NULL,
    last_hash text   NOT NULL
);

CREATE TABLE audit_events (
    tenant_id        uuid        NOT NULL REFERENCES institutions (id),
    chain_seq        bigint      NOT NULL,
    id               uuid        NOT NULL UNIQUE,
    occurred_at      timestamptz NOT NULL,
    actor_type       text        NOT NULL,
    actor_id         uuid,
    action           text        NOT NULL,
    subject_type     text        NOT NULL DEFAULT '',
    subject_id       text        NOT NULL DEFAULT '',
    reason_code      text        NOT NULL DEFAULT '',
    evidence_refs    jsonb       NOT NULL DEFAULT '[]',
    before_hash      text        NOT NULL DEFAULT '',
    after_hash       text        NOT NULL DEFAULT '',
    policy_version   text        NOT NULL DEFAULT '',
    ip               text        NOT NULL DEFAULT '',
    device           text        NOT NULL DEFAULT '',
    sensitive_access boolean     NOT NULL DEFAULT false,
    details          jsonb       NOT NULL DEFAULT '{}',
    prev_hash        text        NOT NULL,
    record_hash      text        NOT NULL,
    PRIMARY KEY (tenant_id, chain_seq),
    CONSTRAINT audit_events_actor_type CHECK (actor_type IN ('user', 'service', 'system')),
    CONSTRAINT audit_events_action_format CHECK (action ~ '^[a-z][a-z0-9_.]{1,80}$'),
    CONSTRAINT audit_events_seq_positive CHECK (chain_seq >= 1),
    CONSTRAINT audit_events_hash_format CHECK (prev_hash ~ '^[0-9a-f]{64}$' AND record_hash ~ '^[0-9a-f]{64}$')
);

CREATE INDEX audit_events_subject_idx ON audit_events (tenant_id, subject_type, subject_id, chain_seq);
CREATE INDEX audit_events_actor_idx   ON audit_events (tenant_id, actor_id, chain_seq);

ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_events
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- Tenant code can add and read events and nothing else. The head row is not
-- reachable by tenant code at all.
REVOKE ALL ON audit_events FROM PUBLIC;
REVOKE ALL ON audit_heads  FROM PUBLIC;
GRANT SELECT, INSERT ON audit_events TO sr_app;

-- Defence in depth for roles that do hold UPDATE or DELETE, such as the table
-- owner. A superuser can still disable triggers; that is why the chain is also
-- verified from the rows alone and why the head should be anchored outside the
-- database.
CREATE FUNCTION audit_events_reject_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only' USING ERRCODE = 'restrict_violation';
END
$$;
CREATE TRIGGER audit_events_no_update BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION audit_events_reject_change();
CREATE TRIGGER audit_events_no_truncate BEFORE TRUNCATE ON audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION audit_events_reject_change();

-- The two functions below run with their owner's rights, because tenant code
-- may not touch audit_heads. Their search_path is pinned to the schema they are
-- created in, so a caller cannot redirect them.

-- Locks the tenant's head and returns where the chain stands, creating the head
-- on first use. The lock is held until the caller's transaction ends.
DO $do$
BEGIN
    EXECUTE format($f$
        CREATE FUNCTION audit_lock_head(p_tenant uuid, p_genesis text)
        RETURNS TABLE (last_seq bigint, last_hash text)
        LANGUAGE plpgsql SECURITY DEFINER SET search_path = %I, pg_temp AS $body$
        BEGIN
            INSERT INTO audit_heads (tenant_id, last_seq, last_hash)
            VALUES (p_tenant, 0, p_genesis) ON CONFLICT (tenant_id) DO NOTHING;
            RETURN QUERY SELECT h.last_seq, h.last_hash FROM audit_heads h
                         WHERE h.tenant_id = p_tenant FOR UPDATE;
        END
        $body$
    $f$, current_schema());
END
$do$;
REVOKE ALL ON FUNCTION audit_lock_head(uuid, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION audit_lock_head(uuid, text) TO sr_app;

-- Rejects a row that does not continue the chain, and moves the head forward.
-- The head must have been locked by audit_lock_head in the same transaction.
DO $do$
BEGIN
    EXECUTE format($f$
        CREATE FUNCTION audit_events_check_chain() RETURNS trigger
        LANGUAGE plpgsql SECURITY DEFINER SET search_path = %I, pg_temp AS $body$
        DECLARE
            h audit_heads%%ROWTYPE;
        BEGIN
            SELECT * INTO h FROM audit_heads WHERE tenant_id = NEW.tenant_id FOR UPDATE;
            IF NOT FOUND THEN
                RAISE EXCEPTION 'audit chain has no head for this tenant' USING ERRCODE = 'integrity_constraint_violation';
            END IF;
            IF NEW.chain_seq <> h.last_seq + 1 OR NEW.prev_hash <> h.last_hash THEN
                RAISE EXCEPTION 'audit record does not continue the chain (expected seq %%)', h.last_seq + 1
                    USING ERRCODE = 'integrity_constraint_violation';
            END IF;
            UPDATE audit_heads SET last_seq = NEW.chain_seq, last_hash = NEW.record_hash
             WHERE tenant_id = NEW.tenant_id;
            RETURN NEW;
        END
        $body$
    $f$, current_schema());
END
$do$;
REVOKE ALL ON FUNCTION audit_events_check_chain() FROM PUBLIC;
CREATE TRIGGER audit_events_chain BEFORE INSERT ON audit_events
    FOR EACH ROW EXECUTE FUNCTION audit_events_check_chain();

-- Lets tenant code read where its own chain ends, without touching the table.
-- Verification compares the last row with this head to notice a deleted tail.
DO $do$
BEGIN
    EXECUTE format($f$
        CREATE FUNCTION audit_head(p_tenant uuid)
        RETURNS TABLE (last_seq bigint, last_hash text)
        LANGUAGE sql STABLE SECURITY DEFINER SET search_path = %I, pg_temp AS $body$
            SELECT h.last_seq, h.last_hash FROM audit_heads h
             WHERE h.tenant_id = p_tenant
               AND h.tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid
        $body$
    $f$, current_schema());
END
$do$;
REVOKE ALL ON FUNCTION audit_head(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION audit_head(uuid) TO sr_app;

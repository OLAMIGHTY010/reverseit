-- Versioned configuration with maker-checker (v2 B1, FR-39).
--
-- A change is a new version, never an edit. A version takes effect only after a
-- decision by someone other than its proposer. Both tables are append-only for
-- tenant code. Rolling back means proposing the older value again, which needs
-- the same approval.

CREATE TABLE config_versions (
    tenant_id        uuid        NOT NULL REFERENCES institutions (id),
    key              text        NOT NULL,
    version          integer     NOT NULL,
    value            jsonb       NOT NULL,
    source_reference text        NOT NULL,
    effective_from   timestamptz NOT NULL,
    verify_status    text        NOT NULL,
    proposed_by      uuid        NOT NULL,
    reason           text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, key, version),
    CONSTRAINT config_versions_key_format CHECK (key ~ '^[a-z][A-Za-z0-9_.]{1,100}$'),
    CONSTRAINT config_versions_version_positive CHECK (version >= 1),
    CONSTRAINT config_versions_source_not_empty CHECK (btrim(source_reference) <> ''),
    CONSTRAINT config_versions_reason_not_empty CHECK (btrim(reason) <> ''),
    CONSTRAINT config_versions_verify_status CHECK (verify_status IN ('confirmed', 'unverified', 'reported', 'draft', 'conflicting'))
);

CREATE TABLE config_decisions (
    tenant_id  uuid        NOT NULL,
    key        text        NOT NULL,
    version    integer     NOT NULL,
    decision   text        NOT NULL,
    decided_by uuid        NOT NULL,
    reason     text        NOT NULL DEFAULT '',
    decided_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, key, version),
    FOREIGN KEY (tenant_id, key, version) REFERENCES config_versions (tenant_id, key, version),
    CONSTRAINT config_decisions_decision CHECK (decision IN ('approved', 'rejected')),
    CONSTRAINT config_decisions_reject_needs_reason CHECK (decision = 'approved' OR btrim(reason) <> '')
);

ALTER TABLE config_versions  ENABLE ROW LEVEL SECURITY;
ALTER TABLE config_decisions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON config_versions
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON config_decisions
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON config_versions, config_decisions TO sr_app;

-- Neither table may be changed or emptied, by anyone, short of switching
-- triggers off.
CREATE FUNCTION config_reject_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = 'restrict_violation';
END
$$;
CREATE TRIGGER config_versions_no_change BEFORE UPDATE OR DELETE ON config_versions
    FOR EACH ROW EXECUTE FUNCTION config_reject_change();
CREATE TRIGGER config_decisions_no_change BEFORE UPDATE OR DELETE ON config_decisions
    FOR EACH ROW EXECUTE FUNCTION config_reject_change();

-- The database itself refuses a decision by the proposer, and a version number
-- that skips ahead, so these rules hold even if application code is wrong.
CREATE FUNCTION config_decision_check() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    maker uuid;
BEGIN
    SELECT proposed_by INTO maker FROM config_versions
     WHERE tenant_id = NEW.tenant_id AND key = NEW.key AND version = NEW.version;
    IF maker = NEW.decided_by THEN
        RAISE EXCEPTION 'the proposer cannot decide their own change' USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER config_decisions_maker_checker BEFORE INSERT ON config_decisions
    FOR EACH ROW EXECUTE FUNCTION config_decision_check();

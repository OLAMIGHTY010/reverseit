-- Declarative detection rules and their lifecycle (docs/backend/detection-rules.md).
-- A version's definition is immutable. Only its status and the review fields change.

CREATE TABLE detection_rules (
    tenant_id                uuid        NOT NULL REFERENCES institutions (id),
    rule_id                  text        NOT NULL,
    version                  integer     NOT NULL,
    definition               jsonb       NOT NULL,
    definition_hash          text        NOT NULL,
    status                   text        NOT NULL DEFAULT 'draft',
    created_by               uuid        NOT NULL,
    created_at               timestamptz NOT NULL DEFAULT now(),
    shadow_started_at        timestamptz,
    activation_requested_by  uuid,
    activation_requested_at  timestamptz,
    activation_reason        text        NOT NULL DEFAULT '',
    decided_by               uuid,
    decided_at               timestamptz,
    decision_reason          text        NOT NULL DEFAULT '',
    activated_at             timestamptz,
    retired_at               timestamptz,
    retired_by               uuid,
    retired_reason           text        NOT NULL DEFAULT '',
    PRIMARY KEY (tenant_id, rule_id, version),
    CONSTRAINT detection_rules_status CHECK (status IN ('draft', 'shadow', 'pending_activation', 'active', 'retired', 'rejected')),
    CONSTRAINT detection_rules_version CHECK (version >= 1),
    -- Whoever decides is not whoever made or asked for the change.
    CONSTRAINT detection_rules_four_eyes CHECK (decided_by IS NULL OR (decided_by <> created_by AND decided_by IS DISTINCT FROM activation_requested_by))
);
-- At most one version of a rule is live.
CREATE UNIQUE INDEX detection_rules_one_active ON detection_rules (tenant_id, rule_id) WHERE status = 'active';

CREATE FUNCTION detection_rules_definition_is_fixed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.definition IS DISTINCT FROM OLD.definition OR NEW.definition_hash <> OLD.definition_hash
       OR NEW.rule_id <> OLD.rule_id OR NEW.version <> OLD.version OR NEW.created_by <> OLD.created_by THEN
        RAISE EXCEPTION 'a rule version cannot be edited; save a new version' USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER detection_rules_fixed BEFORE UPDATE ON detection_rules
    FOR EACH ROW EXECUTE FUNCTION detection_rules_definition_is_fixed();

-- What each rule concluded about each transaction, in shadow or live, so a rule can be
-- judged before it is trusted and a missing case can be explained afterwards.
CREATE TABLE rule_evaluations (
    tenant_id    uuid        NOT NULL REFERENCES institutions (id),
    rule_id      text        NOT NULL,
    version      integer     NOT NULL,
    txn_id       uuid        NOT NULL,
    mode         text        NOT NULL,
    outcome      text        NOT NULL,
    held_reason  text        NOT NULL DEFAULT '',
    evaluated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, rule_id, version, txn_id, mode),
    FOREIGN KEY (tenant_id, rule_id, version) REFERENCES detection_rules (tenant_id, rule_id, version),
    CONSTRAINT rule_evaluations_mode CHECK (mode IN ('shadow', 'active', 'backtest')),
    CONSTRAINT rule_evaluations_outcome CHECK (outcome IN ('matched', 'no_match', 'held'))
);
CREATE INDEX rule_evaluations_rule_idx ON rule_evaluations (tenant_id, rule_id, version, outcome);

ALTER TABLE detection_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE rule_evaluations ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON detection_rules USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON rule_evaluations USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE ON detection_rules, rule_evaluations TO sr_app;

-- Work held back by the kill switch (docs/backend/kill-switch-enforcement.md).
-- Nothing a freeze blocks is thrown away: detection that could not proceed is
-- recorded here and picked up again after an authorised unfreeze, and a reversal
-- that was approved but not yet submitted is marked parked.

CREATE TABLE deferred_work (
    tenant_id       uuid        NOT NULL REFERENCES institutions (id),
    id              uuid        NOT NULL DEFAULT gen_random_uuid(),
    kind            text        NOT NULL,
    subject_id      uuid        NOT NULL,
    blocked_scope   text        NOT NULL,
    blocked_target  text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_blocked_at timestamptz NOT NULL DEFAULT now(),
    attempts        integer     NOT NULL DEFAULT 0,
    resumed_at      timestamptz,
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT deferred_work_kind CHECK (kind IN ('detect')),
    CONSTRAINT deferred_work_attempts CHECK (attempts >= 0)
);
-- One open deferral per subject, however many times it is blocked.
CREATE UNIQUE INDEX deferred_work_one_open ON deferred_work (tenant_id, kind, subject_id) WHERE resumed_at IS NULL;
CREATE INDEX deferred_work_open_idx ON deferred_work (tenant_id, kind, created_at) WHERE resumed_at IS NULL;

-- What each scheduled job is doing, per institution, so a pause can be seen.
CREATE TABLE automation_jobs (
    tenant_id   uuid        NOT NULL REFERENCES institutions (id),
    job         text        NOT NULL,
    state       text        NOT NULL,
    frozen      text        NOT NULL DEFAULT '',
    since       timestamptz NOT NULL DEFAULT now(),
    last_run_at timestamptz,
    last_result jsonb       NOT NULL DEFAULT '{}',
    PRIMARY KEY (tenant_id, job),
    CONSTRAINT automation_jobs_state CHECK (state IN ('running', 'paused'))
);

ALTER TABLE reversals
    ADD COLUMN parked_at     timestamptz,
    ADD COLUMN parked_scope  text NOT NULL DEFAULT '',
    ADD COLUMN parked_target text NOT NULL DEFAULT '';

ALTER TABLE deferred_work   ENABLE ROW LEVEL SECURITY;
ALTER TABLE automation_jobs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON deferred_work USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON automation_jobs USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE ON deferred_work, automation_jobs TO sr_app;

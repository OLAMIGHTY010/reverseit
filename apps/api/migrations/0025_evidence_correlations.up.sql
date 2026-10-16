-- One row per piece of evidence from a source that does not know our instruction
-- reference: linked automatically or by a person, or waiting in the open queue
-- (docs/backend/correlation.md).

CREATE TABLE evidence_correlations (
    tenant_id        uuid        NOT NULL REFERENCES institutions (id),
    id               uuid        NOT NULL DEFAULT gen_random_uuid(),
    source_system    text        NOT NULL,
    source_event_ref text        NOT NULL,
    observation      jsonb       NOT NULL,
    observation_hash text        NOT NULL,
    reason           text        NOT NULL,
    candidates       jsonb       NOT NULL DEFAULT '[]',
    status           text        NOT NULL DEFAULT 'open',
    linked_txn_id    uuid,
    link_method      text        NOT NULL DEFAULT '',
    confidence       numeric(4,3),
    resolved_by      uuid,
    resolved_reason  text        NOT NULL DEFAULT '',
    resolved_at      timestamptz,
    attempts         integer     NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, source_system, source_event_ref),
    FOREIGN KEY (tenant_id, linked_txn_id) REFERENCES transactions (tenant_id, id),
    CONSTRAINT evidence_correlations_status CHECK (status IN ('open', 'linked', 'discarded')),
    CONSTRAINT evidence_correlations_method CHECK (link_method IN ('', 'automatic', 'manual'))
);
CREATE INDEX evidence_correlations_open_idx ON evidence_correlations (tenant_id, created_at) WHERE status = 'open';

ALTER TABLE evidence_correlations ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON evidence_correlations USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE ON evidence_correlations TO sr_app;

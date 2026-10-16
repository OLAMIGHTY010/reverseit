-- Incidents group exceptions that share a rail, channel and error signature inside a
-- time window (docs/backend/alerts-and-incidents.md). A case belongs to at most one.

CREATE SEQUENCE incident_number_seq;

CREATE TABLE incidents (
    tenant_id        uuid        NOT NULL REFERENCES institutions (id),
    id               uuid        NOT NULL DEFAULT gen_random_uuid(),
    incident_ref     text        NOT NULL,
    rail             text        NOT NULL,
    channel          text        NOT NULL,
    signature        text        NOT NULL,
    status           text        NOT NULL DEFAULT 'open',
    severity         text        NOT NULL,
    start_at         timestamptz NOT NULL,
    last_seen_at     timestamptz NOT NULL,
    affected_count   integer     NOT NULL DEFAULT 0,
    exposure         jsonb       NOT NULL DEFAULT '{}',
    suspected_cause  text        NOT NULL DEFAULT '',
    causation_status text        NOT NULL DEFAULT 'correlated',
    causation_note   text        NOT NULL DEFAULT '',
    reviewed_by      uuid,
    review_note      text        NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    resolved_at      timestamptz,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, incident_ref),
    CONSTRAINT incidents_status CHECK (status IN ('open', 'resolved')),
    CONSTRAINT incidents_severity CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    CONSTRAINT incidents_causation CHECK (causation_status IN ('correlated', 'probable', 'confirmed'))
);
-- One open incident per scope: a continuing spike joins it instead of starting another.
CREATE UNIQUE INDEX incidents_one_open ON incidents (tenant_id, rail, channel, signature) WHERE status = 'open';

CREATE TABLE incident_cases (
    tenant_id   uuid        NOT NULL,
    incident_id uuid        NOT NULL,
    case_id     uuid        NOT NULL,
    linked_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, incident_id, case_id),
    UNIQUE (tenant_id, case_id),
    FOREIGN KEY (tenant_id, incident_id) REFERENCES incidents (tenant_id, id),
    FOREIGN KEY (tenant_id, case_id) REFERENCES cases (tenant_id, id)
);

ALTER TABLE incidents      ENABLE ROW LEVEL SECURITY;
ALTER TABLE incident_cases ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON incidents USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON incident_cases USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE ON incidents TO sr_app;
GRANT SELECT, INSERT ON incident_cases TO sr_app;
GRANT USAGE ON SEQUENCE incident_number_seq TO sr_app;

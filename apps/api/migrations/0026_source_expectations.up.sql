-- How often each source is expected to send events, so silence can be told from
-- normal quiet (docs/backend/source-health.md). Nothing is assumed: a source with no
-- expectation is reported as unconfigured.

CREATE TABLE source_expectations (
    tenant_id          uuid        NOT NULL REFERENCES institutions (id),
    source_system      text        NOT NULL,
    stale_after_secs   integer     NOT NULL,
    down_after_secs    integer     NOT NULL,
    max_skew_secs      integer     NOT NULL,
    updated_by         uuid,
    updated_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, source_system),
    CONSTRAINT source_expectations_order CHECK (stale_after_secs > 0 AND down_after_secs > stale_after_secs AND max_skew_secs >= 0),
    CONSTRAINT source_expectations_bounds CHECK (down_after_secs <= 2592000 AND max_skew_secs <= 86400)
);
ALTER TABLE source_expectations ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON source_expectations USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE ON source_expectations TO sr_app;

-- Freshness reads the newest events of a source.
CREATE INDEX IF NOT EXISTS transaction_events_source_idx ON transaction_events (tenant_id, source_system, received_at DESC);

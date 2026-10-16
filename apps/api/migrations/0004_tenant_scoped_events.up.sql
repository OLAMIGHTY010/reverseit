-- Events belong to a tenant (ADR 0003). The relay publishes across tenants and
-- runs as the connection's own role. Tenant code runs as sr_app and sees only
-- its own rows. sr_app can add events but never change or remove them.

ALTER TABLE outbox_events ADD COLUMN tenant_id uuid;
ALTER TABLE outbox_events ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_tenant_fk FOREIGN KEY (tenant_id) REFERENCES institutions (id);
CREATE INDEX outbox_events_tenant_idx ON outbox_events (tenant_id, id);

ALTER TABLE processed_events ADD COLUMN tenant_id uuid;
ALTER TABLE processed_events ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE processed_events ADD CONSTRAINT processed_events_tenant_fk FOREIGN KEY (tenant_id) REFERENCES institutions (id);

ALTER TABLE outbox_events     ENABLE ROW LEVEL SECURITY;
ALTER TABLE processed_events  ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON outbox_events
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON processed_events
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON outbox_events    TO sr_app;
GRANT SELECT, INSERT ON processed_events TO sr_app;
GRANT USAGE ON SEQUENCE outbox_events_id_seq TO sr_app;

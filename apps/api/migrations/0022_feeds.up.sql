-- Versioned file layouts and a record of every file imported (docs/backend/file-ingestion.md).

CREATE TABLE feed_mappings (
    tenant_id  uuid        NOT NULL REFERENCES institutions (id),
    name       text        NOT NULL,
    version    integer     NOT NULL,
    definition jsonb       NOT NULL,
    status     text        NOT NULL DEFAULT 'draft',
    created_by text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, name, version),
    CONSTRAINT feed_mappings_status CHECK (status IN ('draft', 'active', 'retired')),
    CONSTRAINT feed_mappings_version CHECK (version >= 1)
);
-- At most one version of a mapping is in use at a time.
CREATE UNIQUE INDEX feed_mappings_one_active ON feed_mappings (tenant_id, name) WHERE status = 'active';

CREATE TABLE feed_files (
    tenant_id       uuid        NOT NULL REFERENCES institutions (id),
    id              uuid        NOT NULL DEFAULT gen_random_uuid(),
    mapping_name    text        NOT NULL,
    mapping_version integer     NOT NULL,
    file_hash       text        NOT NULL,
    size_bytes      bigint      NOT NULL,
    rows_total      integer     NOT NULL DEFAULT 0,
    rows_accepted   integer     NOT NULL DEFAULT 0,
    rows_duplicate  integer     NOT NULL DEFAULT 0,
    rows_rejected   integer     NOT NULL DEFAULT 0,
    status          text        NOT NULL,
    imported_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    -- The same bytes are imported once. A second upload reports the first result.
    CONSTRAINT feed_files_once UNIQUE (tenant_id, mapping_name, file_hash),
    CONSTRAINT feed_files_status CHECK (status IN ('imported', 'imported_with_errors', 'refused'))
);

ALTER TABLE feed_mappings ENABLE ROW LEVEL SECURITY;
ALTER TABLE feed_files    ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON feed_mappings USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON feed_files USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE ON feed_mappings TO sr_app;
GRANT SELECT, INSERT ON feed_files TO sr_app;

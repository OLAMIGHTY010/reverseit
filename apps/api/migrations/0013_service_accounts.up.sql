-- Service-to-service authentication (v2 B1, item 2).
--
-- Each calling service has its own Ed25519 key pair. It signs a short-lived
-- assertion on every call; the API holds only the public key. Scopes live here,
-- not in the token, so a token can never claim more than the account allows.
-- Accounts are per tenant and are disabled, not deleted.

CREATE TABLE service_accounts (
    tenant_id  uuid        NOT NULL REFERENCES institutions (id),
    id         uuid        NOT NULL DEFAULT gen_random_uuid(),
    name       text        NOT NULL,
    public_key bytea       NOT NULL,
    scopes     text[]      NOT NULL,
    status     text        NOT NULL DEFAULT 'active',
    created_by uuid        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, name),
    CONSTRAINT service_accounts_name_format CHECK (name ~ '^[a-z][a-z0-9-]{1,40}$'),
    CONSTRAINT service_accounts_key_length CHECK (octet_length(public_key) = 32),
    CONSTRAINT service_accounts_status CHECK (status IN ('active', 'disabled')),
    CONSTRAINT service_accounts_scopes_present CHECK (cardinality(scopes) BETWEEN 1 AND 20)
);

-- Used assertion ids, kept until the assertion would have expired anyway, so a
-- captured assertion cannot be replayed.
CREATE TABLE service_assertion_uses (
    tenant_id  uuid        NOT NULL,
    service_id uuid        NOT NULL,
    jti        text        NOT NULL,
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, service_id, jti),
    FOREIGN KEY (tenant_id, service_id) REFERENCES service_accounts (tenant_id, id),
    CONSTRAINT service_assertion_jti_length CHECK (char_length(jti) BETWEEN 8 AND 64)
);
CREATE INDEX service_assertion_uses_expiry ON service_assertion_uses (expires_at);

ALTER TABLE service_accounts        ENABLE ROW LEVEL SECURITY;
ALTER TABLE service_assertion_uses  ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON service_accounts
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON service_assertion_uses
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON service_accounts TO sr_app;
GRANT UPDATE (status) ON service_accounts TO sr_app;
GRANT SELECT, INSERT, DELETE ON service_assertion_uses TO sr_app;

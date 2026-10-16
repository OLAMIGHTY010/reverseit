-- Sealed account numbers (ADR 0003, D10). Everything else in the system refers to
-- an account by its token. The number itself exists only here, encrypted, and is
-- released through the audited unmask call.

CREATE TABLE account_vault (
    tenant_id      uuid        NOT NULL REFERENCES institutions (id),
    token          text        NOT NULL,
    masked_display text        NOT NULL,
    ciphertext     bytea       NOT NULL,
    key_id         text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, token),
    CONSTRAINT account_vault_token_format CHECK (token ~ '^acct_[0-9a-f]{32}$'),
    CONSTRAINT account_vault_masked_format CHECK (masked_display ~ '^\*{4}[A-Z0-9]{4}$')
);

ALTER TABLE account_vault ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON account_vault
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- Added and read, never changed or removed by tenant code.
GRANT SELECT, INSERT ON account_vault TO sr_app;

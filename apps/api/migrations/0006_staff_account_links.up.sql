-- Links between staff and accounts, for the conflict-of-interest check on
-- approvals (v2 section 6.4: an approver cannot be related to the account or
-- the transaction owner). Accounts are referred to by token, never by number.

CREATE TABLE staff_account_links (
    tenant_id     uuid        NOT NULL,
    user_id       uuid        NOT NULL,
    account_token text        NOT NULL,
    relationship  text        NOT NULL DEFAULT 'holder',
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, account_token),
    FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT staff_account_links_token_not_empty CHECK (account_token <> '')
);

ALTER TABLE staff_account_links ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON staff_account_links
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, DELETE ON staff_account_links TO sr_app;

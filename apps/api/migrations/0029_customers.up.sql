-- ReverseIt customers and the accounts they own (docs/backend/customer-backend.md).
-- A customer is not an operator: separate table, separate token audience. Which
-- accounts a customer owns is established by the bank, never by anything the customer
-- pastes. The development login below is for synthetic customers only; a production
-- identity provider replaces it behind the same interface.

CREATE TABLE customers (
    tenant_id       uuid        NOT NULL REFERENCES institutions (id),
    id              uuid        NOT NULL DEFAULT gen_random_uuid(),
    customer_ref    text        NOT NULL,
    display_name    text        NOT NULL DEFAULT '',
    password_hash   text        NOT NULL DEFAULT '',
    status          text        NOT NULL DEFAULT 'active',
    failed_attempts integer     NOT NULL DEFAULT 0,
    locked_until    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, customer_ref),
    CONSTRAINT customers_status CHECK (status IN ('active', 'disabled')),
    CONSTRAINT customers_ref_format CHECK (customer_ref ~ '^cust_[0-9a-f]{24}$')
);

CREATE TABLE customer_accounts (
    tenant_id     uuid        NOT NULL,
    customer_id   uuid        NOT NULL,
    account_token text        NOT NULL,
    linked_at     timestamptz NOT NULL DEFAULT now(),
    linked_by     text        NOT NULL DEFAULT '',
    PRIMARY KEY (tenant_id, customer_id, account_token),
    FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id),
    CONSTRAINT customer_accounts_token CHECK (account_token ~ '^acct_[0-9a-f]{32}$')
);
CREATE INDEX customer_accounts_token_idx ON customer_accounts (tenant_id, account_token);

ALTER TABLE customers         ENABLE ROW LEVEL SECURITY;
ALTER TABLE customer_accounts ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON customers USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON customer_accounts USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE ON customers TO sr_app;
GRANT SELECT, INSERT ON customer_accounts TO sr_app;

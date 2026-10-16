-- Tenancy foundation (ADR 0003).
--
-- Tenant work runs as sr_app, a role with no BYPASSRLS, inside a transaction
-- that sets app.tenant_id. Row-level security policies compare tenant_id to
-- that setting. sr_app gets only the grants listed here.

DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'sr_app') THEN
        CREATE ROLE sr_app NOLOGIN NOBYPASSRLS;
    END IF;
END
$$;
GRANT sr_app TO CURRENT_USER;

CREATE TABLE institutions (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    code           text        NOT NULL UNIQUE,
    name           text        NOT NULL,
    timezone       text        NOT NULL DEFAULT 'Africa/Lagos',
    config_version integer     NOT NULL DEFAULT 1,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT institutions_code_format CHECK (code ~ '^[A-Z0-9-]{2,40}$')
);

CREATE TABLE users (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid        NOT NULL REFERENCES institutions (id),
    email           text        NOT NULL,
    display_name    text        NOT NULL,
    password_hash   text        NOT NULL,
    status          text        NOT NULL DEFAULT 'active',
    failed_attempts integer     NOT NULL DEFAULT 0,
    locked_until    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_status CHECK (status IN ('active', 'disabled')),
    CONSTRAINT users_email_lower CHECK (email = lower(email)),
    CONSTRAINT users_failed_attempts_non_negative CHECK (failed_attempts >= 0),
    UNIQUE (tenant_id, email),
    UNIQUE (tenant_id, id)
);

CREATE TABLE user_roles (
    tenant_id uuid NOT NULL,
    user_id   uuid NOT NULL,
    role      text NOT NULL,
    PRIMARY KEY (user_id, role),
    FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT user_roles_role_format CHECK (role ~ '^[A-Z_]{3,40}$')
);

ALTER TABLE institutions ENABLE ROW LEVEL SECURITY;
ALTER TABLE users        ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_roles   ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON institutions
    USING (id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON users
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation ON user_roles
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT ON institutions TO sr_app;
GRANT SELECT, INSERT, UPDATE ON users TO sr_app;
GRANT SELECT, INSERT, DELETE ON user_roles TO sr_app;

-- Login needs the tenant id before a tenant context exists. This function
-- returns only the id for a code and nothing else, and runs with its owner's
-- rights so it can read past the policy. Its search_path is pinned to the
-- schema it is created in, so a caller cannot redirect it.
DO $$
BEGIN
    EXECUTE format(
        'CREATE FUNCTION institution_id_by_code(p_code text) RETURNS uuid
            LANGUAGE sql STABLE SECURITY DEFINER SET search_path = %I
            AS $f$ SELECT id FROM institutions WHERE code = p_code $f$',
        current_schema());
END
$$;
REVOKE ALL ON FUNCTION institution_id_by_code(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION institution_id_by_code(text) TO sr_app;

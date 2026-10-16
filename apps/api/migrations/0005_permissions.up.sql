-- Role and permission matrix as data (v2 prompt pack, section 6.4).
--
-- A missing row means "no access". Roles and capability names are the
-- project's working names. The matrix itself says to verify thresholds and role
-- names against the institution's delegated-authority matrix, so every role
-- carries a verify_status that starts as 'unverified'.

CREATE TABLE roles (
    role          text PRIMARY KEY,
    description   text NOT NULL,
    verify_status text NOT NULL DEFAULT 'unverified',
    CONSTRAINT roles_format CHECK (role ~ '^[A-Z_]{3,40}$'),
    CONSTRAINT roles_verify_status CHECK (verify_status IN ('unverified', 'confirmed'))
);

CREATE TABLE capabilities (
    capability   text PRIMARY KEY,
    description  text    NOT NULL,
    requires_mfa boolean NOT NULL DEFAULT false,
    CONSTRAINT capabilities_format CHECK (capability ~ '^[a-z_]{3,60}$')
);

CREATE TABLE permission_grants (
    role       text NOT NULL REFERENCES roles (role),
    capability text NOT NULL REFERENCES capabilities (capability),
    scope      text NOT NULL,
    PRIMARY KEY (role, capability),
    CONSTRAINT permission_grants_scope CHECK (scope IN (
        'full', 'metadata', 'aggregate', 'own_customer', 'reason_gated', 'notes_only',
        'within_limit', 'higher_limit', 'override_dual', 'propose', 'approve', 'draft',
        'own', 'team', 'all_ops', 'all', 'system', 'review', 'dual_control', 'masked', 'controlled'))
);

INSERT INTO roles (role, description) VALUES
    ('TXN_ANALYST',      'Transaction analyst'),
    ('RECON_ANALYST',    'Reconciliation analyst'),
    ('REVERSAL_OFFICER', 'Reversal officer (maker)'),
    ('SUPERVISOR',       'Supervisor'),
    ('HEAD_OPS',         'Head of operations'),
    ('RISK_COMPLIANCE',  'Risk and compliance'),
    ('ADMIN_INTEGRATION','Administrator and integration engineer'),
    ('EXECUTIVE',        'Management and executive (aggregates only)'),
    ('AGENT',            'Customer service agent');

INSERT INTO capabilities (capability, description, requires_mfa) VALUES
    ('view_exception_queue',    'View the exception queue',                         false),
    ('view_unmasked_account',   'View unmasked account data',                       true),
    ('add_notes_evidence',      'Add notes and evidence to a case',                 false),
    ('request_reversal',        'Request a reversal (maker)',                       false),
    ('approve_reversal',        'Approve a reversal (checker)',                     true),
    ('override_gate',           'Manual override of the reversal gate',             true),
    ('change_rules',            'Change rules and policies',                        true),
    ('trigger_kill_switch',     'Freeze automation with the kill switch',           false),
    ('view_audit',              'View the audit trail',                             false),
    ('manage_users',            'Manage users and roles',                           true),
    ('export_data',             'Export data',                                      false);

INSERT INTO permission_grants (role, capability, scope) VALUES
    -- view_exception_queue
    ('TXN_ANALYST','view_exception_queue','full'), ('RECON_ANALYST','view_exception_queue','full'),
    ('REVERSAL_OFFICER','view_exception_queue','full'), ('SUPERVISOR','view_exception_queue','full'),
    ('HEAD_OPS','view_exception_queue','full'), ('RISK_COMPLIANCE','view_exception_queue','full'),
    ('ADMIN_INTEGRATION','view_exception_queue','metadata'), ('EXECUTIVE','view_exception_queue','aggregate'),
    ('AGENT','view_exception_queue','own_customer'),
    -- view_unmasked_account
    ('TXN_ANALYST','view_unmasked_account','reason_gated'), ('RECON_ANALYST','view_unmasked_account','reason_gated'),
    ('REVERSAL_OFFICER','view_unmasked_account','reason_gated'), ('SUPERVISOR','view_unmasked_account','reason_gated'),
    ('RISK_COMPLIANCE','view_unmasked_account','reason_gated'),
    -- add_notes_evidence
    ('TXN_ANALYST','add_notes_evidence','full'), ('RECON_ANALYST','add_notes_evidence','full'),
    ('REVERSAL_OFFICER','add_notes_evidence','full'), ('SUPERVISOR','add_notes_evidence','full'),
    ('RISK_COMPLIANCE','add_notes_evidence','full'), ('AGENT','add_notes_evidence','notes_only'),
    -- request_reversal
    ('REVERSAL_OFFICER','request_reversal','full'), ('SUPERVISOR','request_reversal','full'),
    -- approve_reversal
    ('REVERSAL_OFFICER','approve_reversal','within_limit'), ('SUPERVISOR','approve_reversal','within_limit'),
    ('HEAD_OPS','approve_reversal','higher_limit'),
    -- override_gate
    ('SUPERVISOR','override_gate','override_dual'), ('HEAD_OPS','override_gate','override_dual'),
    -- change_rules
    ('SUPERVISOR','change_rules','propose'), ('HEAD_OPS','change_rules','propose'),
    ('RISK_COMPLIANCE','change_rules','approve'), ('ADMIN_INTEGRATION','change_rules','draft'),
    -- trigger_kill_switch
    ('REVERSAL_OFFICER','trigger_kill_switch','full'), ('SUPERVISOR','trigger_kill_switch','full'),
    ('HEAD_OPS','trigger_kill_switch','full'), ('RISK_COMPLIANCE','trigger_kill_switch','full'),
    ('ADMIN_INTEGRATION','trigger_kill_switch','full'),
    -- view_audit
    ('TXN_ANALYST','view_audit','own'), ('RECON_ANALYST','view_audit','own'), ('REVERSAL_OFFICER','view_audit','own'),
    ('SUPERVISOR','view_audit','team'), ('HEAD_OPS','view_audit','all_ops'), ('RISK_COMPLIANCE','view_audit','all'),
    ('ADMIN_INTEGRATION','view_audit','system'),
    -- manage_users
    ('RISK_COMPLIANCE','manage_users','review'), ('ADMIN_INTEGRATION','manage_users','dual_control'),
    -- export_data
    ('RECON_ANALYST','export_data','masked'), ('SUPERVISOR','export_data','masked'), ('HEAD_OPS','export_data','masked'),
    ('RISK_COMPLIANCE','export_data','controlled'), ('EXECUTIVE','export_data','aggregate');

-- Reference data: tenant code can read it, nobody but the owner can change it.
GRANT SELECT ON roles, capabilities, permission_grants TO sr_app;

-- A user can only hold a role that exists.
ALTER TABLE user_roles ADD CONSTRAINT user_roles_role_fk FOREIGN KEY (role) REFERENCES roles (role);

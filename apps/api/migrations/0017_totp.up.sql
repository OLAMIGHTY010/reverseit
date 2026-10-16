-- Development-only multi-factor authentication (ADR 0003, D20). The secret is
-- synthetic, enrolled by the bootstrap command, and stored as is. A bank
-- deployment replaces this with its identity provider; nothing here is a claim
-- of production-grade MFA. last_step stops a code from being used twice.
ALTER TABLE users
    ADD COLUMN totp_secret    bytea,
    ADD COLUMN totp_last_step bigint NOT NULL DEFAULT 0;

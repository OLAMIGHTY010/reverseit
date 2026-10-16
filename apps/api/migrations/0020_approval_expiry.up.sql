-- Approvals expire. An approval records when its authority ends (expires_at), a
-- reversal whose approval has lapsed is marked EXPIRED, and a fresh approval can be
-- requested for it, so one reversal may now have several approvals over time.
-- Only one may be waiting for a decision at once.
ALTER TABLE approvals
    ADD COLUMN expires_at timestamptz,
    ADD COLUMN policy_id  text NOT NULL DEFAULT '';

DO $$
DECLARE c text;
BEGIN
    SELECT conname INTO c FROM pg_constraint
     WHERE conrelid = 'approvals'::regclass AND contype = 'u'
       AND pg_get_constraintdef(oid) = 'UNIQUE (tenant_id, reversal_id)';
    IF c IS NOT NULL THEN
        EXECUTE format('ALTER TABLE approvals DROP CONSTRAINT %I', c);
    END IF;
END $$;
CREATE UNIQUE INDEX approvals_one_pending ON approvals (tenant_id, reversal_id) WHERE status = 'PENDING';
CREATE INDEX approvals_reversal_idx ON approvals (tenant_id, reversal_id, created_at DESC);

ALTER TABLE reversals DROP CONSTRAINT reversals_approval;
ALTER TABLE reversals ADD CONSTRAINT reversals_approval
    CHECK (approval_status IN ('NOT_REQUIRED', 'PENDING', 'APPROVED', 'REJECTED', 'EXPIRED'));

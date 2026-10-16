-- Reversals that were marked EXPIRED go back to waiting for approval, and only the
-- newest approval of each reversal is kept, so the old constraints can return.
UPDATE reversals SET approval_status = 'PENDING' WHERE approval_status = 'EXPIRED';
DELETE FROM approvals a USING approvals b
 WHERE a.tenant_id = b.tenant_id AND a.reversal_id = b.reversal_id AND (a.created_at, a.id) < (b.created_at, b.id);
DROP INDEX approvals_reversal_idx;
DROP INDEX approvals_one_pending;
ALTER TABLE approvals ADD CONSTRAINT approvals_tenant_id_reversal_id_key UNIQUE (tenant_id, reversal_id);
ALTER TABLE reversals DROP CONSTRAINT reversals_approval;
ALTER TABLE reversals ADD CONSTRAINT reversals_approval
    CHECK (approval_status IN ('NOT_REQUIRED', 'PENDING', 'APPROVED', 'REJECTED'));
ALTER TABLE approvals DROP COLUMN policy_id, DROP COLUMN expires_at;

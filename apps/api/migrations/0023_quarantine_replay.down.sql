DELETE FROM permission_grants WHERE capability = 'manage_ingestion';
DELETE FROM capabilities WHERE capability = 'manage_ingestion';
REVOKE UPDATE ON quarantined_events FROM sr_app;
DROP INDEX IF EXISTS quarantined_events_open_idx;
ALTER TABLE quarantined_events
    DROP CONSTRAINT IF EXISTS quarantined_events_status,
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS last_reason,
    DROP COLUMN IF EXISTS last_attempt_at,
    DROP COLUMN IF EXISTS attempts,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS payload;

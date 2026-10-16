DROP FUNCTION audit_head(uuid);
DROP TRIGGER audit_events_chain ON audit_events;
DROP FUNCTION audit_events_check_chain();
DROP FUNCTION audit_lock_head(uuid, text);
DROP TRIGGER audit_events_no_truncate ON audit_events;
DROP TRIGGER audit_events_no_update ON audit_events;
DROP FUNCTION audit_events_reject_change();
DROP TABLE audit_events;
DROP TABLE audit_heads;

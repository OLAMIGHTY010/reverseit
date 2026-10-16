DROP TRIGGER notifications_no_change ON notifications;
DROP TRIGGER case_evidence_no_change ON case_evidence;
DROP TRIGGER case_events_no_change ON case_events;
DROP FUNCTION case_history_reject_change();
DROP TABLE notifications;
DROP TABLE case_evidence;
DROP TABLE case_events;
DROP TABLE cases;
DROP SEQUENCE case_number_seq;

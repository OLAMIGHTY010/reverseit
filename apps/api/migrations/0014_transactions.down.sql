DROP TRIGGER transaction_events_no_change ON transaction_events;
DROP FUNCTION transaction_events_reject_change();
DROP TABLE quarantined_events;
DROP TABLE transaction_events;
DROP TABLE transactions;

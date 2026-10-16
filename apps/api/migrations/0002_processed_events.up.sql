-- Idempotent consumption. A consumer records the event id in the same
-- transaction as its own writes, so a redelivered event is detected and skipped
-- and a failed handler leaves no record.
CREATE TABLE processed_events (
    consumer     text        NOT NULL,
    event_id     uuid        NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id),
    CONSTRAINT processed_events_consumer_not_empty CHECK (consumer <> '')
);

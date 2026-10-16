-- Transactional outbox. A state change and the event describing it are written
-- in one PostgreSQL transaction. A relay later publishes unpublished rows to
-- Redis Streams. PostgreSQL stays the record of what happened; the stream is
-- only a delivery channel.
CREATE TABLE outbox_events (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id     uuid        NOT NULL UNIQUE,
    stream       text        NOT NULL,
    event_type   text        NOT NULL,
    aggregate_id text        NOT NULL,
    payload      jsonb       NOT NULL,
    trace_id     text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    attempts     integer     NOT NULL DEFAULT 0,
    last_error   text        NOT NULL DEFAULT '',
    CONSTRAINT outbox_events_stream_format CHECK (stream ~ '^[a-z0-9._-]{1,100}$'),
    CONSTRAINT outbox_events_type_not_empty CHECK (event_type <> ''),
    CONSTRAINT outbox_events_attempts_non_negative CHECK (attempts >= 0)
);

CREATE INDEX outbox_events_unpublished_idx ON outbox_events (id) WHERE published_at IS NULL;

-- +goose Up
-- Transactional outbox: events are written in the same transaction as the shipment change
-- and relayed to NATS by a background worker.
CREATE TABLE outbox (
    id         uuid PRIMARY KEY,
    subject    text        NOT NULL,
    payload    bytea       NOT NULL,
    -- Trace context (W3C traceparent) captured at write time, so the trace continues in the relay.
    headers    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    sent_at    timestamptz
);

CREATE INDEX outbox_unsent_idx ON outbox (created_at) WHERE sent_at IS NULL;

-- +goose Down
DROP TABLE outbox;

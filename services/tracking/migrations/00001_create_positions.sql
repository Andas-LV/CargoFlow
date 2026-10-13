-- +goose Up
CREATE TABLE positions (
    id          uuid PRIMARY KEY, -- event ID for positions created from events (idempotency)
    shipment_id uuid             NOT NULL,
    station     text             NOT NULL,
    lat         double precision NOT NULL DEFAULT 0,
    lon         double precision NOT NULL DEFAULT 0,
    source      text             NOT NULL,
    recorded_at timestamptz      NOT NULL
);

CREATE INDEX positions_shipment_recorded_idx ON positions (shipment_id, recorded_at);

-- +goose Down
DROP TABLE positions;

-- +goose Up
CREATE TABLE shipments (
    id          uuid PRIMARY KEY,
    owner_id    uuid        NOT NULL,
    cargo_type  text        NOT NULL,
    origin      text        NOT NULL,
    destination text        NOT NULL,
    status      text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX shipments_owner_created_idx ON shipments (owner_id, created_at DESC);

-- +goose Down
DROP TABLE shipments;

-- +goose Up
CREATE TABLE users (
    id            uuid PRIMARY KEY,
    email         text        NOT NULL UNIQUE,
    password_hash text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;

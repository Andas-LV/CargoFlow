-- +goose Up
-- Shipments reference the station directory instead of free-text station names.
-- Existing shipments (test data with free text) are deleted.
DELETE FROM outbox;
DELETE FROM shipments;

ALTER TABLE shipments
    DROP COLUMN origin,
    DROP COLUMN destination,
    ADD COLUMN origin_station_id      text NOT NULL REFERENCES stations (id),
    ADD COLUMN destination_station_id text NOT NULL REFERENCES stations (id);

-- +goose Down
DELETE FROM outbox;
DELETE FROM shipments;

ALTER TABLE shipments
    DROP COLUMN origin_station_id,
    DROP COLUMN destination_station_id,
    ADD COLUMN origin      text NOT NULL,
    ADD COLUMN destination text NOT NULL;

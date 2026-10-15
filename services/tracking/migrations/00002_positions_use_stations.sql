-- +goose Up
-- Positions reference the station directory (owned by the shipment service, so no foreign key
-- across databases). Old positions belonged to deleted test shipments and are removed.
DELETE FROM positions;

ALTER TABLE positions ADD COLUMN station_id text NOT NULL;

-- +goose Down
ALTER TABLE positions DROP COLUMN station_id;

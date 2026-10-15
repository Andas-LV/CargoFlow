-- name: InsertPosition :execrows
-- ON CONFLICT makes redelivered events a no-op: 0 rows affected.
INSERT INTO positions (id, shipment_id, station_id, station, lat, lon, source, recorded_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO NOTHING;

-- name: ListPositionsByShipment :many
SELECT * FROM positions
WHERE shipment_id = $1
ORDER BY recorded_at;

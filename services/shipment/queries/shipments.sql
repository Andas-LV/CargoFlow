-- name: CreateShipment :one
INSERT INTO shipments (id, owner_id, cargo_type, origin_station_id, destination_station_id, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetShipment :one
SELECT * FROM shipments
WHERE id = $1;

-- name: GetShipmentForUpdate :one
SELECT * FROM shipments
WHERE id = $1
FOR UPDATE;

-- name: ListShipmentsByOwner :many
SELECT * FROM shipments
WHERE owner_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: UpdateShipmentStatus :one
UPDATE shipments
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING *;

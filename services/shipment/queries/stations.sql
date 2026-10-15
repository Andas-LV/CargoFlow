-- name: UpsertStations :exec
-- Bulk upsert from the embedded CSV; existing rows are updated, nothing is deleted
-- (shipments reference stations by foreign key).
INSERT INTO stations (id, esr, kind, name, name_ru, name_kk, name_en, lat, lon)
-- Several unnest() calls in one SELECT expand in lockstep: row i takes element i of each array.
SELECT
    unnest(@ids::text[]),
    unnest(@esrs::text[]),
    unnest(@kinds::text[]),
    unnest(@names::text[]),
    unnest(@names_ru::text[]),
    unnest(@names_kk::text[]),
    unnest(@names_en::text[]),
    unnest(@lats::float8[]),
    unnest(@lons::float8[])
ON CONFLICT (id) DO UPDATE SET
    esr = EXCLUDED.esr,
    kind = EXCLUDED.kind,
    name = EXCLUDED.name,
    name_ru = EXCLUDED.name_ru,
    name_kk = EXCLUDED.name_kk,
    name_en = EXCLUDED.name_en,
    lat = EXCLUDED.lat,
    lon = EXCLUDED.lon;

-- name: ListStations :many
SELECT * FROM stations
ORDER BY name_ru;

-- +goose Up
-- Railway station directory (Kazakhstan). Filled at service start from data/stations_kz.csv.
-- Data: © OpenStreetMap contributors (ODbL).
CREATE TABLE stations (
    id      text PRIMARY KEY,           -- e.g. osm-n922818608
    esr     text             NOT NULL DEFAULT '',
    kind    text             NOT NULL,  -- station | halt
    name    text             NOT NULL,
    name_ru text             NOT NULL,
    name_kk text             NOT NULL DEFAULT '',
    name_en text             NOT NULL DEFAULT '',
    lat     double precision NOT NULL,
    lon     double precision NOT NULL
);

CREATE INDEX stations_esr_idx ON stations (esr) WHERE esr <> '';

-- +goose Down
DROP TABLE stations;

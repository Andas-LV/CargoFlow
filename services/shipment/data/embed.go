// Package data embeds the station directory built by tools/stations.
package data

import _ "embed"

// StationsKZ is stations_kz.csv: Kazakhstan railway stations and halts (© OpenStreetMap contributors, ODbL).
//
//go:embed stations_kz.csv
var StationsKZ []byte

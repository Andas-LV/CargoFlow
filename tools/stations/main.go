// Command stations builds the Kazakhstan railway station directory (stations and halts).
//
// Names and coordinates come from OpenStreetMap (Overpass API). ESR codes come from the OSM
// esr:user tag or, when missing, from the osm.sbin.ru ESR registry (osm2esr.csv).
//
//	go run ./tools/stations                      # writes services/shipment/data/stations_kz.csv
//	go run ./tools/stations -out stations.csv
//
// The output is derived from OpenStreetMap and is licensed under ODbL:
// "© OpenStreetMap contributors".
package main

import (
	"cmp"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const userAgent = "CargoFlow-stations/1.0 (+https://github.com/Andas-LV/cargoflow)"

// Every railway station and halt inside Kazakhstan; ways/relations are reduced to their center.
const overpassQuery = `[out:json][timeout:180];
area["ISO3166-1"="KZ"][admin_level=2]->.kz;
nwr["railway"~"^(station|halt)$"](area.kz);
out center tags;`

// The main instance has the freshest data; mirrors can lag behind by a lot (we saw renamed
// stations come back with Soviet-era names), so they are only a last resort.
const overpassMain = "https://overpass-api.de/api/interpreter"

var overpassMirrors = []string{
	"https://overpass.private.coffee/api/interpreter",
	"https://overpass.kumi.systems/api/interpreter",
}

const mainAttempts = 6

const esrRegistryURL = "http://osm.sbin.ru/esr/osm2esr.csv"

var esrPattern = regexp.MustCompile(`^\d{6}$`)

// Station kinds of urban rail, not mainline railway: excluded from the directory.
var urbanRail = map[string]bool{"subway": true, "light_rail": true, "monorail": true, "funicular": true, "miniature": true, "tram": true}

type element struct {
	Type   string                      `json:"type"`
	ID     int64                       `json:"id"`
	Lat    float64                     `json:"lat"`
	Lon    float64                     `json:"lon"`
	Center *struct{ Lat, Lon float64 } `json:"center"`
	Tags   map[string]string           `json:"tags"`
}

type station struct {
	ID     string // stable id: osm-<n|w|r><osm id>
	ESR    string
	Kind   string // station | halt
	Name   string // default OSM name as signed locally
	NameRU string
	NameKK string
	NameEN string
	Lat    float64
	Lon    float64
}

func main() {
	out := flag.String("out", filepath.FromSlash("services/shipment/data/stations_kz.csv"), "output CSV path")
	flag.Parse()
	if err := run(*out); err != nil {
		log.Fatal(err)
	}
}

func run(out string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	elements, err := fetchOverpass(ctx)
	if err != nil {
		return fmt.Errorf("overpass: %w", err)
	}
	log.Printf("overpass: %d elements", len(elements))

	registry, err := fetchESRRegistry(ctx)
	if err != nil {
		log.Printf("esr registry unavailable, using only OSM esr:user tags: %v", err)
	}

	stations, skipped := build(elements, registry)
	if err := writeCSV(out, stations); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}

	var withESR, withKK, withEN int
	for _, s := range stations {
		if s.ESR != "" {
			withESR++
		}
		if s.NameKK != "" {
			withKK++
		}
		if s.NameEN != "" {
			withEN++
		}
	}
	log.Printf("wrote %d stations to %s (skipped %d: unnamed, urban rail or duplicates); esr: %d, kk: %d, en: %d",
		len(stations), out, skipped, withESR, withKK, withEN)
	return nil
}

func build(elements []element, registry map[string]string) ([]station, int) {
	stations := make([]station, 0, len(elements))
	skipped := 0
	for _, e := range elements {
		t := e.Tags
		name := strings.TrimSpace(t["name"])
		if name == "" || urbanRail[t["station"]] || t["subway"] == "yes" || t["light_rail"] == "yes" {
			skipped++
			continue
		}

		lat, lon := e.Lat, e.Lon
		if e.Center != nil {
			lat, lon = e.Center.Lat, e.Center.Lon
		}

		key := e.Type[:1] + strconv.FormatInt(e.ID, 10) // n123, w456, r789
		esr := strings.TrimSpace(t["esr:user"])
		if !esrPattern.MatchString(esr) {
			esr = registry[key]
		}

		stations = append(stations, station{
			ID:     "osm-" + key,
			ESR:    esr,
			Kind:   t["railway"],
			Name:   name,
			NameRU: cmp.Or(t["name:ru"], name),
			NameKK: t["name:kk"],
			NameEN: t["name:en"],
			Lat:    lat,
			Lon:    lon,
		})
	}
	before := len(stations)
	stations = dedupe(stations)
	skipped += before - len(stations)
	slices.SortFunc(stations, func(a, b station) int {
		return cmp.Or(strings.Compare(a.NameRU, b.NameRU), strings.Compare(a.ID, b.ID))
	})
	return stations, skipped
}

// dedupe merges objects that describe the same station: OSM sometimes has several points
// for one station (a node and the station building, or copies of a numbered siding).
// Duplicates share an ESR code, or have the same name within dupRadiusKm.
// The record with more information (ESR, translations) wins.
func dedupe(stations []station) []station {
	slices.SortStableFunc(stations, func(a, b station) int { return richness(b) - richness(a) })
	kept := make([]station, 0, len(stations))
	seenESR := map[string]bool{}
next:
	for _, s := range stations {
		if s.ESR != "" && seenESR[s.ESR] {
			continue
		}
		for _, k := range kept {
			if sameName(k, s) && distanceKm(k, s) < dupRadiusKm {
				continue next
			}
		}
		if s.ESR != "" {
			seenESR[s.ESR] = true
		}
		kept = append(kept, s)
	}
	return kept
}

const dupRadiusKm = 3

// kazakhToRussian folds Kazakh-specific letters so that "Жезқазған" matches "Жезказган".
var kazakhToRussian = strings.NewReplacer(
	"ә", "а", "ғ", "г", "қ", "к", "ң", "н", "ө", "о", "ұ", "у", "ү", "у", "һ", "х", "і", "и",
	"ё", "е", "-", " ",
)

// sameName reports whether any spelling of a matches any spelling of b.
func sameName(a, b station) bool {
	for _, x := range []string{a.Name, a.NameRU, a.NameKK} {
		for _, y := range []string{b.Name, b.NameRU, b.NameKK} {
			if x != "" && y != "" && normalize(x) == normalize(y) {
				return true
			}
		}
	}
	return false
}

func normalize(s string) string {
	return strings.Join(strings.Fields(kazakhToRussian.Replace(strings.ToLower(s))), " ")
}

func richness(s station) int {
	n := 0
	for _, v := range []string{s.ESR, s.NameKK, s.NameEN} {
		if v != "" {
			n++
		}
	}
	if s.ESR != "" {
		n += 2 // the code matters most
	}
	return n
}

// distanceKm is an equirectangular approximation, accurate enough for a few kilometres.
func distanceKm(a, b station) float64 {
	const earthRadiusKm = 6371
	lat := (a.Lat + b.Lat) / 2 * math.Pi / 180
	dx := (b.Lon - a.Lon) * math.Pi / 180 * math.Cos(lat)
	dy := (b.Lat - a.Lat) * math.Pi / 180
	return earthRadiusKm * math.Hypot(dx, dy)
}

func fetchOverpass(ctx context.Context) ([]element, error) {
	endpoints := make([]string, 0, mainAttempts+len(overpassMirrors))
	for range mainAttempts {
		endpoints = append(endpoints, overpassMain)
	}
	endpoints = append(endpoints, overpassMirrors...)

	var errs []error
	for _, endpoint := range endpoints {
		elements, err := queryOverpass(ctx, endpoint)
		if err == nil {
			log.Printf("overpass data from %s", endpoint)
			if endpoint != overpassMain {
				log.Printf("WARNING: data comes from a mirror and may be outdated; rerun later to use %s", overpassMain)
			}
			return elements, nil
		}
		log.Printf("overpass %s: %v", endpoint, err)
		errs = append(errs, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Second):
		}
	}
	return nil, errors.Join(errs...)
}

func queryOverpass(ctx context.Context, endpoint string) ([]element, error) {
	form := url.Values{"data": {overpassQuery}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent) // Overpass rejects requests without one (406)

	body, err := do(req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Elements []element `json:"elements"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(resp.Elements) == 0 {
		return nil, errors.New("empty result")
	}
	return resp.Elements, nil
}

// fetchESRRegistry returns ESR codes keyed by OSM object ("n123", "w456").
// osm2esr.csv columns: "esr";"status";"type";"osm_id";"lat";"lon";"name";... where type is
// 0 = node, 1 = way, 2 = relation.
func fetchESRRegistry(ctx context.Context) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, esrRegistryURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	body, err := do(req)
	if err != nil {
		return nil, err
	}

	r := csv.NewReader(strings.NewReader(string(body)))
	r.Comma = ';'
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}

	prefix := map[string]string{"0": "n", "1": "w", "2": "r"}
	registry := make(map[string]string, len(rows))
	for _, row := range rows[1:] { // skip header
		if len(row) < 4 || !esrPattern.MatchString(row[0]) || prefix[row[2]] == "" {
			continue
		}
		registry[prefix[row[2]]+row[3]] = row[0]
	}
	return registry, nil
}

func do(req *http.Request) ([]byte, error) {
	resp, err := (&http.Client{Timeout: 4 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return body, nil
}

func writeCSV(path string, stations []station) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"id", "esr", "kind", "name", "name_ru", "name_kk", "name_en", "lat", "lon"})
	for _, s := range stations {
		_ = w.Write([]string{
			s.ID, s.ESR, s.Kind, s.Name, s.NameRU, s.NameKK, s.NameEN,
			strconv.FormatFloat(s.Lat, 'f', 6, 64), strconv.FormatFloat(s.Lon, 'f', 6, 64),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

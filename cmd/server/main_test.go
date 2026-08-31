package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gasapp/internal/db"
	"gasapp/internal/station"
)

func TestStationsHandlerEmpty(t *testing.T) {
	w := doRequest(t, newTestDB(t), "/stations/")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	resp := decodeStations(t, w)
	if len(resp) != 0 {
		t.Errorf("got %d stations, want 0", len(resp))
	}
}

func TestStationsHandlerContentType(t *testing.T) {
	w := doRequest(t, newTestDB(t), "/stations/")

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestStationsHandlerCenter(t *testing.T) {
	database := newTestDB(t)
	insertStation(t, database, 1, 40.4, -3.7, 1.5) // at center
	insertStation(t, database, 2, 41.4, -3.7, 1.6) // ~111 km north
	insertStation(t, database, 3, 42.4, -3.7, 1.7) // ~222 km north

	w := doRequest(t, database, "/stations/?center=-3.7,40.4")

	rows := decodeStations(t, w)
	if len(rows) != 3 {
		t.Fatalf("got %d stations, want 3", len(rows))
	}
	if id := int64(rows[0][0].(float64)); id != 1 {
		t.Errorf("closest station ID = %d, want 1", id)
	}
	if id := int64(rows[2][0].(float64)); id != 3 {
		t.Errorf("furthest station ID = %d, want 3", id)
	}
}

func TestStationsHandlerInvalidCenter(t *testing.T) {
	w := doRequest(t, newTestDB(t), "/stations/?center=notvalid")

	// Falls back gracefully to returning all stations.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestStationsHandlerByIDs(t *testing.T) {
	database := newTestDB(t)
	insertStation(t, database, 1, 40.0, -3.0, 1.5)
	insertStation(t, database, 2, 41.0, -3.0, 1.6)
	insertStation(t, database, 3, 42.0, -3.0, 1.7)

	w := doRequest(t, database, "/stations/?ids=3,1,99")

	rows := decodeStations(t, w)
	if len(rows) != 2 {
		t.Fatalf("got %d stations, want 2 (unknown ID 99 must be skipped)", len(rows))
	}
	if id := int64(rows[0][0].(float64)); id != 3 {
		t.Errorf("rows[0] ID = %d, want 3 (request order preserved)", id)
	}
	if id := int64(rows[1][0].(float64)); id != 1 {
		t.Errorf("rows[1] ID = %d, want 1", id)
	}
}

func TestStationsHandlerByIDsOverridesCenter(t *testing.T) {
	database := newTestDB(t)
	insertStation(t, database, 1, 40.0, -3.0, 1.5)
	insertStation(t, database, 2, 41.0, -3.0, 1.6)

	w := doRequest(t, database, "/stations/?ids=2&center=-3.0,40.0")

	rows := decodeStations(t, w)
	if len(rows) != 1 {
		t.Fatalf("got %d stations, want 1 (ids must override center)", len(rows))
	}
	if id := int64(rows[0][0].(float64)); id != 2 {
		t.Errorf("ID = %d, want 2", id)
	}
}

func TestStationsHandlerByIDsEmpty(t *testing.T) {
	database := newTestDB(t)
	insertStation(t, database, 1, 40.0, -3.0, 1.5)

	w := doRequest(t, database, "/stations/?ids=")

	rows := decodeStations(t, w)
	if len(rows) != 0 {
		t.Errorf("got %d stations, want 0 for empty ids", len(rows))
	}
}

func TestStationsHandlerByIDsMalformed(t *testing.T) {
	database := newTestDB(t)
	insertStation(t, database, 1, 40.0, -3.0, 1.5)

	w := doRequest(t, database, "/stations/?ids=abc,xyz")

	rows := decodeStations(t, w)
	if len(rows) != 0 {
		t.Errorf("got %d stations, want 0 for malformed ids", len(rows))
	}
}

func TestStationsHandlerIncludesHistory(t *testing.T) {
	database := newTestDB(t)
	insertStation(t, database, 1, 40.4, -3.7, 1.5)

	w := doRequest(t, database, "/stations/?center=-3.7,40.4&fuel=petrol95")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body struct {
		Stations   [][]any `json:"stations"`
		HistoryEnd int64   `json:"history_end"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Stations) != 1 {
		t.Fatalf("got %d rows, want 1", len(body.Stations))
	}
	row := body.Stations[0]
	if len(row) != 12 {
		t.Fatalf("row has %d fields, want 12", len(row))
	}
	hist, ok := row[11].([]any)
	if !ok {
		t.Fatalf("history field type = %T, want []any", row[11])
	}
	if len(hist) != station.HistoryDays {
		t.Errorf("history length = %d, want %d", len(hist), station.HistoryDays)
	}
	// Today's bucket (index 29) should hold the price the upsert just recorded.
	last := hist[len(hist)-1]
	if last == nil {
		t.Errorf("today's history slot = nil, want 1.5")
	} else if v, ok := last.(float64); !ok || v != 1.5 {
		t.Errorf("today's history slot = %v, want 1.5", last)
	}
	if body.HistoryEnd == 0 {
		t.Error("history_end = 0, want unix seconds")
	}
}

func TestStationsHandlerRejectsUnknownFuel(t *testing.T) {
	w := doRequest(t, newTestDB(t), "/stations/?fuel=hydrogen")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestStationsHandlerDefaultsToPetrol95(t *testing.T) {
	database := newTestDB(t)
	insertStation(t, database, 1, 40.4, -3.7, 1.5)
	w := doRequest(t, database, "/stations/?center=-3.7,40.4")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Stations [][]any `json:"stations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Stations) != 1 || len(body.Stations[0]) != 12 {
		t.Fatalf("unexpected shape: %v", body.Stations)
	}
	hist, ok := body.Stations[0][11].([]any)
	if !ok || len(hist) != station.HistoryDays {
		t.Fatalf("missing/short history on default fuel")
	}
	if hist[len(hist)-1] == nil {
		t.Error("petrol95 today's slot should be populated")
	}
}

// helpers

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal("open db:", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func insertStation(t *testing.T, database *sql.DB, id int64, lat, lng, price float64) {
	t.Helper()
	if err := station.Upsert(database, station.Station{
		ID:           id,
		Name:         "Test",
		Updated:      time.Now().Unix(),
		PostalCode:   "00000",
		Address:      "Test",
		OpeningHours: "24H",
		Town:         "Test",
		City:         "Test",
		State:        "Test",
		Petrol95:     &price,
		Lat:          lat,
		Lng:          lng,
	}); err != nil {
		t.Fatal("insert:", err)
	}
}

func doRequest(t *testing.T, database *sql.DB, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	stationsHandler(database)(w, req)
	return w
}

func decodeStations(t *testing.T, w *httptest.ResponseRecorder) [][]any {
	t.Helper()
	var body struct {
		Stations [][]any `json:"stations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, w.Body.String())
	}
	return body.Stations
}

func TestStationsHandlerUnfilteredIsLimited(t *testing.T) {
	database := newTestDB(t)
	for i := int64(1); i <= nearbyLimit+25; i++ {
		insertStation(t, database, i, 40.0+float64(i)/1000, -3.0, 1.5)
	}

	w := doRequest(t, database, "/stations/")

	rows := decodeStations(t, w)
	if len(rows) != nearbyLimit {
		t.Fatalf("got %d stations, want %d", len(rows), nearbyLimit)
	}
	// Bounded like the map query, and still carrying history.
	hist, ok := rows[0][11].([]any)
	if !ok || len(hist) != station.HistoryDays {
		t.Fatalf("history missing or short: %T", rows[0][11])
	}
	if hist[len(hist)-1] == nil {
		t.Error("today's slot should hold the price the upsert recorded")
	}
}

func TestStationsHandlerByIDsIncludesHistory(t *testing.T) {
	database := newTestDB(t)
	insertStation(t, database, 7, 40.4, -3.7, 1.5)

	w := doRequest(t, database, "/stations/?ids=7")

	rows := decodeStations(t, w)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	hist, ok := rows[0][11].([]any)
	if !ok || len(hist) != station.HistoryDays {
		t.Fatalf("history missing or short: %T", rows[0][11])
	}
	if hist[len(hist)-1] == nil {
		t.Error("today's slot should hold the price the upsert recorded")
	}
}

func TestStationsHandlerCenterLimitsResults(t *testing.T) {
	database := newTestDB(t)
	for i := int64(1); i <= nearbyLimit+25; i++ {
		insertStation(t, database, i, 40.0+float64(i)/1000, -3.0, 1.5)
	}

	w := doRequest(t, database, "/stations/?center=-3.0,40.0")

	rows := decodeStations(t, w)
	if len(rows) != nearbyLimit {
		t.Errorf("got %d stations, want %d", len(rows), nearbyLimit)
	}
	if id := int64(rows[0][0].(float64)); id != 1 {
		t.Errorf("closest ID = %d, want 1", id)
	}
}

func TestParseCenter(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		lat, lng float64
		ok       bool
	}{
		{"lng first", "-3.70,40.41", 40.41, -3.70, true},
		{"integers", "2,41", 41, 2, true},
		{"missing part", "40.41", 0, 0, false},
		{"not a number", "notvalid,40.41", 0, 0, false},
		{"empty", "", 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lat, lng, ok := parseCenter(c.raw)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if lat != c.lat || lng != c.lng {
				t.Errorf("lat,lng = %v,%v, want %v,%v", lat, lng, c.lat, c.lng)
			}
		})
	}
}

func TestParseIDs(t *testing.T) {
	t.Run("skips malformed entries", func(t *testing.T) {
		got := parseIDs("1,abc,3")
		if len(got) != 2 || got[0] != 1 || got[1] != 3 {
			t.Errorf("ids = %v, want [1 3]", got)
		}
	})

	t.Run("caps the list", func(t *testing.T) {
		raw := strings.TrimSuffix(strings.Repeat("1,", maxIDs+10), ",")
		if got := parseIDs(raw); len(got) != maxIDs {
			t.Errorf("len = %d, want %d", len(got), maxIDs)
		}
	})
}

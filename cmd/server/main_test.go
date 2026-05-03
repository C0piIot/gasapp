package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"gasapp/internal/db"
	"gasapp/internal/station"
)

func TestStationsHandlerEmpty(t *testing.T) {
	sc := &stationCache{db: newTestDB(t)}
	w := doRequest(t, sc, "/stations/")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	resp := decodeStations(t, w)
	if len(resp) != 0 {
		t.Errorf("got %d stations, want 0", len(resp))
	}
}

func TestStationsHandlerContentType(t *testing.T) {
	sc := &stationCache{db: newTestDB(t)}
	w := doRequest(t, sc, "/stations/")

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestStationsHandlerCenter(t *testing.T) {
	database := newTestDB(t)
	insertStation(t, database, 1, 40.4, -3.7, 1.5)   // at center
	insertStation(t, database, 2, 41.4, -3.7, 1.6)   // ~111 km north
	insertStation(t, database, 3, 42.4, -3.7, 1.7)   // ~222 km north

	sc := &stationCache{db: database}
	w := doRequest(t, sc, "/stations/?center=-3.7,40.4")

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
	sc := &stationCache{db: newTestDB(t)}
	w := doRequest(t, sc, "/stations/?center=notvalid")

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

	sc := &stationCache{db: database}
	w := doRequest(t, sc, "/stations/?ids=3,1,99")

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

	sc := &stationCache{db: database}
	w := doRequest(t, sc, "/stations/?ids=2&center=-3.0,40.0")

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

	sc := &stationCache{db: database}
	w := doRequest(t, sc, "/stations/?ids=")

	rows := decodeStations(t, w)
	if len(rows) != 0 {
		t.Errorf("got %d stations, want 0 for empty ids", len(rows))
	}
}

func TestStationsHandlerByIDsMalformed(t *testing.T) {
	database := newTestDB(t)
	insertStation(t, database, 1, 40.0, -3.0, 1.5)

	sc := &stationCache{db: database}
	w := doRequest(t, sc, "/stations/?ids=abc,xyz")

	rows := decodeStations(t, w)
	if len(rows) != 0 {
		t.Errorf("got %d stations, want 0 for malformed ids", len(rows))
	}
}

func TestStationsHandlerByIDsDoesNotMutateCache(t *testing.T) {
	database := newTestDB(t)
	insertStation(t, database, 1, 40.0, -3.0, 1.5)
	insertStation(t, database, 2, 41.0, -3.0, 1.6)

	sc := &stationCache{db: database}
	// Prime the cache.
	if _, err := sc.get(); err != nil {
		t.Fatal(err)
	}

	doRequest(t, sc, "/stations/?ids=2")

	stations, err := sc.get()
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 2 {
		t.Errorf("cache size = %d, want 2 (filter must not mutate the shared slice)", len(stations))
	}
}

func TestStationCacheInvalidate(t *testing.T) {
	database := newTestDB(t)
	sc := &stationCache{db: database}

	stations, err := sc.get()
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 0 {
		t.Fatalf("want 0 stations, got %d", len(stations))
	}

	insertStation(t, database, 99, 40.0, -3.0, 1.5)
	sc.invalidate()

	stations, err = sc.get()
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 1 {
		t.Fatalf("want 1 station after invalidate, got %d", len(stations))
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

func doRequest(t *testing.T, sc *stationCache, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	stationsHandler(sc)(w, req)
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

package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

func TestThemeColorMatchesManifest(t *testing.T) {
	var manifest struct {
		ThemeColor string `json:"theme_color"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "static", "site.webmanifest"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	// Browsers let the meta tag override the manifest, so a drifting pair
	// shows one colour in the tab and another in the installed app.
	metaColor := regexp.MustCompile(`<meta name="theme-color" content="([^"]*)">`)
	for _, name := range []string{"home.html", "offline.html"} {
		t.Run(name, func(t *testing.T) {
			page, err := os.ReadFile(filepath.Join("..", "..", "templates", name))
			if err != nil {
				t.Fatal(err)
			}
			found := metaColor.FindAllStringSubmatch(string(page), -1)
			if len(found) != 1 {
				t.Fatalf("got %d theme-color metas, want 1", len(found))
			}
			if found[0][1] != manifest.ThemeColor {
				t.Errorf("theme-color = %q, manifest theme_color = %q", found[0][1], manifest.ThemeColor)
			}
		})
	}
}

func TestManifestShortcuts(t *testing.T) {
	var manifest struct {
		Scope     string `json:"scope"`
		StartURL  string `json:"start_url"`
		Shortcuts []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"shortcuts"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "static", "site.webmanifest"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if len(manifest.Shortcuts) == 0 {
		t.Fatal("no shortcuts declared")
	}

	base, err := url.Parse(manifest.Scope)
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	start, err := base.Parse(manifest.StartURL)
	if err != nil {
		t.Fatalf("parse start_url: %v", err)
	}

	// An empty path and "/" name the same resource, so compare canonical forms.
	canonical := func(u *url.URL) url.URL {
		c := *u
		if c.Path == "" {
			c.Path = "/"
		}
		return c
	}
	scope, startURL := canonical(base), canonical(start)

	for _, sc := range manifest.Shortcuts {
		t.Run(sc.Name, func(t *testing.T) {
			parsed, err := base.Parse(sc.URL)
			if err != nil {
				t.Fatalf("parse url: %v", err)
			}
			target := canonical(parsed)
			if target.Host != scope.Host || !strings.HasPrefix(target.Path, scope.Path) {
				t.Errorf("%s resolves to %s, outside scope %s", sc.URL, &target, &scope)
			}
			// A shortcut landing on start_url adds nothing to the launcher menu.
			if target.String() == startURL.String() {
				t.Errorf("%s is start_url", sc.URL)
			}
		})
	}
}

func TestAndroidPackageConsistent(t *testing.T) {
	readJSON := func(t *testing.T, path string, v any) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, v); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
	}

	var twa struct {
		PackageID      string `json:"packageId"`
		Host           string `json:"host"`
		WebManifestURL string `json:"webManifestUrl"`
	}
	readJSON(t, "android/twa-manifest.json", &twa)

	var links []struct {
		Target struct {
			PackageName string `json:"package_name"`
		} `json:"target"`
	}
	readJSON(t, "static/assetlinks.json", &links)

	var manifest struct {
		Scope   string `json:"scope"`
		Related []struct {
			Platform string `json:"platform"`
			ID       string `json:"id"`
		} `json:"related_applications"`
	}
	readJSON(t, "static/site.webmanifest", &manifest)

	// A package mismatch with assetlinks silently degrades the TWA to a Custom
	// Tab with a URL bar instead of failing the build.
	t.Run("assetlinks", func(t *testing.T) {
		for _, l := range links {
			if l.Target.PackageName == twa.PackageID {
				return
			}
		}
		t.Errorf("no assetlinks entry for %s", twa.PackageID)
	})

	t.Run("related_applications", func(t *testing.T) {
		for _, r := range manifest.Related {
			if r.Platform == "play" && r.ID == twa.PackageID {
				return
			}
		}
		t.Errorf("no play related_application for %s", twa.PackageID)
	})

	t.Run("host", func(t *testing.T) {
		scope, err := url.Parse(manifest.Scope)
		if err != nil {
			t.Fatal(err)
		}
		if scope.Host != twa.Host {
			t.Errorf("twa host = %q, manifest scope host = %q", twa.Host, scope.Host)
		}
		wm, err := url.Parse(twa.WebManifestURL)
		if err != nil {
			t.Fatal(err)
		}
		if wm.Host != twa.Host || wm.Path != "/static/site.webmanifest" {
			t.Errorf("webManifestUrl = %q", twa.WebManifestURL)
		}
	})
}

func TestAssetLinksHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil)
	w := httptest.NewRecorder()
	assetLinksHandler(filepath.Join("..", "..", "static"))(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var links []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &links); err != nil {
		t.Errorf("body is not JSON: %v", err)
	}
}

package station

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"
	"time"

	"gasapp/internal/db"
)

func TestHaversine(t *testing.T) {
	cases := []struct {
		name                   string
		lat1, lng1, lat2, lng2 float64
		wantKm, toleranceKm    float64
	}{
		{"same point", 40.4168, -3.7038, 40.4168, -3.7038, 0, 0.001},
		{"one degree latitude", 0, 0, 1, 0, 111.195, 0.5},
		{"Madrid to Barcelona", 40.4168, -3.7038, 41.3851, 2.1734, 504, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := haversine(c.lat1, c.lng1, c.lat2, c.lng2)
			if math.Abs(got-c.wantKm) > c.toleranceKm {
				t.Errorf("haversine = %.2f km, want %.2f ±%.2f km", got, c.wantKm, c.toleranceKm)
			}
		})
	}
}

func TestByDistance(t *testing.T) {
	stations := []Station{
		{ID: 1, Lat: 3, Lng: 0}, // ~333 km from origin
		{ID: 2, Lat: 1, Lng: 0}, // ~111 km — closest
		{ID: 3, Lat: 2, Lng: 0}, // ~222 km
		{ID: 4, Lat: 4, Lng: 0}, // ~444 km — furthest
	}

	t.Run("sorted by distance", func(t *testing.T) {
		result := ByDistance(stations, 0, 0, 10)
		wantOrder := []int64{2, 3, 1, 4}
		for i, s := range result {
			if s.ID != wantOrder[i] {
				t.Errorf("position %d: ID=%d, want %d", i, s.ID, wantOrder[i])
			}
		}
	})

	t.Run("limit is respected", func(t *testing.T) {
		result := ByDistance(stations, 0, 0, 2)
		if len(result) != 2 {
			t.Fatalf("len=%d, want 2", len(result))
		}
		if result[0].ID != 2 {
			t.Errorf("closest ID=%d, want 2", result[0].ID)
		}
	})

	t.Run("limit larger than input", func(t *testing.T) {
		result := ByDistance(stations, 0, 0, 100)
		if len(result) != len(stations) {
			t.Errorf("len=%d, want %d", len(result), len(stations))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		result := ByDistance(nil, 0, 0, 10)
		if len(result) != 0 {
			t.Errorf("len=%d, want 0", len(result))
		}
	})
}

func TestUpsertAndAll(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	p95 := 1.659
	s := Station{
		ID:           42,
		Name:         "Test Station",
		Updated:      time.Now().Unix(),
		PostalCode:   "28001",
		Address:      "Calle Test 1",
		OpeningHours: "L-D: 24H",
		Town:         "Madrid",
		City:         "Madrid",
		State:        "Madrid",
		Petrol95:     &p95,
		Lat:          40.4168,
		Lng:          -3.7038,
	}

	if err := Upsert(database, s); err != nil {
		t.Fatal("upsert:", err)
	}

	stations, err := All(database)
	if err != nil {
		t.Fatal("all:", err)
	}
	if len(stations) != 1 {
		t.Fatalf("got %d stations, want 1", len(stations))
	}
	got := stations[0]
	if got.ID != s.ID {
		t.Errorf("ID = %d, want %d", got.ID, s.ID)
	}
	if got.Lat != s.Lat || got.Lng != s.Lng {
		t.Errorf("location = (%v,%v), want (%v,%v)", got.Lat, got.Lng, s.Lat, s.Lng)
	}
	if got.Petrol95 == nil || *got.Petrol95 != p95 {
		t.Errorf("Petrol95 = %v, want %v", got.Petrol95, p95)
	}
	if got.Petrol98 != nil || got.Gasoil != nil || got.GLP != nil {
		t.Error("expected nil for unset price fields")
	}

	t.Run("upsert updates existing row", func(t *testing.T) {
		newPrice := 1.759
		s.Petrol95 = &newPrice
		s.Updated = time.Now().Unix() + 1
		if err := Upsert(database, s); err != nil {
			t.Fatal("upsert:", err)
		}
		stations, err := All(database)
		if err != nil {
			t.Fatal("all:", err)
		}
		if len(stations) != 1 {
			t.Fatalf("got %d stations after update, want 1", len(stations))
		}
		if *stations[0].Petrol95 != newPrice {
			t.Errorf("Petrol95 after update = %v, want %v", *stations[0].Petrol95, newPrice)
		}
	})
}

func TestPriceChanged(t *testing.T) {
	v1, v2 := 1.659, 1.759
	cases := []struct {
		name       string
		old, new   *float64
		wantRecord bool
		wantNil    bool
		wantValue  float64
	}{
		{"both nil", nil, nil, false, false, 0},
		{"both equal", &v1, &v1, false, false, 0},
		{"different values", &v1, &v2, true, false, 1.759},
		{"old nil, new set", nil, &v1, true, false, 1.659},
		{"old set, new nil", &v1, nil, true, true, 0},
		{"tiny diff still counts", &v1, ptr(1.660), true, false, 1.660},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotRecord, gotWrite := priceChanged(c.old, c.new)
			if gotRecord != c.wantRecord {
				t.Fatalf("record = %v, want %v", gotRecord, c.wantRecord)
			}
			if !c.wantRecord {
				return
			}
			if c.wantNil {
				if gotWrite != nil {
					t.Errorf("write = %v, want nil", *gotWrite)
				}
				return
			}
			if gotWrite == nil || *gotWrite != c.wantValue {
				t.Errorf("write = %v, want %v", gotWrite, c.wantValue)
			}
		})
	}
}

type historyRow struct {
	StationID  int64
	Fuel       string
	Price      *float64
	ObservedAt int64
}

func priceHistory(t *testing.T, database *sql.DB) []historyRow {
	t.Helper()
	rows, err := database.Query(
		`SELECT station_id, fuel, price, observed_at FROM price_history
		 ORDER BY observed_at, fuel`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []historyRow
	for rows.Next() {
		var r historyRow
		if err := rows.Scan(&r.StationID, &r.Fuel, &r.Price, &r.ObservedAt); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func baseStation(updated int64) Station {
	return Station{
		ID:           42,
		Name:         "Test Station",
		Updated:      updated,
		PostalCode:   "28001",
		Address:      "Calle Test 1",
		OpeningHours: "L-D: 24H",
		Town:         "Madrid",
		City:         "Madrid",
		State:        "Madrid",
		Lat:          40.4168,
		Lng:          -3.7038,
	}
}

func ptr(f float64) *float64 { return &f }

func TestUpsertRecordsInitialPrices(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	s := baseStation(1000)
	s.Gasoil = ptr(1.559)
	s.Petrol95 = ptr(1.659)
	s.Petrol98 = ptr(1.799)
	// GLP intentionally nil

	if err := Upsert(database, s); err != nil {
		t.Fatal(err)
	}

	rows := priceHistory(t, database)
	if len(rows) != 3 {
		t.Fatalf("got %d history rows, want 3", len(rows))
	}
	want := map[string]float64{"gasoil": 1.559, "petrol95": 1.659, "petrol98": 1.799}
	for _, r := range rows {
		if r.StationID != s.ID {
			t.Errorf("station_id = %d, want %d", r.StationID, s.ID)
		}
		if r.ObservedAt != s.Updated {
			t.Errorf("observed_at = %d, want %d", r.ObservedAt, s.Updated)
		}
		w, ok := want[r.Fuel]
		if !ok {
			t.Errorf("unexpected fuel %q", r.Fuel)
			continue
		}
		if r.Price == nil || *r.Price != w {
			t.Errorf("price for %s = %v, want %v", r.Fuel, r.Price, w)
		}
	}
}

func TestUpsertSkipsUnchangedPrices(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	s := baseStation(1000)
	s.Petrol95 = ptr(1.659)
	if err := Upsert(database, s); err != nil {
		t.Fatal(err)
	}
	before := len(priceHistory(t, database))
	if before != 1 {
		t.Fatalf("initial rows = %d, want 1", before)
	}

	s.Updated = 2000
	if err := Upsert(database, s); err != nil {
		t.Fatal(err)
	}
	after := len(priceHistory(t, database))
	if after != before {
		t.Errorf("rows after no-op upsert = %d, want %d", after, before)
	}
}

func TestUpsertRecordsChangedPrice(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	s := baseStation(1000)
	s.Gasoil = ptr(1.559)
	s.Petrol95 = ptr(1.659)
	if err := Upsert(database, s); err != nil {
		t.Fatal(err)
	}

	s.Updated = 2000
	s.Petrol95 = ptr(1.759) // gasoil unchanged
	if err := Upsert(database, s); err != nil {
		t.Fatal(err)
	}

	rows := priceHistory(t, database)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (2 initial + 1 change)", len(rows))
	}
	last := rows[len(rows)-1]
	if last.Fuel != "petrol95" {
		t.Errorf("last fuel = %q, want petrol95", last.Fuel)
	}
	if last.ObservedAt != 2000 {
		t.Errorf("last observed_at = %d, want 2000", last.ObservedAt)
	}
	if last.Price == nil || *last.Price != 1.759 {
		t.Errorf("last price = %v, want 1.759", last.Price)
	}
}

func TestUpsertRecordsPriceDisappearance(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	s := baseStation(1000)
	s.Petrol98 = ptr(1.799)
	if err := Upsert(database, s); err != nil {
		t.Fatal(err)
	}

	s.Updated = 2000
	s.Petrol98 = nil
	if err := Upsert(database, s); err != nil {
		t.Fatal(err)
	}

	rows := priceHistory(t, database)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	last := rows[len(rows)-1]
	if last.Fuel != "petrol98" {
		t.Errorf("fuel = %q, want petrol98", last.Fuel)
	}
	if last.Price != nil {
		t.Errorf("price = %v, want NULL", *last.Price)
	}
	if last.ObservedAt != 2000 {
		t.Errorf("observed_at = %d, want 2000", last.ObservedAt)
	}
}

func TestUpsertHistoryAtomicWithStation(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	s := baseStation(1000)
	s.Petrol95 = ptr(1.659)

	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := upsertTx(tx, s); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	stations, err := All(database)
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 0 {
		t.Errorf("stations after rollback = %d, want 0", len(stations))
	}
	if rows := priceHistory(t, database); len(rows) != 0 {
		t.Errorf("history after rollback = %d, want 0", len(rows))
	}
}

func insertAt(t *testing.T, database *sql.DB, id int64, lat, lng float64) {
	t.Helper()
	s := baseStation(time.Now().Unix())
	s.ID = id
	s.Lat = lat
	s.Lng = lng
	s.Petrol95 = ptr(1.659)
	if err := Upsert(database, s); err != nil {
		t.Fatal("upsert:", err)
	}
}

func ids(stations []Station) []int64 {
	out := make([]int64, len(stations))
	for i, s := range stations {
		out[i] = s.ID
	}
	return out
}

func distances(stations []Station, lat, lng float64) []float64 {
	out := make([]float64, len(stations))
	for i, s := range stations {
		out[i] = haversine(lat, lng, s.Lat, s.Lng)
	}
	return out
}

func equalIDs(got []Station, want []int64) bool {
	g := ids(got)
	if len(g) != len(want) {
		return false
	}
	for i := range g {
		if g[i] != want[i] {
			return false
		}
	}
	return true
}

func TestNearby(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	insertAt(t, database, 1, 1, 0) // ~111 km
	insertAt(t, database, 2, 2, 0) // ~222 km
	insertAt(t, database, 3, 3, 0) // ~333 km

	t.Run("sorted by distance", func(t *testing.T) {
		got, err := Nearby(database, 0, 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if !equalIDs(got, []int64{1, 2, 3}) {
			t.Errorf("ids = %v, want [1 2 3]", ids(got))
		}
	})

	t.Run("limit is respected", func(t *testing.T) {
		got, err := Nearby(database, 0, 0, 2)
		if err != nil {
			t.Fatal(err)
		}
		if !equalIDs(got, []int64{1, 2}) {
			t.Errorf("ids = %v, want [1 2]", ids(got))
		}
	})

	t.Run("zero limit", func(t *testing.T) {
		got, err := Nearby(database, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("empty table", func(t *testing.T) {
		empty, err := db.Open(filepath.Join(t.TempDir(), "empty.sqlite3"))
		if err != nil {
			t.Fatal(err)
		}
		defer empty.Close()
		got, err := Nearby(empty, 40, -3, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("stale stations are excluded", func(t *testing.T) {
		s := baseStation(time.Now().Add(-8 * 24 * time.Hour).Unix())
		s.ID = 99
		s.Lat, s.Lng = 0.01, 0.01 // nearest by far, but dropped from the feed
		if err := Upsert(database, s); err != nil {
			t.Fatal(err)
		}
		got, err := Nearby(database, 0, 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range got {
			if s.ID == 99 {
				t.Fatal("stale station 99 must not be returned")
			}
		}
	})
}

// The search box only guarantees a complete answer inside its inscribed
// circle. With enough candidates sitting in a corner of the first box, a
// nearer station just past an edge would be missed unless the box grows.
func TestNearbyGrowsBoxPastCornerCandidates(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// nearbySearchBox is 0.15 degrees, so its inscribed circle is ~16.7 km.
	insertAt(t, database, 1, 0.140, 0.140) // in the first box, corner: ~22.0 km
	insertAt(t, database, 2, 0.145, 0.145) // in the first box, corner: ~22.8 km
	insertAt(t, database, 3, 0.160, 0.000) // outside the first box: ~17.8 km

	got, err := Nearby(database, 0, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !equalIDs(got, []int64{3, 1}) {
		t.Errorf("ids = %v, want [3 1]: station 3 is nearer than both corner stations", ids(got))
	}
}

func TestNearbyMatchesBruteForce(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// A deterministic spread across the peninsula, dense enough that the box
	// resolves without expanding for some centres and has to grow for others.
	id := int64(1)
	for lat := 36.0; lat <= 43.0; lat += 0.5 {
		for lng := -8.0; lng <= 3.0; lng += 0.5 {
			insertAt(t, database, id, lat, lng)
			id++
		}
	}

	all, err := All(database)
	if err != nil {
		t.Fatal(err)
	}

	centres := []struct {
		name     string
		lat, lng float64
	}{
		{"middle of the grid", 40.0, -3.0},
		{"corner of the grid", 36.0, -8.0},
		{"off the grid", 43.5, 3.5},
	}
	for _, c := range centres {
		t.Run(c.name, func(t *testing.T) {
			for _, limit := range []int{1, 5, 50} {
				got, err := Nearby(database, c.lat, c.lng, limit)
				if err != nil {
					t.Fatal(err)
				}
				want := ByDistance(all, c.lat, c.lng, limit)
				// Compare distances, not ids: a regular grid produces exact
				// ties and the sort is not stable, so tied stations may come
				// back in either order without the answer being wrong.
				gotKm := distances(got, c.lat, c.lng)
				wantKm := distances(want, c.lat, c.lng)
				if len(gotKm) != len(wantKm) {
					t.Fatalf("limit %d: len = %d, want %d", limit, len(gotKm), len(wantKm))
				}
				for i := range gotKm {
					if math.Abs(gotKm[i]-wantKm[i]) > 0.001 {
						t.Fatalf("limit %d: distance[%d] = %.3f km, want %.3f km",
							limit, i, gotKm[i], wantKm[i])
					}
				}
			}
		})
	}
}

func TestByIDs(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	insertAt(t, database, 1, 40.0, -3.0)
	insertAt(t, database, 2, 41.0, -3.0)
	insertAt(t, database, 3, 42.0, -3.0)

	t.Run("request order is preserved", func(t *testing.T) {
		got, err := ByIDs(database, []int64{3, 1, 2})
		if err != nil {
			t.Fatal(err)
		}
		if !equalIDs(got, []int64{3, 1, 2}) {
			t.Errorf("ids = %v, want [3 1 2]", ids(got))
		}
	})

	t.Run("unknown ids are skipped", func(t *testing.T) {
		got, err := ByIDs(database, []int64{2, 999})
		if err != nil {
			t.Fatal(err)
		}
		if !equalIDs(got, []int64{2}) {
			t.Errorf("ids = %v, want [2]", ids(got))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		got, err := ByIDs(database, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("stale stations are excluded", func(t *testing.T) {
		s := baseStation(time.Now().Add(-8 * 24 * time.Hour).Unix())
		s.ID = 77
		if err := Upsert(database, s); err != nil {
			t.Fatal(err)
		}
		got, err := ByIDs(database, []int64{77})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0 for a station outside the freshness window", len(got))
		}
	})
}

func TestPlaceholders(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, ""},
		{1, "?"},
		{3, "?,?,?"},
	}
	for _, c := range cases {
		if got := placeholders(c.n); got != c.want {
			t.Errorf("placeholders(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestListing(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	for i := int64(1); i <= 5; i++ {
		insertAt(t, database, i, 40.0+float64(i), -3.0)
	}

	t.Run("limit is respected", func(t *testing.T) {
		got, err := Listing(database, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Errorf("len = %d, want 3", len(got))
		}
	})

	t.Run("limit larger than the table", func(t *testing.T) {
		got, err := Listing(database, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 5 {
			t.Errorf("len = %d, want 5", len(got))
		}
	})

	t.Run("zero limit", func(t *testing.T) {
		got, err := Listing(database, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("stale stations are excluded", func(t *testing.T) {
		s := baseStation(time.Now().Add(-8 * 24 * time.Hour).Unix())
		s.ID = 88
		if err := Upsert(database, s); err != nil {
			t.Fatal(err)
		}
		got, err := Listing(database, 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range got {
			if s.ID == 88 {
				t.Fatal("stale station 88 must not be listed")
			}
		}
	})
}

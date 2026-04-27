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

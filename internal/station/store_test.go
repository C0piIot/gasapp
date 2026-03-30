package station

import (
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

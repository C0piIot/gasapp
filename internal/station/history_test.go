package station

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"gasapp/internal/db"
)

// Anchor "now" to a fixed instant so day buckets are stable across runs.
// 2026-04-15 12:34:56 UTC — comfortably mid-day, mid-month.
var fixedNow = time.Date(2026, 4, 15, 12, 34, 56, 0, time.UTC)

func dayStart(now time.Time, daysBack int) int64 {
	return WindowEnd(now).Add(time.Duration(-daysBack) * 24 * time.Hour).Unix()
}

func insertHistory(t *testing.T, database *sql.DB, stationID int64, fuel string, observedAt int64, price *float64) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO price_history (station_id, fuel, price, observed_at) VALUES (?,?,?,?)`,
		stationID, fuel, price, observedAt,
	); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryByFuel(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	stations := []Station{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}}

	// Station 1: no history at all -> all-nil 30-element row.
	// Station 2: one observation strictly before the window -> carry forward.
	insertHistory(t, database, 2, "petrol95", dayStart(fixedNow, 31)+3600, ptr(1.500))

	// Station 3: two observations on different in-window days.
	// Day index 5 = 1.600, day index 10 = 1.700.
	insertHistory(t, database, 3, "petrol95", dayStart(fixedNow, 30-5)+1000, ptr(1.600))
	insertHistory(t, database, 3, "petrol95", dayStart(fixedNow, 30-10)+1000, ptr(1.700))

	// Station 4: two observations on the same in-window day at different
	// prices -> the bucket should hold the max.
	insertHistory(t, database, 4, "petrol95", dayStart(fixedNow, 30-7)+1000, ptr(1.650))
	insertHistory(t, database, 4, "petrol95", dayStart(fixedNow, 30-7)+5000, ptr(1.700))

	// Station 5: seed value, then a NULL observation mid-window (fuel
	// disappears) -> nil from that day forward.
	insertHistory(t, database, 5, "petrol95", dayStart(fixedNow, 31)+3600, ptr(1.800))
	insertHistory(t, database, 5, "petrol95", dayStart(fixedNow, 30-15)+1000, nil)

	// Noise: a different fuel and a different station that's not in the
	// requested set must not leak into petrol95 results.
	insertHistory(t, database, 3, "gasoil", dayStart(fixedNow, 30-3)+1000, ptr(99.0))
	insertHistory(t, database, 999, "petrol95", dayStart(fixedNow, 30-3)+1000, ptr(99.0))

	got, err := HistoryByFuel(database, "petrol95", stations, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(stations) {
		t.Fatalf("len=%d, want %d", len(got), len(stations))
	}

	t.Run("station 1: empty history", func(t *testing.T) {
		for d, p := range got[0] {
			if p != nil {
				t.Errorf("day %d: got %v, want nil", d, *p)
			}
		}
	})

	t.Run("station 2: seed carries forward to all 30 days", func(t *testing.T) {
		for d, p := range got[1] {
			if p == nil || *p != 1.500 {
				t.Errorf("day %d: got %v, want 1.500", d, p)
			}
		}
	})

	t.Run("station 3: step pattern at days 5 and 10", func(t *testing.T) {
		for d, p := range got[2] {
			var want float64
			switch {
			case d < 5:
				if p != nil {
					t.Errorf("day %d: got %v, want nil (no seed)", d, *p)
				}
				continue
			case d < 10:
				want = 1.600
			default:
				want = 1.700
			}
			if p == nil || *p != want {
				t.Errorf("day %d: got %v, want %v", d, p, want)
			}
		}
	})

	t.Run("station 4: same-day max", func(t *testing.T) {
		for d, p := range got[3] {
			if d < 7 {
				if p != nil {
					t.Errorf("day %d: got %v, want nil", d, *p)
				}
				continue
			}
			if p == nil || *p != 1.700 {
				t.Errorf("day %d: got %v, want 1.700", d, p)
			}
		}
	})

	t.Run("station 5: NULL observation resets carry-forward", func(t *testing.T) {
		for d, p := range got[4] {
			if d < 15 {
				if p == nil || *p != 1.800 {
					t.Errorf("day %d: got %v, want 1.800", d, p)
				}
				continue
			}
			if p != nil {
				t.Errorf("day %d: got %v, want nil after NULL", d, *p)
			}
		}
	})
}

func TestHistoryByFuelRejectsUnknownFuel(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := HistoryByFuel(database, "hydrogen", nil, fixedNow); err == nil {
		t.Error("want error for unknown fuel, got nil")
	}
}

func TestHistoryByFuelOrderMatchesInput(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	insertHistory(t, database, 10, "petrol95", dayStart(fixedNow, 30-1)+100, ptr(1.111))
	insertHistory(t, database, 20, "petrol95", dayStart(fixedNow, 30-1)+100, ptr(2.222))

	stations := []Station{{ID: 20}, {ID: 10}}
	got, err := HistoryByFuel(database, "petrol95", stations, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if got[0][29] == nil || *got[0][29] != 2.222 {
		t.Errorf("result[0] (id=20) day 29 = %v, want 2.222", got[0][29])
	}
	if got[1][29] == nil || *got[1][29] != 1.111 {
		t.Errorf("result[1] (id=10) day 29 = %v, want 1.111", got[1][29])
	}
}

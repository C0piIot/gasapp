package station

import (
	"database/sql"
	"errors"
	"time"
)

// AllowedFuels lists the fuel identifiers accepted by HistoryByFuel and the
// /stations/?fuel= query parameter.
var AllowedFuels = []string{"petrol95", "petrol98", "gasoil", "glp"}

// IsAllowedFuel reports whether fuel is one of the supported fuel identifiers.
func IsAllowedFuel(fuel string) bool {
	for _, f := range AllowedFuels {
		if f == fuel {
			return true
		}
	}
	return false
}

// WindowEnd returns the exclusive upper bound (start-of-next-UTC-day) of the
// HistoryDays-long window ending at now. It is exposed so the HTTP layer can
// echo it to clients without re-deriving the rounding.
func WindowEnd(now time.Time) time.Time {
	utc := now.UTC()
	day := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	return day.Add(24 * time.Hour)
}

// HistoryByFuel returns the per-station daily-max price history for one fuel,
// over the HistoryDays days ending at now. The result is parallel to stations:
// result[i] is the history for stations[i]. Days with no observation carry
// forward the most recent prior known price; a NULL-price observation (fuel
// disappearance) resets the carried value to nil from that day forward until
// another non-nil observation lands.
func HistoryByFuel(db *sql.DB, fuel string, stations []Station, now time.Time) ([]DailyHistory, error) {
	if !IsAllowedFuel(fuel) {
		return nil, errors.New("station: unknown fuel " + fuel)
	}

	out := make([]DailyHistory, len(stations))
	if len(stations) == 0 {
		return out, nil
	}

	end := WindowEnd(now)
	start := end.Add(-HistoryDays * 24 * time.Hour)
	startTs := start.Unix()
	endTs := end.Unix()

	idx := make(map[int64]int, len(stations))
	for i, s := range stations {
		idx[s.ID] = i
	}

	// Seed: most recent observation strictly before the window per station.
	// The composite index (station_id, fuel, observed_at DESC) makes the
	// inner MAX cheap. We only care about stations we're returning.
	seedRows, err := db.Query(`
		SELECT ph.station_id, ph.price
		FROM price_history ph
		WHERE ph.fuel = ?
		  AND ph.observed_at < ?
		  AND ph.observed_at = (
		      SELECT MAX(observed_at) FROM price_history
		      WHERE station_id = ph.station_id
		        AND fuel = ph.fuel
		        AND observed_at < ?
		  )`, fuel, startTs, startTs)
	if err != nil {
		return nil, err
	}
	seeds := make(map[int64]*float64, len(stations))
	for seedRows.Next() {
		var sid int64
		var price *float64
		if err := seedRows.Scan(&sid, &price); err != nil {
			seedRows.Close()
			return nil, err
		}
		seeds[sid] = price
	}
	if err := seedRows.Err(); err != nil {
		seedRows.Close()
		return nil, err
	}
	seedRows.Close()

	// In-window changes, ordered for a single linear pass per station.
	winRows, err := db.Query(`
		SELECT station_id, observed_at, price
		FROM price_history
		WHERE fuel = ? AND observed_at >= ? AND observed_at < ?
		ORDER BY station_id, observed_at`, fuel, startTs, endTs)
	if err != nil {
		return nil, err
	}
	defer winRows.Close()

	type bucket struct {
		hasObs bool     // true if any observation landed in this day
		obsMax *float64 // max observed value within the day (may be nil if a NULL came in)
		obsNil bool     // true if a NULL observation was seen (signals "fuel disappeared")
	}
	buckets := make(map[int64]*[HistoryDays]bucket)
	for winRows.Next() {
		var sid, ts int64
		var price *float64
		if err := winRows.Scan(&sid, &ts, &price); err != nil {
			return nil, err
		}
		if _, ok := idx[sid]; !ok {
			continue
		}
		day := int((ts - startTs) / 86400)
		if day < 0 || day >= HistoryDays {
			continue
		}
		bs, ok := buckets[sid]
		if !ok {
			bs = &[HistoryDays]bucket{}
			buckets[sid] = bs
		}
		b := &bs[day]
		b.hasObs = true
		if price == nil {
			b.obsNil = true
			continue
		}
		if b.obsMax == nil || *price > *b.obsMax {
			v := *price
			b.obsMax = &v
		}
	}
	if err := winRows.Err(); err != nil {
		return nil, err
	}

	for sid, i := range idx {
		carry := seeds[sid]
		bs := buckets[sid]
		var row DailyHistory
		for d := 0; d < HistoryDays; d++ {
			if bs != nil {
				b := bs[d]
				if b.hasObs {
					// A NULL observation alone wipes carry-forward; if a
					// non-nil price also landed that day, take that as the
					// day's max regardless of the NULL.
					if b.obsMax != nil {
						carry = b.obsMax
					} else if b.obsNil {
						carry = nil
					}
				}
			}
			if carry != nil {
				v := *carry
				row[d] = &v
			}
		}
		out[i] = row
	}

	return out, nil
}

package station

import (
	"database/sql"
	"math"
	"sort"
	"strings"
	"time"
)

// Upsert inserts or updates a station by its ID, recording any price changes
// to price_history in the same transaction.
func Upsert(db *sql.DB, s Station) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := upsertTx(tx, s); err != nil {
		return err
	}
	return tx.Commit()
}

// upsertTx inserts or updates a station within an existing transaction,
// appending a price_history row for each fuel whose price differs from the
// stored value (including value->NULL transitions).
func upsertTx(tx *sql.Tx, s Station) error {
	if err := recordPriceHistory(tx, s); err != nil {
		return err
	}
	_, err := tx.Exec(upsertSQL,
		s.ID, s.Name, s.Updated, s.PostalCode, s.Address, s.OpeningHours,
		s.Town, s.City, s.State,
		s.Gasoil, s.Petrol95, s.Petrol98, s.GLP, s.Lat, s.Lng,
	)
	return err
}

const upsertSQL = `
	INSERT INTO stations
		(id, name, updated, postal_code, address, opening_hours, town, city, state,
		 gasoil, petrol95, petrol98, glp, lat, lng)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET
		name=excluded.name, updated=excluded.updated,
		postal_code=excluded.postal_code, address=excluded.address,
		opening_hours=excluded.opening_hours, town=excluded.town,
		city=excluded.city, state=excluded.state,
		gasoil=excluded.gasoil, petrol95=excluded.petrol95,
		petrol98=excluded.petrol98, glp=excluded.glp,
		lat=excluded.lat, lng=excluded.lng`

// OR IGNORE so two updates landing in the same second on the same station+fuel
// (Updated is set from time.Now().Unix() in buildStation) don't blow up.
const historyInsertSQL = `INSERT OR IGNORE INTO price_history
	(station_id, fuel, price, observed_at) VALUES (?, ?, ?, ?)`

// priceChanged reports whether new should be recorded as a history row given
// the previously stored old value, and what price to write (which may be nil
// for a value->NULL transition).
func priceChanged(old, new *float64) (record bool, write *float64) {
	switch {
	case old == nil && new == nil:
		return false, nil
	case old == nil:
		return true, new
	case new == nil:
		return true, nil
	case *old == *new:
		return false, nil
	default:
		return true, new
	}
}

// loadCurrentPrices returns the four price fields stored for a station, or all
// nils if the station does not exist yet.
func loadCurrentPrices(tx *sql.Tx, id int64) (gasoil, petrol95, petrol98, glp *float64, err error) {
	err = tx.QueryRow(
		`SELECT gasoil, petrol95, petrol98, glp FROM stations WHERE id = ?`, id,
	).Scan(&gasoil, &petrol95, &petrol98, &glp)
	if err == sql.ErrNoRows {
		return nil, nil, nil, nil, nil
	}
	return
}

func recordPriceHistory(tx *sql.Tx, s Station) error {
	oldGasoil, oldP95, oldP98, oldGLP, err := loadCurrentPrices(tx, s.ID)
	if err != nil {
		return err
	}
	pairs := []struct {
		fuel string
		old  *float64
		new  *float64
	}{
		{"gasoil", oldGasoil, s.Gasoil},
		{"petrol95", oldP95, s.Petrol95},
		{"petrol98", oldP98, s.Petrol98},
		{"glp", oldGLP, s.GLP},
	}
	for _, p := range pairs {
		record, write := priceChanged(p.old, p.new)
		if !record {
			continue
		}
		if _, err := tx.Exec(historyInsertSQL, s.ID, p.fuel, write, s.Updated); err != nil {
			return err
		}
	}
	return nil
}

const stationColumns = `id, name, updated, postal_code, address, opening_hours,
	town, city, state, gasoil, petrol95, petrol98, glp, lat, lng`

// recentCutoff is the age limit for a station to count as live. The feed
// re-stamps Updated on everything it still lists, so an older row means the
// station dropped out of the feed rather than that its prices are stale.
func recentCutoff() int64 {
	return time.Now().Add(-7 * 24 * time.Hour).Unix()
}

func scanStations(rows *sql.Rows) ([]Station, error) {
	defer rows.Close()
	var stations []Station
	for rows.Next() {
		var s Station
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Updated, &s.PostalCode, &s.Address, &s.OpeningHours,
			&s.Town, &s.City, &s.State,
			&s.Gasoil, &s.Petrol95, &s.Petrol98, &s.GLP, &s.Lat, &s.Lng,
		); err != nil {
			return nil, err
		}
		stations = append(stations, s)
	}
	return stations, rows.Err()
}

// placeholders renders n comma-separated bind markers for an IN clause.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// All returns all stations updated within the last 7 days.
func All(db *sql.DB) ([]Station, error) {
	rows, err := db.Query(`SELECT `+stationColumns+`
		FROM stations
		WHERE updated > ?`, recentCutoff())
	if err != nil {
		return nil, err
	}
	return scanStations(rows)
}

// Listing returns up to limit live stations, in no particular order. It backs
// the request that names neither a centre nor a set of ids: there is nothing to
// sort by, but the response still has to be bounded so that attaching price
// history stays proportional to what is returned.
func Listing(db *sql.DB, limit int) ([]Station, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := db.Query(`SELECT `+stationColumns+`
		FROM stations
		WHERE updated > ?
		LIMIT ?`, recentCutoff(), limit)
	if err != nil {
		return nil, err
	}
	return scanStations(rows)
}

// ByIDs returns the stations for ids, in the order given, skipping ids with no
// live row.
func ByIDs(db *sql.DB, ids []int64) ([]Station, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, recentCutoff())
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := db.Query(`SELECT `+stationColumns+`
		FROM stations
		WHERE updated > ? AND id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return nil, err
	}
	found, err := scanStations(rows)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]Station, len(found))
	for _, s := range found {
		byID[s.ID] = s
	}
	out := make([]Station, 0, len(ids))
	for _, id := range ids {
		if s, ok := byID[id]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

const (
	// kmPerDegree is one degree of latitude in kilometres. It only sizes the
	// search box; the haversine refinement decides the actual result.
	kmPerDegree = 111.32
	// nearbySearchBox is the initial half-height of the search box in degrees
	// (~17 km), sized so a query over a city resolves in one round trip.
	nearbySearchBox = 0.15
	// nearbyMaxBox spans well past the peninsula and islands, so the search
	// always terminates even when the table holds fewer stations than asked for.
	nearbyMaxBox = 8.0
)

// Nearby returns the limit stations closest to (lat, lng), sorted by distance.
// It grows a bounding box until the result is provably complete, so the
// stations_lat_lng index prunes the table instead of every row being
// distance-checked on every request.
func Nearby(db *sql.DB, lat, lng float64, limit int) ([]Station, error) {
	if limit <= 0 {
		return nil, nil
	}
	cutoff := recentCutoff()
	for box := nearbySearchBox; ; box *= 2 {
		// A degree of longitude shortens with latitude, so widen the longitude
		// side to keep the box roughly square in kilometres.
		lngBox := box / math.Cos(lat*math.Pi/180)
		rows, err := db.Query(`SELECT `+stationColumns+`
			FROM stations
			WHERE updated > ?
			  AND lat BETWEEN ? AND ?
			  AND lng BETWEEN ? AND ?`,
			cutoff, lat-box, lat+box, lng-lngBox, lng+lngBox)
		if err != nil {
			return nil, err
		}
		candidates, err := scanStations(rows)
		if err != nil {
			return nil, err
		}
		nearest := ByDistance(candidates, lat, lng, limit)

		// The box only guarantees completeness inside its inscribed circle: a
		// station just past an edge could still beat the farthest result. Keep
		// growing until the whole result fits inside that circle.
		if box >= nearbyMaxBox || complete(nearest, lat, lng, limit, box) {
			return nearest, nil
		}
	}
}

func complete(nearest []Station, lat, lng float64, limit int, box float64) bool {
	if len(nearest) < limit {
		return false
	}
	farthest := nearest[len(nearest)-1]
	return haversine(lat, lng, farthest.Lat, farthest.Lng) <= box*kmPerDegree
}

// ByDistance returns the nearest limit stations to (lat, lng), sorted by distance.
func ByDistance(stations []Station, lat, lng float64, limit int) []Station {
	idx := byDistanceIndices(stations, lat, lng, limit)
	result := make([]Station, len(idx))
	for i, j := range idx {
		result[i] = stations[j]
	}
	return result
}

func byDistanceIndices(stations []Station, lat, lng float64, limit int) []int {
	type ranked struct {
		idx  int
		dist float64
	}
	ranked_ := make([]ranked, len(stations))
	for i, s := range stations {
		ranked_[i] = ranked{i, haversine(lat, lng, s.Lat, s.Lng)}
	}
	sort.Slice(ranked_, func(i, j int) bool {
		return ranked_[i].dist < ranked_[j].dist
	})
	if len(ranked_) > limit {
		ranked_ = ranked_[:limit]
	}
	out := make([]int, len(ranked_))
	for i, r := range ranked_ {
		out[i] = r.idx
	}
	return out
}

func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	dlat := (lat2 - lat1) * math.Pi / 180
	dlng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dlng/2)*math.Sin(dlng/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

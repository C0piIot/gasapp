package station

import (
	"database/sql"
	"math"
	"sort"
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

// All returns all stations updated within the last 7 days.
func All(db *sql.DB) ([]Station, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).Unix()
	rows, err := db.Query(`
		SELECT id, name, updated, postal_code, address, opening_hours,
		       town, city, state, gasoil, petrol95, petrol98, glp, lat, lng
		FROM stations
		WHERE updated > ?`, cutoff)
	if err != nil {
		return nil, err
	}
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

// ByDistance returns the nearest limit stations to (lat, lng), sorted by distance.
func ByDistance(stations []Station, lat, lng float64, limit int) []Station {
	type ranked struct {
		s    Station
		dist float64
	}
	ranked_ := make([]ranked, len(stations))
	for i, s := range stations {
		ranked_[i] = ranked{s, haversine(lat, lng, s.Lat, s.Lng)}
	}
	sort.Slice(ranked_, func(i, j int) bool {
		return ranked_[i].dist < ranked_[j].dist
	})
	if len(ranked_) > limit {
		ranked_ = ranked_[:limit]
	}
	result := make([]Station, len(ranked_))
	for i, r := range ranked_ {
		result[i] = r.s
	}
	return result
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

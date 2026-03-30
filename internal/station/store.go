package station

import (
	"database/sql"
	"math"
	"sort"
	"time"
)

// Upsert inserts or updates a station by its ID.
func Upsert(db *sql.DB, s Station) error {
	_, err := db.Exec(upsertSQL,
		s.ID, s.Name, s.Updated, s.PostalCode, s.Address, s.OpeningHours,
		s.Town, s.City, s.State,
		s.Gasoil, s.Petrol95, s.Petrol98, s.GLP, s.Lat, s.Lng,
	)
	return err
}

// upsertTx inserts or updates a station within an existing transaction.
func upsertTx(tx *sql.Tx, s Station) error {
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

package db

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	// modernc.org/sqlite only reads pragmas through _pragma=; the _journal_mode
	// and _foreign_keys forms are mattn/go-sqlite3 syntax and are ignored
	// silently. busy_timeout matters because reads now hit the DB on every
	// request, concurrently with the hourly price update transaction.
	db, err := sql.Open("sqlite", path+
		"?_pragma=journal_mode(WAL)"+
		"&_pragma=foreign_keys(on)"+
		"&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS stations (
		id            INTEGER PRIMARY KEY,
		name          TEXT    NOT NULL,
		updated       INTEGER NOT NULL,
		postal_code   TEXT    NOT NULL,
		address       TEXT    NOT NULL,
		opening_hours TEXT    NOT NULL,
		town          TEXT    NOT NULL,
		city          TEXT    NOT NULL,
		state         TEXT    NOT NULL,
		gasoil        REAL,
		petrol95      REAL,
		petrol98      REAL,
		glp           REAL,
		lat           REAL    NOT NULL,
		lng           REAL    NOT NULL
	)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS price_history (
		station_id  INTEGER NOT NULL,
		fuel        TEXT    NOT NULL,
		price       REAL,
		observed_at INTEGER NOT NULL,
		PRIMARY KEY (station_id, fuel, observed_at)
	)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS price_history_station_fuel_observed
		ON price_history (station_id, fuel, observed_at DESC)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS stations_lat_lng
		ON stations (lat, lng)`); err != nil {
		return err
	}
	return nil
}

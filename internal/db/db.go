package db

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
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
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS stations (
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
	)`)
	return err
}

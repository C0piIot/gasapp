package main

import (
	"flag"
	"gasapp/internal/db"
	"gasapp/internal/station"
	"log"
)

func main() {
	dbPath := flag.String("db", "db.sqlite3", "path to SQLite database")
	flag.Parse()

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatal("open db:", err)
	}
	defer database.Close()

	log.Println("fetching prices...")
	if err := station.UpdatePrices(database); err != nil {
		log.Fatal("update prices:", err)
	}
	log.Println("done")
}

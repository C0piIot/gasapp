// Dev tool: seed random price_history rows for every station so the
// station-info chart has something to draw locally. Not built or shipped with
// the server. Run via:
//
//	docker compose stop gas_app
//	mkdir -p /tmp/seed-work
//	docker cp gas_app:/data/db.sqlite3 /tmp/seed-work/db.sqlite3
//	docker run --rm -v "$PWD:/src" -v /tmp/seed-work:/data \
//	    -v gasapp-go-mod-cache:/go/pkg/mod \
//	    -v gasapp-go-build-cache:/root/.cache/go-build \
//	    -w /src golang:1.25-alpine \
//	    go run ./cmd/seed-history -db /data/db.sqlite3
//	docker cp /tmp/seed-work/db.sqlite3 gas_app:/data/db.sqlite3
//	docker compose start gas_app
package main

import (
	"flag"
	"log"
	"math"
	"math/rand"
	"sort"
	"time"

	"gasapp/internal/db"
)

func main() {
	dbPath := flag.String("db", "db.sqlite3", "path to SQLite database")
	seed := flag.Int64("seed", 42, "RNG seed for reproducibility")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatal("open db:", err)
	}
	defer database.Close()

	rows, err := database.Query(`SELECT id, gasoil, petrol95, petrol98, glp FROM stations`)
	if err != nil {
		log.Fatal("query stations:", err)
	}
	defer rows.Close()

	type stn struct {
		id     int64
		prices map[string]*float64
	}
	var stations []stn
	for rows.Next() {
		var id int64
		var gasoil, p95, p98, glp *float64
		if err := rows.Scan(&id, &gasoil, &p95, &p98, &glp); err != nil {
			log.Fatal(err)
		}
		stations = append(stations, stn{
			id: id,
			prices: map[string]*float64{
				"gasoil":   gasoil,
				"petrol95": p95,
				"petrol98": p98,
				"glp":      glp,
			},
		})
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}

	log.Printf("seeding history for %d stations", len(stations))

	tx, err := database.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(
		`INSERT OR IGNORE INTO price_history (station_id, fuel, price, observed_at) VALUES (?, ?, ?, ?)`,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	now := time.Now()
	inserts := 0
	for _, s := range stations {
		for fuel, current := range s.prices {
			if current == nil {
				continue
			}
			n := 5 + rng.Intn(4) // 5–8 observations across the window
			ts := make([]int64, n)
			for i := range ts {
				offsetSec := rng.Int63n(30 * 86400)
				ts[i] = now.Add(-time.Duration(offsetSec) * time.Second).Unix()
			}
			sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })

			prices := make([]float64, n)
			for i := range prices {
				drift := (rng.Float64() - 0.5) * 0.10 * *current // ±5%
				prices[i] = math.Round((*current+drift)*1000) / 1000
			}
			// Pin the last point to the current price so the chart's right
			// edge matches what stations.<fuel> currently reports.
			prices[n-1] = *current

			for i := range prices {
				if _, err := stmt.Exec(s.id, fuel, prices[i], ts[i]); err != nil {
					log.Fatal(err)
				}
				inserts++
			}
		}
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	log.Printf("inserted %d price_history rows", inserts)
}

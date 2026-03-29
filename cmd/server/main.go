package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"gasapp/internal/db"
	"gasapp/internal/station"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	dbPath := flag.String("db", "db.sqlite3", "path to SQLite database")
	addr := flag.String("addr", ":8080", "listen address")
	staticDir := flag.String("static", "static", "compiled static files directory (STATIC_ROOT)")
	templatesDir := flag.String("templates", "gasapp/templates", "templates directory")
	flag.Parse()

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatal("open db:", err)
	}
	defer database.Close()

	sc := &stationCache{db: database}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(*staticDir))))
	mux.HandleFunc("/stations/", stationsHandler(sc))
	mux.HandleFunc("/offline/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, *templatesDir+"/offline.html")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, *templatesDir+"/home.html")
	})

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// stationCache holds all recent stations in memory, refreshed every 24 hours.
type stationCache struct {
	mu       sync.RWMutex
	db       *sql.DB
	stations []station.Station
	expires  time.Time
}

func (sc *stationCache) get() ([]station.Station, error) {
	sc.mu.RLock()
	if time.Now().Before(sc.expires) {
		s := sc.stations
		sc.mu.RUnlock()
		return s, nil
	}
	sc.mu.RUnlock()

	sc.mu.Lock()
	defer sc.mu.Unlock()
	// Re-check after acquiring write lock (another goroutine may have refreshed).
	if time.Now().Before(sc.expires) {
		return sc.stations, nil
	}

	stations, err := station.All(sc.db)
	if err != nil {
		return nil, err
	}
	sc.stations = stations
	sc.expires = time.Now().Add(24 * time.Hour)
	return sc.stations, nil
}

func stationsHandler(sc *stationCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stations, err := sc.get()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println("station cache:", err)
			return
		}

		result := stations
		if center := r.URL.Query().Get("center"); center != "" {
			parts := strings.SplitN(center, ",", 2)
			if len(parts) == 2 {
				lat, errLat := strconv.ParseFloat(parts[0], 64)
				lng, errLng := strconv.ParseFloat(parts[1], 64)
				if errLat == nil && errLng == nil {
					result = station.ByDistance(stations, lat, lng, 200)
				}
			}
		}

		rows := make([][]any, len(result))
		for i, s := range result {
			rows[i] = []any{
				s.ID,
				s.Name,
				s.Petrol95,
				s.Petrol98,
				s.Gasoil,
				s.GLP,
				s.Address,
				s.City,
				s.PostalCode,
				[]float64{s.Lng, s.Lat},
				float64(s.Updated),
			}
		}

		data, err := json.Marshal(map[string]any{"stations": rows})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

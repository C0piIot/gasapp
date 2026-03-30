package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"gasapp/internal/db"
	"gasapp/internal/station"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type templateData struct {
	BuildVersion string
}

func main() {
	dbPath := flag.String("db", "db.sqlite3", "path to SQLite database")
	addr := flag.String("addr", ":8080", "listen address")
	staticDir := flag.String("static", "static", "static files directory")
	templatesDir := flag.String("templates", "templates", "templates directory")
	buildVersion := flag.String("build", "dev", "build version string")
	flag.Parse()

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatal("open db:", err)
	}
	defer database.Close()

	tmpl, err := template.ParseFiles(
		filepath.Join(*templatesDir, "home.html"),
		filepath.Join(*templatesDir, "offline.html"),
	)
	if err != nil {
		log.Fatal("parse templates:", err)
	}

	data := templateData{BuildVersion: *buildVersion}
	sc := &stationCache{db: database}

	go runUpdates(database, sc)

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(*staticDir))))
	mux.HandleFunc("/worker.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Service-Worker-Allowed", "/")
		http.ServeFile(w, r, filepath.Join(*staticDir, "worker.js"))
	})
	mux.HandleFunc("/stations/", stationsHandler(sc))
	mux.HandleFunc("/offline/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.ExecuteTemplate(w, "offline.html", data)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.ExecuteTemplate(w, "home.html", data)
	})

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// stationCache holds all recent stations in memory, refreshed after each update.
type stationCache struct {
	mu       sync.RWMutex
	db       *sql.DB
	stations []station.Station
	expires  time.Time
}

func (sc *stationCache) invalidate() {
	sc.mu.Lock()
	sc.expires = time.Time{}
	sc.mu.Unlock()
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

// runUpdates fetches prices immediately on startup, then repeats every hour.
// Mirrors the original entrypoint.sh while loop.
func runUpdates(database *sql.DB, sc *stationCache) {
	for {
		log.Println("updating prices...")
		if err := station.UpdatePrices(database); err != nil {
			log.Println("update prices:", err)
		} else {
			log.Println("prices updated")
			sc.invalidate()
		}
		time.Sleep(time.Hour)
	}
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

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

// buildVersion is set at build time via -ldflags "-X main.buildVersion=..."
var buildVersion = "dev"

type templateData struct {
	BuildVersion string
}

func main() {
	dbPath := flag.String("db", "db.sqlite3", "path to SQLite database")
	addr := flag.String("addr", ":8080", "listen address")
	staticDir := flag.String("static", "static", "static files directory")
	templatesDir := flag.String("templates", "templates", "templates directory")
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

	data := templateData{BuildVersion: buildVersion}
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

// stationCache holds all recent stations in memory, refreshed after each
// update. It also caches per-fuel daily price history aligned positionally
// with stations, so /stations/?fuel=... can answer without a DB roundtrip.
type stationCache struct {
	mu       sync.RWMutex
	db       *sql.DB
	stations []station.Station
	history  map[string][]station.DailyHistory
	endTs    int64 // window end (exclusive) used when computing history
	expires  time.Time
}

func (sc *stationCache) invalidate() {
	sc.mu.Lock()
	sc.expires = time.Time{}
	sc.mu.Unlock()
}

// snapshot returns the cached stations, the per-station history for the given
// fuel (parallel to stations), and the unix-second window-end timestamp used
// to compute that history. The cache is refreshed if expired.
func (sc *stationCache) snapshot(fuel string) ([]station.Station, []station.DailyHistory, int64, error) {
	sc.mu.RLock()
	if time.Now().Before(sc.expires) {
		s := sc.stations
		h := sc.history[fuel]
		end := sc.endTs
		sc.mu.RUnlock()
		return s, h, end, nil
	}
	sc.mu.RUnlock()

	sc.mu.Lock()
	defer sc.mu.Unlock()
	// Re-check after acquiring write lock (another goroutine may have refreshed).
	if time.Now().Before(sc.expires) {
		return sc.stations, sc.history[fuel], sc.endTs, nil
	}

	stations, err := station.All(sc.db)
	if err != nil {
		return nil, nil, 0, err
	}
	now := time.Now()
	history := make(map[string][]station.DailyHistory, len(station.AllowedFuels))
	for _, f := range station.AllowedFuels {
		h, err := station.HistoryByFuel(sc.db, f, stations, now)
		if err != nil {
			return nil, nil, 0, err
		}
		history[f] = h
	}
	sc.stations = stations
	sc.history = history
	sc.endTs = station.WindowEnd(now).Unix()
	sc.expires = now.Add(24 * time.Hour)
	return sc.stations, sc.history[fuel], sc.endTs, nil
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
		fuel := r.URL.Query().Get("fuel")
		if fuel == "" {
			fuel = "petrol95"
		}
		if !station.IsAllowedFuel(fuel) {
			http.Error(w, "unknown fuel", http.StatusBadRequest)
			return
		}

		stations, history, endTs, err := sc.snapshot(fuel)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println("station cache:", err)
			return
		}

		indices := make([]int, len(stations))
		for i := range stations {
			indices[i] = i
		}
		if r.URL.Query().Has("ids") {
			raw := r.URL.Query().Get("ids")
			parts := strings.Split(raw, ",")
			idxByID := make(map[int64]int, len(stations))
			for i, s := range stations {
				idxByID[s.ID] = i
			}
			indices = make([]int, 0, len(parts))
			for _, p := range parts {
				id, err := strconv.ParseInt(p, 10, 64)
				if err != nil {
					continue
				}
				if j, ok := idxByID[id]; ok {
					indices = append(indices, j)
				}
			}
		} else if center := r.URL.Query().Get("center"); center != "" {
			parts := strings.SplitN(center, ",", 2)
			if len(parts) == 2 {
				lng, errLng := strconv.ParseFloat(parts[0], 64)
				lat, errLat := strconv.ParseFloat(parts[1], 64)
				if errLat == nil && errLng == nil {
					indices = station.ByDistanceIndices(stations, lat, lng, 200)
				}
			}
		}

		rows := make([][]any, len(indices))
		for i, j := range indices {
			s := stations[j]
			var hist station.DailyHistory
			if history != nil {
				hist = history[j]
			}
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
				hist,
			}
		}

		data, err := json.Marshal(map[string]any{
			"stations":    rows,
			"history_end": endTs,
		})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

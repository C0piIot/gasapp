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
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// buildVersion is set at build time via -ldflags "-X main.buildVersion=..."
var buildVersion = "dev"

const (
	// nearbyLimit caps the map query and the unfiltered listing. The client
	// only draws what is in view, and every response carries price history, so
	// the row count has to stay bounded whichever way the query arrives.
	nearbyLimit = 200
	// maxIDs bounds the favorites query. It has to stay well clear of any
	// plausible favorites list: the client deletes from local storage every id
	// the response omits, so truncating here would wipe a user's favorites.
	maxIDs = 1000
)

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

	go runUpdates(database)

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(*staticDir))))
	mux.HandleFunc("/worker.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Service-Worker-Allowed", "/")
		http.ServeFile(w, r, filepath.Join(*staticDir, "worker.js"))
	})
	mux.HandleFunc("/stations/", stationsHandler(database))
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

// runUpdates fetches prices immediately on startup, then repeats every hour.
func runUpdates(database *sql.DB) {
	for {
		log.Println("updating prices...")
		if err := station.UpdatePrices(database); err != nil {
			log.Println("update prices:", err)
		} else {
			log.Println("prices updated")
		}
		time.Sleep(time.Hour)
	}
}

func stationsHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fuel := r.URL.Query().Get("fuel")
		if fuel == "" {
			fuel = "petrol95"
		}
		if !station.IsAllowedFuel(fuel) {
			http.Error(w, "unknown fuel", http.StatusBadRequest)
			return
		}

		stations, err := requestedStations(database, r.URL.Query())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println("stations:", err)
			return
		}

		now := time.Now()
		history, err := station.HistoryByFuel(database, fuel, stations, now)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println("station history:", err)
			return
		}

		rows := make([][]any, len(stations))
		for i, s := range stations {
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
				history[i],
			}
		}

		data, err := json.Marshal(map[string]any{
			"stations":    rows,
			"history_end": station.WindowEnd(now).Unix(),
		})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

// requestedStations resolves a query into the stations to return. Every branch
// is bounded, so attaching price history costs one bind parameter and one index
// seek per station returned rather than per station in the table.
func requestedStations(database *sql.DB, q url.Values) ([]station.Station, error) {
	if q.Has("ids") {
		return station.ByIDs(database, parseIDs(q.Get("ids")))
	}
	if center := q.Get("center"); center != "" {
		if lat, lng, ok := parseCenter(center); ok {
			return station.Nearby(database, lat, lng, nearbyLimit)
		}
	}
	return station.Listing(database, nearbyLimit)
}

func parseIDs(raw string) []int64 {
	parts := strings.Split(raw, ",")
	if len(parts) > maxIDs {
		parts = parts[:maxIDs]
	}
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// parseCenter reads the "lng,lat" pair the client sends: longitude first.
func parseCenter(raw string) (lat, lng float64, ok bool) {
	parts := strings.SplitN(raw, ",", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	lng, errLng := strconv.ParseFloat(parts[0], 64)
	lat, errLat := strconv.ParseFloat(parts[1], 64)
	return lat, lng, errLat == nil && errLng == nil
}

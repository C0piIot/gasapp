# ⛽ gasapp

[![Build](https://github.com/C0piIot/gasapp/actions/workflows/build.yml/badge.svg)](https://github.com/C0piIot/gasapp/actions/workflows/build.yml)

Spain gas stations price comparator — interactive map showing fuel prices at nearby stations.

https://gasapp.dropdatabase.es

## How it works

- On startup the server fetches all Spanish gas station prices from the [MINETUR REST API](https://sedeaplicaciones.minetur.gob.es/ServiciosRESTCarburantes/PreciosCarburantes/EstacionesTerrestres/) and repeats every hour.
- Prices are stored in a local SQLite database.
- The `/stations/` endpoint returns the 200 nearest stations to a given coordinate, sorted by distance using the Haversine formula.
- The frontend is a Leaflet map PWA with offline support via a service worker.

## Stack

- **Go 1.22** — single binary, no external process manager
- **modernc.org/sqlite** — pure-Go SQLite driver, no CGO required
- **Leaflet** + **leaflet.locatecontrol** — map and geolocation (loaded from CDN)

## Project layout

```
cmd/server/         HTTP server (price update loop + API + static serving)
internal/db/        SQLite schema and connection
internal/station/   Station model, store (upsert/query), XML price updater
templates/          HTML templates (home, offline)
static/             PWA assets (icons, fonts, service worker, manifests)
```

## Running locally

```bash
docker compose up
```

The server starts on `http://localhost:8080`. The database is created automatically on first run and prices are fetched immediately in the background.

## Running tests

```bash
docker run --rm -v $(pwd):/app -w /app golang:1.22-alpine go test ./...
```

## Deployment (Fly.io)

Deployments are triggered automatically on push to `master` via GitHub Actions. To deploy manually:

```bash
fly deploy --build-arg BUILD_VERSION=$(git rev-parse --short HEAD)
```

The build version is embedded in the binary at compile time and shown in the map attribution.

## Data source

Prices are fetched from Spain's Ministry of Economic Affairs:
[Precios Carburantes — MINETUR](https://sedeaplicaciones.minetur.gob.es/ServiciosRESTCarburantes/PreciosCarburantes/EstacionesTerrestres/)

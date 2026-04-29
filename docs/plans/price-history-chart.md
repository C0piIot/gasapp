# Price History Chart in the Station Panel

## Context

`docs/plans/price-history.md` shipped a `price_history` table that records a row per station-fuel only when the price changes. Today nothing reads it. The frontend (`templates/home.html`) shows a station's *current* prices in `#station-info` after a marker click, but a user can't see how those prices have moved over time.

The goal of this change is to show a small price-evolution chart inside `#station-info`, under the existing prices block, using the data already being recorded. Constraints from the user:

- **Minimize requests.** No new endpoint. History rides along with the existing `/stations/` response so the chart renders synchronously when a marker is clicked.
- **Minimize third-party dependencies.** No charting library. Stay consistent with the rest of the frontend (Leaflet + vanilla JS in a single template).

Scope decisions, made deliberately to keep payload small:

- **Only the user's preferred fuel** is sent. Switching fuel triggers a refetch (the existing `#fuel-selector` button handler already calls `updateStations(true)`).
- **Last 30 days, one datapoint per day.** Each datapoint is the **max price observed that day**; days with no observations carry the most recent prior value forward.
- **Implicit dates.** The wire format is a flat array of 30 numbers (or `null`s) representing the 30 most recent days, oldest first. Both server and client agree on the day boundaries from the response timestamp.

Sizing (sanity check, see prior conversation): ~30 numbers × ~6 bytes ≈ 190 B/station extra. With the existing 200-station ceiling that's ~38 KB raw added on top of the existing ~44 KB — roughly doubles the response, gzip would cut it to ~10–15 KB on the wire (server doesn't gzip today).

Non-goals:

- Retention/pruning of `price_history` (still out of scope, as in the previous plan).
- A dedicated history endpoint (`/history/{id}`). Explicitly skipped.
- Showing more than one fuel at a time, or a fuel-comparison view.
- Tooltips/hover interactions on the chart for v1 (see "Future work").
- Server-side response gzip (worth doing eventually, but separable from this change).

## Design

### Data flow

1. Client includes its currently selected `fuel` in the `/stations/` request (`?center=...&fuel=petrol95`).
2. Server returns each station with a `history` field: a 30-element array of `*float64` representing the daily-max price for that fuel over the last 30 days, with carry-forward filling.
3. On marker click, `showMarker` reads `s.history` from the already-fetched station data and renders the chart synchronously. No extra fetch.

This means the chart is bounded by the same 10-minute throttle as the rest of `/stations/` (`updateStations` already gates refetches), which is fine — prices update hourly upstream.

### Backend

#### Cache extension

The current `stationCache` in `cmd/server/main.go` holds `[]station.Station` and is invalidated after each successful price update (`runUpdates` → `sc.invalidate()`). Extend it to also hold the per-fuel daily history:

```go
type stationCache struct {
    mu       sync.RWMutex
    db       *sql.DB
    stations []station.Station
    history  map[string][]station.DailyHistory // keyed by fuel: "petrol95", etc.
    expires  time.Time
}
```

Each `station.DailyHistory` is one station's 30-day vector for that fuel, in the same order as `stations` so we can join positionally without an id lookup.

Memory cost: worst case ~10K stations × 4 fuels × 30 × 8 bytes ≈ ~10 MB. Fine for a single-process server.

`stationCache.get()` is replaced by `stationCache.snapshot(fuel string) ([]station.Station, []station.DailyHistory, error)`. On cache miss it loads stations (as today) and computes the four history maps in one pass over the relevant rows. `invalidate()` is unchanged — clears `expires` so the next `snapshot` rebuilds.

We compute history for all four fuels at refresh time (not lazily per request) because the work is bounded, the input is the same scan of `price_history`, and it avoids per-request branches for the locking logic.

#### Store

New type and function in `internal/station/`:

```go
// internal/station/model.go
type DailyHistory [30]*float64 // oldest-first; nil = no known price for that day

// internal/station/store.go
// HistoryByFuel returns the per-station 30-day daily-max history for one fuel,
// in the same order as `stations`. Days with no observation carry forward the
// most recent prior known price; nil means no price was ever recorded by that day.
//
// `now` is passed in (rather than read from time.Now) so tests can pin the window.
func HistoryByFuel(db *sql.DB, fuel string, stations []Station, now time.Time) ([]DailyHistory, error)
```

Implementation sketch:

1. Compute the window: `endTs := startOfUTCDay(now).Add(24*time.Hour)`, `startTs := endTs.Add(-30 * 24 * time.Hour)`. Day buckets are `[startTs+24h*i, startTs+24h*(i+1))` for `i ∈ [0, 30)`.
2. **Seed query** — most recent observation strictly before the window per station (carry-forward source for day 0):
   ```sql
   SELECT station_id, price
   FROM price_history ph
   WHERE fuel = ?
     AND observed_at < ?
     AND observed_at = (
       SELECT MAX(observed_at) FROM price_history
       WHERE station_id = ph.station_id AND fuel = ph.fuel AND observed_at < ?
     )
   ```
   The composite index `price_history_station_fuel_observed (station_id, fuel, observed_at DESC)` makes this efficient.
3. **In-window query** — every change inside the window:
   ```sql
   SELECT station_id, observed_at, price
   FROM price_history
   WHERE fuel = ? AND observed_at >= ? AND observed_at < ?
   ORDER BY station_id, observed_at
   ```
4. Walk the rows in Go: for each station, start with the seed value (or nil if none), then for each day bucket take `max(observations in bucket)` if any, else carry the previous day's value.
5. Return slice ordered to match the input `stations` slice. Stations with no history at all keep all-nil arrays.

This produces a dense 30-element array per station, so the JSON serializer emits a flat list with `null` for unknown days — no client-side gap-filling needed.

Float `max`: straight `>` comparison on dereferenced `*float64`. NULL prices in `price_history` mean "fuel disappeared" — treat them as a real observation that resets carry-forward to nil for that day's bucket and forward.

#### Handler

Update `stationsHandler` in `cmd/server/main.go`:

- Parse `?fuel=` (default `"petrol95"`, validate against the four allowed values; reject others to a 400 to avoid empty-history confusion).
- Call the new `sc.snapshot(fuel)` instead of `sc.get()`.
- Pass `history[i]` through `ByDistance` filtering: since `ByDistance` reorders/limits, the parallel slice must be filtered together. Easiest: zip into a small slice of `(Station, DailyHistory)` pairs before/after the filter, or run the filter to produce indices and re-project both slices. **Cleanest fix**: change `ByDistance` to also accept and return the history slice, or refactor to return the chosen indices.

  Recommended: refactor `ByDistance` to return `[]int` indices (or split into `ByDistanceIndices`); apply those to both `stations` and `history`. Keeps the two slices aligned and avoids materializing a temporary pair struct just for one call site. Update the existing `TestByDistance` (if any) accordingly.

- Append the daily history as a new positional element on each row:

  ```go
  rows[i] = []any{
      s.ID, s.Name,
      s.Petrol95, s.Petrol98, s.Gasoil, s.GLP,
      s.Address, s.City, s.PostalCode,
      []float64{s.Lng, s.Lat},
      float64(s.Updated),
      history[i], // []*float64 of length 30
  }
  ```

  `*float64` marshals as a JSON number or `null`, which is what we want.

The response top-level shape gains one field so the client knows how to map day index → real date without hardcoding clock state:

```json
{
  "stations": [...],
  "history_end": 1714521600   // unix seconds for the exclusive upper bound of the last day bucket
}
```

The client renders chronologically without knowing exact per-day dates (the chart only labels first/last); but `history_end` lets us label the right edge as "today" reliably.

#### Files to modify (backend)

- **`internal/station/model.go`** — add `DailyHistory` type.
- **`internal/station/store.go`** — add `HistoryByFuel`. Refactor `ByDistance` to return indices (or add a sibling helper used by the handler).
- **`internal/station/store_test.go`** — add `TestHistoryByFuel` (see Tests). Update any existing `TestByDistance` for the new signature.
- **`cmd/server/main.go`** — extend `stationCache` to carry per-fuel history, replace `get()` with `snapshot(fuel)`, parse `fuel` query param, append history to the row.
- **`cmd/server/main_test.go`** — new file, `TestStationsHandlerHistory` (see Tests).

No changes to `internal/station/updater.go` or `internal/db/db.go`. The hourly invalidation already drops history along with stations.

### Frontend

All changes in `templates/home.html`. No new files.

#### Markup

Add an `<svg>` placeholder inside `#station-info`, after the `prices` paragraph and before `<address>`:

```html
<svg id="price-chart" class="hidden" viewBox="0 0 320 120" preserveAspectRatio="none"></svg>
```

CSS additions:

```css
#price-chart { width: 100%; height: 120px; display: block; margin: 0.5rem 0; }
#price-chart .axis { stroke: #ccc; stroke-width: 1; }
#price-chart .line { fill: none; stroke: dimgray; stroke-width: 2; }
#price-chart text { font-size: 10px; fill: #555; }
```

Single line color since we only render one fuel — no need for a per-fuel palette.

#### Script

Two changes to the existing JS:

1. **`updateStations` URL** — append `&fuel=${fuel}` to the query string. The existing 10-minute throttle already keys off `latestFetchUrl`, so changing fuel naturally bypasses it (the URL differs). The fuel selector's button handler already calls `updateStations(true)`, which re-fetches; that path now picks up the new fuel automatically.

2. **`showMarker`** — destructure `history` as the 12th element of the station tuple, alongside the existing fields:

   ```js
   [s.pk, s.name, s.petrol95, s.petrol98, s.gasoil, s.glp,
    s.address, s.city, s.postal_code, s.location, s.updated, s.history] = station;
   ```

   At the end of `showMarker`, draw the chart:

   ```js
   const chart = qs('#price-chart');
   chart.replaceChildren();
   if (Array.isArray(s.history) && s.history.some(v => v !== null)) {
       renderChart(chart, s.history, response.history_end);
       chart.classList.remove('hidden');
   } else {
       chart.classList.add('hidden');
   }
   ```

   `response.history_end` needs to be in scope — `updateStations` should stash it on a module-level `let historyEnd` after parsing, similar to how `latestFetchTime` is stored.

3. **`renderChart(svgEl, daily, endTs)`** — ~50 lines of vanilla JS, all SVG primitives:

   - Filter to known points: `daily.map((p, i) => ({i, p})).filter(({p}) => p !== null)`.
   - X scale: bucket index `i ∈ [0, 30)` → SVG x in `[0, 320]`.
   - Y scale: `[min, max]` of known prices, padded by ~5%, mapped to SVG y in `[110, 10]` (inverted).
   - Path: build a step polyline. For each pair `(i_n, p_n) → (i_{n+1}, p_{n+1})`, draw `(x_n, y_n) → (x_{n+1}, y_n) → (x_{n+1}, y_{n+1})`. A `null` after a known value lifts the pen; the next known value starts a new sub-path. Use a single `<path>` with `M`/`L` commands to keep it simple.
   - Two text labels: min and max y on the left; first-day and last-day dates on the bottom (computed from `endTs` and the 30-day window).
   - Clear with `replaceChildren()` before drawing so reopening a station doesn't stack SVGs.

#### Behavior on edge cases

- **All-null history** (brand-new station, or fuel never sold): chart hidden, rest of panel renders unchanged.
- **One known datapoint**: render a flat horizontal line spanning the known range (degenerate but informative).
- **All same value**: y range is zero; clamp to a small epsilon so the line shows centered.
- **User switches fuel while a panel is open**: the existing handler already calls `updateStations(true)`; the panel is hidden by the fuel-selector flow, so this is already handled.

### Tests

Backend:

- **`TestHistoryByFuel`** in `internal/station/store_test.go` — real SQLite via `db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))`. Pin `now` to a fixed timestamp so day boundaries are deterministic. Cases:
  - Station with no history → all-nil 30-element array.
  - Station with one observation strictly before the window → carry-forward fills all 30 days with that value.
  - Station with two observations within the window on different days → step pattern visible at the right indices.
  - Station with two observations on the **same day** at different prices → bucket holds the max.
  - Station with a NULL-price observation (fuel disappeared) → that day and forward are nil until the next non-nil observation.
  - Two stations interleaved → result slice aligns with input `[]Station` order.
- **`TestStationsHandlerHistory`** in `cmd/server/main_test.go` — `httptest.NewRequest("GET", "/stations/?fuel=petrol95", nil)`. Cases:
  - Happy path: each row has 12 elements, the 12th is a 30-element array, response includes `history_end`.
  - Default fuel: missing `fuel=` defaults to `petrol95`.
  - Invalid fuel: `?fuel=hydrogen` → 400.
  - Cache reuse: second call doesn't re-query the DB (assert via a counter or by verifying `expires` didn't reset).

Frontend: no automated tests (no JS test infra in repo). Manual smoke check after running the server against a copy of `db.sqlite3`:

1. Open a station with multi-day history → step line visible, sensible min/max labels.
2. Open a brand-new station with no history → chart hidden, rest of panel intact.
3. Switch fuel via the selector → `/stations/` is refetched with the new `fuel=`, reopen station, chart now reflects the new fuel.
4. Network tab: panning the map within 10 minutes does **not** refetch (existing throttle still applies).
5. Same-station re-open without panning → no extra request, chart redraws from already-cached station data.
6. With DevTools throttling on Slow 3G: the response is still small enough that panning feels acceptable. If not, that's signal to add gzip middleware (separate PR).

## Files to modify

- `internal/station/model.go` — add `DailyHistory` type.
- `internal/station/store.go` — add `HistoryByFuel`; refactor `ByDistance` (or add a sibling) so the handler can keep `stations` and `history` aligned after distance-filtering.
- `internal/station/store_test.go` — add `TestHistoryByFuel`; update any test affected by the `ByDistance` refactor.
- `cmd/server/main.go` — extend `stationCache` with per-fuel history, replace `get()` with `snapshot(fuel)`, parse and validate `?fuel=`, append history + `history_end` to the response.
- `cmd/server/main_test.go` — new file, `TestStationsHandlerHistory`.
- `templates/home.html` — `<svg>` element, CSS block, `renderChart` function, `updateStations` URL change, `showMarker` chart hook.

No changes to `internal/station/updater.go`, `internal/db/db.go`, or build/CI configs.

## Verification

1. Ensure the shared pre-commit hook is enabled: `git config core.hooksPath .githooks` (per `AGENTS.md`).
2. `go test ./...` locally.
3. Run the server against a copy of the prod-like DB: `cp db.sqlite3 /tmp/db.sqlite3.bak && go run ./cmd/server -db /tmp/db.sqlite3.bak`. Hit `curl 'http://localhost:8080/stations/?fuel=petrol95' | jq '.stations[0]'` and eyeball: the row should have 12 elements with the 12th being a 30-length array.
4. Open `http://localhost:8080/`, pan to a populated area, click stations, run through the manual checks above.
5. CI runs `go test ./...` automatically; the test job must stay green for the deploy job to run.

## Future work (not in this PR)

- Gzip the `/stations/` response (~20 lines of middleware). Worth doing once payload meaningfully grew with history.
- Hover tooltip with exact price/date — needs ~30 lines of JS and a transparent overlay rect for hit-testing.
- A configurable window (`?days=N`) if 30 days turns out to be too short or too long.
- A second line for a comparison fuel, if user feedback asks for it (revisit payload sizing first).

# Price History for Gas Stations

## Context

Today, every hourly run of `station.UpdatePrices` calls `upsertTx` which **overwrites** the four price columns on `stations` (`gasoil`, `petrol95`, `petrol98`, `glp`). Once a price changes, the previous value is gone — there is no way to chart a station's prices over time, detect outliers, or answer "when did this station last raise petrol95?".

The goal is to retain a per-fuel time series. Each hourly update should record a new history row **only when a fuel's price differs from the most recent stored value** (including NULL transitions, per user decision), so we don't bloat the table with unchanged rows when the feed is identical hour-to-hour.

Non-goals (out of scope):
- Query/HTTP endpoints for reading history.
- Frontend visualizations.
- Retention/pruning policy.

## Design

### Schema

New table, added to `migrate()` in `internal/db/db.go`:

```sql
CREATE TABLE IF NOT EXISTS price_history (
    station_id  INTEGER NOT NULL,
    fuel        TEXT    NOT NULL,          -- 'gasoil' | 'petrol95' | 'petrol98' | 'glp'
    price       REAL,                       -- NULL means the fuel disappeared from the feed
    observed_at INTEGER NOT NULL,           -- station's Updated unix timestamp
    PRIMARY KEY (station_id, fuel, observed_at)
);
CREATE INDEX IF NOT EXISTS price_history_station_fuel_observed
    ON price_history (station_id, fuel, observed_at DESC);
```

Notes:
- `price` is **nullable** because we record value→NULL transitions ("station stopped selling GLP"). The composite PK still uniquely identifies a row.
- `observed_at` uses the station's `Updated` field (already an INTEGER unix timestamp on the row) — the same value that lands in `stations.updated`. This keeps history consistent with what `stations` reports.
- No FK to `stations(id)` — keeps the design symmetrical with the existing schema (no FKs anywhere) and avoids ON DELETE concerns. PK + index is enough for the read patterns we'll add later.

### Change detection — Go side, inside the existing tx

Detection lives in `upsertTx` (`internal/station/store.go`) so it runs inside the **same transaction** as `parseStream`'s bulk update — either the entire hour's update succeeds or none of it does. This preserves the existing all-or-nothing guarantee.

For each station being upserted:

1. Load the existing row's four prices via a new helper:
   ```go
   func loadCurrentPrices(tx *sql.Tx, id int64) (gasoil, petrol95, petrol98, glp *float64, found bool, err error)
   ```
   Returns `found=false` if the station is brand-new (first sight).
2. For each of the four fuels, compare old `*float64` vs new `*float64`:
   - `found=false` and new is non-nil → **insert** initial row.
   - `found=false` and new is nil → skip.
   - `*old == *new` (both non-nil, equal) → skip.
   - both nil → skip.
   - any other case (nil↔non-nil, or different values) → **insert** a row with the new value (which may be NULL).
3. Insert all changed-fuel rows in a single batched `INSERT INTO price_history ... VALUES (?,?,?,?), (?,?,?,?), ...` so we don't pay 4 round-trips per station.
4. Run the existing UPSERT against `stations` (unchanged).

Float comparison uses straight `==` on the dereferenced `float64`. The feed exposes prices to 3 decimals (Spanish-comma format like `1,659`); `spanishFloat` always rounds to the same parse result for the same string, so `==` is fine and matches what the user intuitively means by "different".

A small helper keeps the logic readable:

```go
// priceChanged reports whether new should be recorded as a history row given old.
// Returns (recordRow, priceToWrite). priceToWrite may be nil for value->NULL.
func priceChanged(old, new *float64) (bool, *float64)
```

### Files to modify

- **`internal/db/db.go`** — add `CREATE TABLE price_history` + index to `migrate()` (the existing `CREATE TABLE IF NOT EXISTS` pattern means re-running on existing DBs is safe).
- **`internal/station/store.go`** — add `loadCurrentPrices`, `priceChanged`, history INSERT SQL constant, and wire them into `upsertTx` before the existing UPSERT.
- **`internal/station/store_test.go`** — new tests (see below).
- **`internal/station/updater_test.go`** — extend `TestParseStream` to assert one history row is written per fuel on first sight.

`internal/station/model.go`, `updater.go`, `cmd/server/main.go`, and the cache logic are unchanged. Cache invalidation already happens after `UpdatePrices` returns, so no new invalidation paths.

### Tests

Follow the existing patterns: real SQLite via `db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))`, table-driven where it fits, sub-tests with `t.Run`.

In `store_test.go`, add:

1. **`TestPriceChanged`** — table-driven for the `priceChanged` helper. Cases: both nil, both equal, both differ, old nil/new non-nil, old non-nil/new nil, very small diff like `1.659` vs `1.660`.
2. **`TestUpsertRecordsInitialPrices`** — fresh DB, upsert a station with three of four fuels set; assert 3 rows in `price_history` with correct `(station_id, fuel, price, observed_at)`.
3. **`TestUpsertSkipsUnchangedPrices`** — upsert a station, then upsert the same station again with the same prices and a different `Updated`; assert `price_history` row count is unchanged.
4. **`TestUpsertRecordsChangedPrice`** — upsert, change only `petrol95`, upsert again; assert exactly one new row, for `petrol95`, with the new value and the new `observed_at`.
5. **`TestUpsertRecordsPriceDisappearance`** — upsert with `petrol98` set, then upsert with `petrol98 = nil`; assert one new row for `petrol98` with `price IS NULL`.
6. **`TestUpsertHistoryAtomicWithStation`** — `upsertTx` inside a tx that's then rolled back: assert neither `stations` nor `price_history` reflects the write (proves the history insert participates in the same tx).

In `updater_test.go`, extend `TestParseStream` (or add a sibling test) to assert that after parsing the canned XML, `price_history` has the expected initial rows for the one station that had prices.

## Verification

After implementing:

1. Ensure the shared pre-commit hook is enabled: `git config core.hooksPath .githooks` (per `AGENTS.md`) — it runs `go test ./...` in Docker before each commit.
2. `go test ./...` locally for fast iteration.
3. Manual smoke check against a copy of `db.sqlite3`:
   - Back up the file: `cp db.sqlite3 /tmp/db.sqlite3.bak`.
   - Run the server briefly so one update cycle completes.
   - `sqlite3 db.sqlite3 'SELECT COUNT(*) FROM price_history;'` — should equal the number of fuels-with-prices across all stations on first run.
   - Run a second update cycle; row count should grow only by the number of fuels whose price actually moved between hours (typically a small fraction of the total).
   - Spot-check one station: `SELECT fuel, price, observed_at FROM price_history WHERE station_id = <id> ORDER BY observed_at;` — values should match the current `stations` row's prices for the latest `observed_at`.
4. CI runs `go test ./...` automatically (`.github/workflows/build.yml`); the test job must stay green for the deploy job to run.

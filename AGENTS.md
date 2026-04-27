# AGENTS.md

Guidance for AI coding agents working in this repo. Read before making changes.

## Workflow

- `master` is a protected branch. **Do not push directly** — open a PR, even for trivial changes.
- Before committing, make sure the shared pre-commit hook is enabled: `git config core.hooksPath .githooks`. It runs `go test ./...` in Docker.
- CI (`.github/workflows/build.yml`) runs the full test suite before deploying to Fly. A red `test` job blocks the deploy — don't merge with failing tests.
- Don't bypass the pre-commit hook (`--no-verify`) unless the user explicitly asks.

## Tests are part of every change

- **Any behavior change ships with a test in the same PR.** Bug fix → regression test. New function → unit test. New handler branch → handler test.
- Pure logic functions: prefer **table-driven tests** (see `TestSpanishFloat`, `TestHaversine`).
- DB code: use a real SQLite file via `db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))` — do **not** mock `*sql.DB`. There's a project preference for real SQLite in tests because the schema and SQL behavior matter.
- HTTP handlers: use `httptest.NewRequest` + `httptest.NewRecorder`, call the handler factory directly (e.g. `stationsHandler(sc)(w, req)`).
- Parsers: feed canned input through `strings.NewReader` (see `TestParseStream`) — don't depend on the network.
- Use `t.Helper()` in test helpers; group related cases with `t.Run`.
- If you add a function that's genuinely not worth testing (one-line glue, untestable boundary), say so in the PR description rather than skipping silently.

## Coding style

- Target Go version is in `go.mod` (currently 1.25). Don't pin a different version in CI or Dockerfiles.
- Standard `gofmt` / `go vet` — no extra linters configured, so don't introduce style churn.
- Keep functions small and named for what they return, not how they work (see `firstNonEmpty`, `spanishFloat`, `titleCase`).
- Prefer pure helpers over methods on types when there's no state.
- Nullable values from the data feed use `*float64` (see `Station.Petrol95` etc.) — preserve that pattern; don't substitute sentinel zeros.
- Comments: default to none. Only write a comment when the *why* is non-obvious (e.g. `Use curl for price fetching — Go TLS fingerprint rejected by MINETUR`). Don't restate what the code does.
- No emojis in code or commit messages.

## Project specifics worth knowing

- The MINETUR XML uses encoded element names like `Precio_x0020_Gasolina_x0020_95_x0020_E5`. Don't "clean up" those literals — they're the wire format.
- Prices are fetched via `curl` subprocess on purpose; Go's TLS client gets blocked by the upstream. Don't refactor it back to `net/http` without coordinating.
- `stationCache` in `cmd/server/main.go` is intentionally invalidated after each price update. If you add code paths that mutate stations, invalidate the cache.
- Spanish decimals use commas (`1,659`); use `spanishFloat` to parse anything coming from the feed.

## Commit messages

Match the existing style: short, imperative, one-line summary, optional em-dash explanation. Examples from history:
- `Wrap station upserts in a transaction to fix missing city stations`
- `Fix swapped lng/lat parsing in center query parameter`
- `Use curl for price fetching — Go TLS fingerprint rejected by MINETUR`

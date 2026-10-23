# Task evidence — S00-T09

Avoid unchanged paper-state rewrites while preserving expiry and financial safety.

| Field | Value |
|---|---|
| Task / owner / reviewer | `S00-T09` / `claude-code-agent` (session `2026-10-23-development-8da61cee`) / **independent review pending** |
| Code commits | Backend `dcbfd2658348da10a9b0e545b14173be7bdc85f8`, then the HEAD fix in the commit that also updates this record; frontend `9c75c9183df2ed9457012260bb904a9ea404a960` (Playwright port setting only). |
| Baseline for "before" | Backend `9b3afb0ea62aca0555fc3aec8f19e070ad73b098`, the last commit without S00-T09 code |
| Environment | macOS 27.0.1 arm64; Go 1.26.10; PostgreSQL 16.15; Docker 29.7.2; Playwright 1.64.1 Chromium. Synthetic paper data only. |
| Normal app | Not used or restarted. Normal paper document MD5 `51285359c2d98893a4d47ead4c2fb4da` before and after all runs; 8080/5173 answered throughout. |

## What changed

- **Reads.** GET routes (`/health`, `/dashboard`, `/risk/policy`), and HEAD on them, use `store.Read`: one plain `SELECT` of the committed document, no row lock, no `UPDATE`. Nothing is cached; every request reads PostgreSQL.
- **Due expiry.** `paper.State.ExpiryDue` uses exactly the condition `Expire` acts on (an active decision at or past `expires_at`). When a read finds expiry due, `expireLocked` re-reads the row `FOR UPDATE` and writes only if **that** latest state still has expiry work. Readers that queued behind another request's expiry commit without an `UPDATE`.
- **Writes.** Only POST goes through `store.Execute`, unchanged (risk policy, receipts, conflict handling, rollback, validation). Every other method that reaches a handler reads; other methods (PUT, DELETE, PATCH, OPTIONS) are refused by the router with 405 and never reach the store.
- **Isolation for tests.** Optional `TS_DASHBOARD_ORIGIN`, accepted only as `http://127.0.0.1:<port>` or `http://localhost:<port>`; without it the two 5173 origins apply as before. `scripts/test-e2e.sh` takes `TS_E2E_API_PORT` / `TS_E2E_FRONTEND_PORT`; Playwright takes `FRONTEND_PORT`.

## Acceptance criteria

| Criterion | Outcome | Evidence |
|---|---|---|
| Unchanged authenticated reads and health checks do not rewrite the document or `updated_at`; no stale cache | **Met.** 400 parallel dashboard reads plus health checks: 0 updates (exact trigger count), `xmin`, `updated_at` and document hash unchanged. Benchmark: 0 updates in both runs (before: 414 and 424). | `TestReadPathWritesOnlyForDueExpiry` step 1; benchmark below |
| Due expiry still releases funds and persists journal and audit exactly once, including concurrent reads, retries and competing writes | **Met.** 12 readers queued behind a held row lock at the exact expiry instant: exactly 1 update, 1 journal line, 1 audit event, 1 notice; every reader saw EXPIRED with the reservation released. A second expiry raced by 8 readers, 3 competing writes and a retried write: still one journal line and one audit event; reads added at most one write. | same test, steps 2–3 |
| Idempotency, rollback, risk, human-only lifecycle, validation and auth unchanged; no broker or live behavior | **Met.** POST path untouched. Retried key returns the same body; key reuse with another body refused; a failed write rolls back (0 updates). Existing 24-way reservation race and 16-way fill race pass. GET routes never call the write path, and no GET handler changes state. Auth, origin and broker-route tests pass. | `TestPostgresAtomicityPersistenceAndConcurrency`; `TestReadsNeverTakeTheWritePath`; `TestHTTPBoundary` |
| Fixed-history before/after read workload with SQL write counts and latency; no invented targets; normal data and accepted behavior preserved | **Met.** See benchmark. Isolated real-API browser E2E passes on private ports. | `scripts/paper-read-bench.sh`; E2E below |

## Review finding fixed after the first checkpoint

Independent review of `1aec76c` found that Go's router also serves HEAD on GET routes, while the dispatch sent only GET to `Read`, so HEAD requests took the write path (3 unwanted updates for 3 HEAD requests). Fixed by letting only POST write. Real-PostgreSQL proof through the real HTTP handler (`TestHTTPReadMethodsNeverWriteThePaperRow`): 60 GET and HEAD requests on the three routes plus 3 refused methods cause **0** updates, and one POST causes exactly 1. With the old dispatch the same test fails with "caused 30 updates" (the 30 HEAD requests).

## Regression proof against the first draft

The first draft fell back to plain `Execute` when expiry was due. With that code the queued-readers test failed: "expiry with 12 queued readers caused 12 updates, want exactly 1". With `expireLocked` it passes with exactly 1.

## Fixed-history benchmark

`scripts/paper-read-bench.sh 9b3afb0…`: for each binary, a fresh database on the spike's isolated PostgreSQL (port 55532), API on 18180, the same synthetic history through the real API (1 account, fixtures, 400 evaluations → 400 recommendations, 402 audit events, a 1.13 MB document, a 682 KB dashboard response), then 10 seconds of reads: 8 concurrent dashboard readers and 1 health poller. Updates were counted by a trigger installed only in the benchmark database.

| | Before, run 1 | After, run 1 | Before, run 2 | After, run 2 |
|---|---|---|---|---|
| Dashboard reads per second | 36.0 | **188.6** | 36.9 | **178.5** |
| Dashboard p50 / p95 / p99 (ms) | 215 / 270 / 301 | **41.6 / 55.0 / 62.8** | 214 / 231 / 244 | **43.2 / 59.1 / 88.5** |
| Health checks per second (p50 ms) | 4.5 (215) | 25.0 (39.5) | 4.7 (214) | 23.8 (40.8) |
| SQL UPDATEs during the reads | 414 | **0** | 424 | **0** |
| Row version / `updated_at` changed | yes / yes | **no / no** | yes / yes | **no / no** |
| Table size on disk during the reads | 36.2 → 110.6 MB | 35.7 → 35.7 MB | 36.1 → 111.6 MB | 36.7 → 36.7 MB |
| Errors | 0 | 0 | 0 | 0 |

Read throughput rose 5.2× and 4.8×; median latency fell to about one fifth. These are local numbers on one machine, not targets (D-07 owns targets).

## Commands and results (code commits above)

| Command | Result |
|---|---|
| `gofmt`, `go vet ./...`, `go build ./...` | clean |
| `go test -race -count=1 ./...` | PASS: 35 top-level tests; PostgreSQL tests skip without a database |
| `./scripts/test-integration.sh` | PASS: 26 top-level tests (store 3, research 23), 0 failed |
| `python3 -B -m unittest discover -s research -p 'test_*.py'` | PASS, 27 tests |
| `python3 -B scripts/test_disposable_api.py` | PASS, 10 tests |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | No vulnerabilities found |
| `planctl.py validate`, `render --check` | PASS (17 sprints, 106 tasks); 18 documents unchanged |
| Frontend `npm run build`, `npx vitest run src`, `npm audit --audit-level=high` | PASS; 18 unit tests; 0 vulnerabilities |
| `TS_E2E_API_PORT=18090 TS_E2E_FRONTEND_PORT=15173 ./scripts/test-e2e.sh` (first checkpoint) and on 18095/15175 after the HEAD fix | PASS both times: 7 passed, 1 skipped (real-SEC test, by design), including the real-API paper lifecycle |
| `scripts/paper-read-bench.sh 9b3afb0…`, twice | PASS, same history both times, results above |

## Limits and residual risks

- **Writes still serialize and rewrite the growing document**, so write latency still rises with history. The relational ledger (original S02 tasks) is the real fix.
- Health still decodes the whole document (about 40 ms here while 8 readers compete for CPU).
- A read shows the latest committed snapshot; a write committing a moment later is not included. Any due expiry is committed by the next request that finds it due.
- `Execute` still writes on idempotent replays; that path was out of scope and is unchanged.
- ADR-017 now says that writes serialize and that reads use a lock-free snapshot (corrected with the HEAD fix).

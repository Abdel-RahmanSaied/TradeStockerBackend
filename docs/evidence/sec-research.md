# Task evidence — S07-T07

Official SEC research, durable jobs and dashboard integration.

| Field | Value |
|---|---|
| Task / owner / reviewer | `S07-T07` / `claude-code-agent` (session `2026-10-23-development-8da61cee`) / **independent review in progress** (Codex coordinates; backend checkpoint `7ab4418` accepted by a separate reviewer; see `sec-research-independent-review.md`, written by the reviewer) |
| Verified commits | Backend `7cf3123ed47ded23f3f74968c98f5bd672855f9c`: Go and Python code identical to the separately accepted `7ab44180b15cafbb16f7f3a5141bd8dfcdada440` except comments, plus docs and isolated test runners. Frontend `2e1841916eecefbb954a3c304d965cff3cf93744`: the accepted `7c739f5beb51089280598804e1499d4691cc05c4` plus one label correction. This update is a docs-only commit on top. |
| Environment and data mode | macOS arm64; Go 1.26.10 (selected by `go.mod`; the installed system Go is 1.26.6); pgx 5.11.0; PostgreSQL 16.15 in Docker; Python 3.9.6 (system); Node 26.7.0; Playwright 1.64.1 Chromium. Real public SEC filings for research runs; synthetic SEC-shaped data in tests; paper state not involved. |
| Run time | Code battery on backend `5da12da` (same Go/Python code): 2026-10-23, finished 06:25:48 UTC. Browser and smoke reruns on `7cf3123` / `2e18419`: 06:31:40–06:32:28 UTC. |

## Acceptance criteria

| Criterion | Outcome | Evidence |
|---|---|---|
| Authenticated idempotent durable jobs survive retries/restart; fetches bounded and limited to official SEC endpoints | **Met.** Token and origin checks wrap every research route. 12 concurrent same-key requests made one job; a different body with the same key got 409. Retry with backoff, lease recovery after a crash, re-queue on shutdown, restart on a new pool, attempt fencing, a visible failure for an interrupted last attempt, and a durable SEC-wide cooldown that holds across restarts. Only `https://data.sec.gov` URLs from a validated CIK; redirects refused; 16 MiB per source; 1 request per second. | `internal/research/research_integration_test.go`, `provider_test.go`, `internal/httpapi/server_test.go` |
| Reports retain source hashes, retrieval/filing times, units and accounting periods; incomplete evidence blocks entry and backtest use | **Met.** Originals are stored with SHA-256 before analysis and are append-only. Each fact carries tag, unit, period, filing, acceptance-based availability, source hash and JSON pointer. Every report has `outcome: INSUFFICIENT_DATA` and `eligible_for_entry` / `eligible_for_backtest` false, enforced again by the Go output gate. | `analyze_test.go`, `research/test_financials.py`, smoke run below |
| Python derives verified annual metrics with explicit missing data and counter-evidence; cannot write financial state | **Met.** Each year comes from its original 10-K. Conflicting values are withheld (in any row order); periods that end after their filing are rejected; later restatements are shown; missing inputs are named, never zero. Python runs with `-I` and only `PATH` and `LANG`; it has no database URL, token or SEC contact. | `research/test_financials.py` (27 tests), `TestPythonReceivesNoSecrets` |
| Frontend launches, polls and displays reports, errors, provenance and limitations from actual APIs | **Met.** Real-API browser test runs Apple end to end, checks five 10-K rows, downloads the original companyfacts and matches its SHA-256 in the test, and shows a visible failure for a ticker/CIK mismatch. Timing races are pinned with a controlled API double. | `tests/research.spec.ts`, `tests/research-ui.spec.ts` (frontend) |

## Commands and results (exact commits above)

| Command | Result | Count |
|---|---|---|
| `gofmt -l .`; `go build ./...`; `go vet ./...` | clean | — |
| `go test -race -count=1 ./...` | PASS | 30 top-level tests; 11 PostgreSQL tests skip without a database |
| `./scripts/test-integration.sh` | PASS on a disposable `tradestocker_test_*` database | 24 top-level tests (store 1, research 23), 0 failed |
| `python3 -B -m unittest discover -s research -p 'test_*.py'` | PASS on Python 3.9.6 | 27 tests |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | No vulnerabilities found | — |
| Frontend `npm run build`; `npx vitest run src`; `npm audit --audit-level=high` | PASS; 0 vulnerabilities | 18 unit tests |
| `python3 -B scripts/test_disposable_api.py` | PASS | 8 tests |
| `test-e2e.sh` and `research-smoke.sh` with a stray server on 18080 | both refused before creating a database (exit 1, "Port 18080 is busy") | 0 test databases created |
| `./scripts/test-e2e.sh` (no SEC contact), on `7cf3123` / `2e18419` | PASS | 7 passed, 1 skipped (real-SEC test) |
| `TS_E2E_REAL_SEC=1 ./scripts/test-e2e.sh`, same commits | PASS | 7 passed, 1 skipped (unconfigured test) |
| `./scripts/research-smoke.sh` (AAPL, 0000320193), same commits | SUCCEEDED in 1 attempt; both downloads match | 5 annual periods |

Earlier in the session, the full integration script passed six times in a row after the `as_of` fix.

## Documented real SEC fetch

Through the real API on a disposable database, 2026-10-23. Two requests to `data.sec.gov`, sent with the owner-approved User-Agent.

| Source | Size | SHA-256 |
|---|---|---|
| `https://data.sec.gov/submissions/CIK0000320193.json` | 163,917 bytes | `47e4383b0133a4146cc631725f9aa28bac6d5679d504f735a78f1a1923c4b622` |
| `https://data.sec.gov/api/xbrl/companyfacts/CIK0000320193.json` | 3,789,099 bytes | `73a86c6aedc31f77cac2ea4df5f80f0b3bd7e6eb58bb4e01444fbedf3afb9c43` |

The same bytes came back on every run today, including Codex's independent download.

| Fiscal year end | Original 10-K | SEC acceptance (UTC) | Revenue (USD) | Net income (USD) | Assets (USD) | Checks |
|---|---|---|---|---|---|---|
| 2025-09-27 | 0000320193-25-000079 | 2025-10-31 14:01:26 | 416,161,000,000 | 112,010,000,000 | 359,241,000,000 | both PASS |
| 2024-09-28 | 0000320193-24-000123 | 2024-11-01 14:01:36 | 391,035,000,000 | 93,736,000,000 | 364,980,000,000 | both PASS |
| 2023-09-30 | 0000320193-23-000106 | 2023-11-03 02:08:27 | 383,285,000,000 | 96,995,000,000 | 352,583,000,000 | both PASS |
| 2022-09-24 | 0000320193-22-000108 | 2022-10-28 02:01:14 | 394,328,000,000 | 99,803,000,000 | 352,755,000,000 | both PASS |
| 2021-09-25 | 0000320193-21-000105 | 2021-10-29 02:04:28 | 365,817,000,000 | 94,680,000,000 | 351,002,000,000 | both PASS |

No metric was missing or withheld, no fact was excluded, and no later filing reported a different value. The report stayed `INSUFFICIENT_DATA` with entry and backtest eligibility false, and warned that notes, auditor opinions, quarters and licensed prices are not analyzed.

## Defects found and fixed (each with a test that fails on the old code)

| Defect | Found by | Fix |
|---|---|---|
| Submissions CIK `"0000320193"` failed `json.Number` decoding; every real job failed | Codex reviewer | `secCIK` accepts the string and number forms |
| Same-context conflicting values gave order-dependent ratios | Codex reviewer | conflicts withheld; identical duplicates merged |
| Periods ending after their filing date were accepted | Codex reviewer | rejected and counted |
| Older years were taken from later filings' comparatives, losing balance sheets | Claude, on real data | each year anchored on its original 10-K |
| Interrupted last attempt re-queued, then broke the attempts limit | Codex reviewer | fails visibly; queued exhausted jobs also fail |
| 403/429 cooldown applied to one job only | Codex reviewer | durable SEC-wide cooldown |
| Embedded `bytes.Buffer` let `io.Copy` bypass the 1 MiB report limit | Codex reviewer | named buffer field plus explicit size gate |
| An SEC limit seen during shutdown was dropped | Codex reviewer | recorded with a short context that outlives cancellation |
| `RFC3339Nano` `as_of` (for example `.74672`) rejected by Python 3.9 at random | Claude, by repeating runs | fixed six fractional digits; parser normalizes any length |
| A late response for an earlier job replaced the selected job in the UI | Codex reviewer | sequence-numbered refresh; heading recorder test fails on the old draft |
| E2E and smoke runners could treat a stale server on 18080 as ready and send it authenticated requests | Codex reviewer | port preflight, a fresh token per run, readiness only from our own live child that owns the port |
| Docs said SEC publishes no recovery time; SEC states access may resume after 10 minutes below its threshold | Codex reviewer | wording corrected after reading the page; the 10-minute pause is described as conservative, with no guaranteed unblock |

## Negative, concurrency and security cases exercised

Missing or wrong token (401), wrong origin (403), malformed bodies, unknown fields, oversized bodies, numeric CIKs, zero CIK, key reuse with another body, the queue limit, SEC 403/404/429/5xx, network failure, redirects, oversized and invalid JSON, identity and ticker mismatches, parser crash/hang/oversized/forged-eligibility output, a stderr flood, secrets in the parent environment, append-only enforcement, a non-leader worker, stale attempts and altered source bytes in the browser.

## Real data versus fixtures

Real: the SEC runs above. Fixtures: all automated tests use synthetic SEC-shaped payloads, marked as test data in the test files. No fixture is presented as a real filing.

## Unrun tests and limitations

- Not covered: quarterly reports, notes, auditor opinions, segments, custom taxonomies, EGX issuers and any prices.
- companyfacts is a current snapshot, so historical backtest certification is impossible by design.
- SEC's older supplemental submissions pages are not fetched. Facts from those older filings are still analyzed from companyfacts, with an estimated availability time and report date, flagged in the report. SEC acceptance time is an availability proxy, not proof of first public availability.
- Only one issuer (Apple) was tested against real SEC data. Financial issuers (SIC 6xxx) are covered by synthetic tests only.
- No load test was run; throughput is deliberately one job at a time.
- The normal paper database was not migrated or touched. It still has only migration 1, and `paper_workspace` was last updated 2026-10-22 23:55:31 UTC. Migration 002 applies on the next normal app start.

## Contract, schema and docs changes

`contracts/research-api.md`; migration `db/migrations/002_research.sql` (research tables, append-only triggers, provider cooldown); [ADR-018](../adrs/ADR-018.md) (proposed); [provider record](provider-sec-edgar.md); backend and frontend READMEs; `scripts/research-smoke.sh`; `scripts/test-e2e.sh` SEC switch.

## Migration and rollback

Migration 002 only adds research tables and triggers, and does not touch `paper_workspace`. Rollback means stopping research and dropping the five research objects (`research_jobs`, `research_sources`, `research_events`, `research_provider_state`, `research_append_only`); paper state is unaffected.

## Residual risks and follow-ups

- SEC does not document which HTTP status signals a rate limit; treating 403/429 as one is our classification. The 10-minute pause for every job follows SEC's statement that access may resume after 10 minutes below its threshold, but cannot guarantee an unblock.
- Commercial or LLM use of SEC data needs the owner's D-08 review. This is not legal advice.
- S00-T03 (licensed US and EGX price vendors, D-01/D-02) is untouched by this work and stays blocked.

## Final reviewer statement

Pending: the full task review (frontend, contract and evidence) is in progress by Codex. This record is not a self-review.

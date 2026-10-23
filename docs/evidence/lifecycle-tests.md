# Task evidence — S01-T05

Add observability and a safe worker lifecycle.

| Field | Value |
|---|---|
| Task / owner / reviewer | `S01-T05` / `claude-code-agent` (session `2026-10-23-development-8da61cee`) / **independent review pending** |
| Base | Backend `29a06e6b24024da68eb5465ed7016a1d788a1947` (state 70) |
| Code | **PENDING:** exact commit to be recorded after the remaining cleared runs |
| Environment | macOS 27.0.1 arm64; Go 1.26.10; CPython 3.14.8; nats.go v1.54.0; nats-server v2.15.1 (in-process, tests only); PostgreSQL 16.15 (`postgres:16.15-alpine@sha256:f4610795…`, a new container per run) |
| Hosted CI | **Not run.** Nothing was pushed. |

Every result here is an **author run**. The independent reviewer's runs are in its own reports and are not claimed here.

Deliverable paths: the catalog's `research/src/telemetry/` is `research/src/tradestocker_research/telemetry.py`, inside the accepted single-package layout (one installable package, checked by `research/tests/test_boundaries.py`).

## What exists now

| Part | What it does |
|---|---|
| `internal/telemetry` (standard library only) | W3C `traceparent` parse/continue/inject; HTTP middleware (continues or starts the trace, returns `traceparent` and `X-Trace-Id`, echoes a valid `X-Request-Id`, one log line per request without query, headers or body); a JSON `slog` handler that adds `trace_id`/`span_id` and cleans every value: registered secrets and credential shapes are replaced, secret-named keys are `[REDACTED]`, content-named keys (body, document, source, payload, ...) and the `Licensed` marker show only a size, also inside typed values, lists, structs and groups opened with `WithGroup`. |
| `internal/natsbus` | Core NATS notifications on `tradestocker.research.job.created`: the job ID and the `traceparent` header. Sent after the job is committed and only as a hint (workers also poll PostgreSQL); a failed publish is logged and never fails the request; nothing is buffered while disconnected. It is not an outbox. URLs with credentials, paths or queries are refused without repeating the value. |
| `internal/httpapi` | `GET /api/v1/live` (checks nothing) and `GET /api/v1/ready` (`ready`, `degraded`, `not_ready` 503, `draining` 503; each check only `ok`/`unavailable`/`not_configured`), both without the token. Every route runs inside the trace middleware. |
| `cmd/platform-core` | Redacting JSON logger with the token, database URL and password and SEC contact registered as secrets; optional `TS_NATS_URL`; `TS_RESEARCH_WORKERS` (1–4); readiness checks (PostgreSQL ping, required; NATS connection, optional); graceful stop in order: readiness `draining`, HTTP shutdown, research workers settle or release, NATS drain, database close. |
| `internal/research` | Jobs keep the creating request's trace (`trace_id`, migration 003). A bounded worker pool (1–4) under the existing leader lock. A durable retry budget (6 retry starts per 10 minutes by default) admitted at claim time from the `ATTEMPT_STARTED` events, with one post-lock clock for admission, lease and events. A known outcome (valid report or analysis failure) is settled with its own bounded deadline even during shutdown, with a fenced release as fallback. Parser stderr is logged by size only; output-gate errors never quote the report. |
| `research/parser_launcher.py`, `tradestocker_research/telemetry.py` | The parser path Go runs: the launcher loads the unchanged `financials.py` (SHA-256 `567065a7…`) and `telemetry.py` from the embedded sources, calls `main()` (stdout stays the parser's exact report) and writes one `research parser ended` event in the job's trace. Go relays it only if it has exactly the event's fields, the job's trace, a non-zero span and this parser's version. The `research-worker` CLI accepts an optional `traceparent` per request and logs one safe line per request. |
| `scripts/test-isolated.sh` | PostgreSQL tests on a new labelled container (tmpfs data, random loopback port, pinned image), packages one at a time; stops only that container after checking its ID and run label. |
| `deploy/compose.yaml` | Dev stack platform-core gets `TS_NATS_URL=nats://dev-nats:4222`. |

## Acceptance criteria

| Criterion | Outcome (author run) | Evidence |
|---|---|---|
| Trace and correlation IDs survive HTTP and NATS boundaries | **Met in the 5-package isolated run, except one timing assertion in the same test (fixed; rerun PENDING).** One request trace reached the HTTP log line, the stored job, the NATS-received notification, the worker attempt lines and the parser event. | `TestTraceCrossesHTTPAndNATSAndOutagesChangeReadiness`, `TestJobContinuesTheRequestTrace`, `TestNotificationCarriesTheTrace`, `TestLauncherKeepsTheReportBytesAndLogsInTheJobTrace` |
| Liveness differs from readiness; a dependency outage changes readiness without leaking secrets | **Met in the same run (rerun PENDING).** NATS outage: `degraded`, HTTP 200, a new job still finished by polling. Database outage (a test relay cut): 503 `not_ready`, liveness 200, then `ready` again. Bodies hold fixed words only; no secret in any log line. | Same e2e test; `TestLivenessAndReadiness` |
| Logs exclude bot tokens, account secrets and full licensed document content | **Met.** Registered secrets, credential shapes, secret-named and content-named keys, `Licensed` values (also inside lists/structs/maps, `WithGroup` scopes and `fmt`), parser stderr and output-gate errors. | `internal/telemetry` tests (11), independent probes 8/8 at `log.go` `044e8532…`; `TestParserFailureLogsNoOutputContent`, `TestRejectedParserInputLogsOnlyTheOutcome`, `TestOnlyTheLaunchersEventIsRelayed` |
| Cancellation drains or requeues work and never loses a financial transaction | **Research part met; the process kill/restart and the financial-transaction kill are PENDING static clearance.** | `TestWorkerPoolBoundsConcurrencyAndDrainsOnStop`, `TestStopAfterAValidReportStillSettles`, `TestShutdownReleasesAttemptAndRestartFinishes`; kill harness below |
| Validation: kill and restart a worker during a synthetic job | **PENDING** (harness written, not run). | `cmd/platform-core/kill_test.go` (build tag `t05kill`) |
| Validation: bounded concurrency with a slow-provider fixture | **Met.** With 2 workers and a 300 ms provider, at most 2 fetches ran at once over 6 jobs; SEC requests from 3 concurrent callers started at least the minimum interval apart, one at a time. | `TestWorkerPoolBoundsConcurrencyAndDrainsOnStop`, `TestSECSpacingHoldsAcrossConcurrentWorkers` |

## Runs

| When (+03) | Command | Result | Resource |
|---|---|---|---|
| 16:1x | `scripts/test-integration.sh` (reviewed script, normal instance, own `tradestocker_test_*` database) | store + research all PASS | Database dropped by the script. Afterwards Codex asked for a separate instance for T05. |
| 16:24 | `scripts/test-isolated.sh ./internal/research` | 34 PASS, 0 FAIL, 0 SKIP | `tradestocker-t05-test-pg-910fd1e382f8`, stopped and removed |
| 16:3x | `scripts/test-isolated.sh ./internal/store ./internal/research ./internal/natsbus ./internal/httpapi ./cmd/platform-core` | 54 PASS, 1 FAIL, 0 SKIP. The failure: the test restored the database before the worker's 1 s session check had logged the outage. Test fixed to wait for that log line (`lifecycle_test.go` `db21b7d8…`). | `tradestocker-t05-test-pg-70cca5351044`, stopped and removed |
| — | Python: `.tools/research-venv/bin/python -I -B -m unittest discover -s research/tests -t research/tests`; ruff; mypy | 43 PASS; ruff and mypy clean (8 files) | none |
| — | `go vet ./...`; `go test ./tests/architecture`; govulncheck v1.9.0 | clean; PASS; 0 reachable vulnerabilities (1 module-level: GO-2026-5932, `x/crypto/openpgp`, no fix, not imported) | none |

## Review findings fixed during the task

022 typed values and content groups; 023–025 `WithGroup` scopes and nested `Licensed`; 026 retry budget at real admission, parser stderr, settlement after a valid report; 027 one post-lock clock; 028 pool-test cleanup; 030 relay re-check, repeatable completion, actual Python parser telemetry; 031 parser event task and span checks.

## Limits

- NATS is a hint only; PostgreSQL holds every job. No JetStream, outbox or exactly-once delivery is claimed.
- Only the `Licensed` type is protected inside structs; a new marker type would need its own `MarshalJSON`.
- The retry budget counts retry starts, not provider requests; SEC spacing (1 request per second) and the SEC cooldown still apply.
- The browser E2E against the real API was not rerun in T05 (it uses the normal database instance and `bin/`).
- No real SEC request, Telegram message, broker call, push or deployment was made.

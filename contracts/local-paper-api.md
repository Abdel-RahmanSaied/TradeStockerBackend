# Local paper API v1

The machine-checked contract is `contracts/openapi/local-api.v1.json` (see `contracts/README.md`); this page explains it.

Implemented contract for S00-T07/T08. It is separate from proposed production contracts. Go is the only authority. Base: `http://127.0.0.1:8080/api/v1`. The dashboard uses a same-origin Vite proxy. All live lanes are disabled. No broker integration or actual/live mutation exists.

Auth: `Authorization: Bearer <TS_LOCAL_DEV_TOKEN>` for every endpoint except health, live and ready. Token is generated locally, ignored by Git and held only in frontend memory. This is a single-owner dev stand-in, not D-05 production identity. JSON-only POSTs require an 8–128 character `Idempotency-Key` (letters, digits, dot, dash, underscore, colon). Same key and exact request bytes return the committed response; a changed body/path yields 409. Use the same key when retrying an ambiguous write.

Tracing: every response carries `traceparent` and `X-Trace-Id`. A valid incoming W3C `traceparent` is continued (new span); otherwise a new trace starts. A valid `X-Request-Id` (1–64 letters, digits, dot, dash, underscore) is echoed. Logs are JSON lines with the trace ID; they never contain headers, bodies, query strings or secrets.

Money/prices: signed JSON integers in **minor currency units**, bounded on input; quantities are positive whole-share integers. No floating-point financial arithmetic in Go. USD/EGP have separate paper accounts and no conversion. Time is RFC3339 UTC. Financial assessments in the fixture pack are invented test evidence, not real research. Real provider or assessment uploads are unsupported. Research stub does not authorize entries.

| Method/path | Input / response |
|---|---|
| GET `/health` | Database-backed health, PAPER, live false; 503 on storage failure |
| GET `/live` | `{"status":"alive"}` while the process answers; checks no dependency |
| GET `/ready` | `status` `ready`, `degraded` (an optional dependency such as NATS is down), `not_ready` (503: PostgreSQL down) or `draining` (503: stopping); `checks` maps each dependency to `ok`, `unavailable` or `not_configured`, never error text |
| GET `/dashboard` | mode, live_advisory_enabled, lanes, accounts, quotes, recommendations, positions, audit, notifications, journal, entries_paused, build; private idempotency receipts omitted |
| GET `/risk/policy` | Versioned PAPER-only test policy, approval DEV_PLACEHOLDER_NOT_APPROVED |
| POST `/fixtures` | `{}` → generated synthetic observations with current generation timestamps; includes explicitly stale and missing-evidence cases |
| POST `/paper/accounts` | `{currency:"USD"|"EGP", amount_minor:int}`; one account per currency; creates paper funds and journal event |
| POST `/paper/recommendations` | `{account_id,symbol,lane,quantity,entry_minor,stop_minor}` → decision; rejection by risk/evidence is recorded as NO_TRADE or INSUFFICIENT_DATA |
| POST `/paper/recommendations/{id}/acknowledge` | `{}`; PROPOSED → ACKNOWLEDGED, no fill |
| POST `/paper/recommendations/{id}/cancel` | `{}`; open → CANCELLED; releases only reserved paper funds |
| POST `/paper/recommendations/{id}/fill` | `{price_minor,confirmation:"PAPER_ONLY"}`; acknowledged → FILLED; full paper fill only, no partial fill implementation |
| POST `/paper/accounts/{id}/sell` | `{symbol,quantity,price_minor,confirmation:"PAPER_ONLY"}`; human-recorded paper reduction, cannot exceed holdings |
| POST `/paper/entries` | `{paused:boolean}`; pause entries, preserve monitoring/positions |
| POST `/research/assessments` | `{symbol}` → STUB / INSUFFICIENT_DATA, no model/provider calls or financial-state writes |

All POSTs return 200 JSON on success. Errors use `{error:{code,message}}`: 401 unauthorized; 403 origin denied; 404 absent entity; 409 lifecycle/idempotency/stale-fill/cash conflict; 413 size limit; 415 content type; 422 invalid inputs; 503 storage failure. Unknown input fields are rejected. Exact routes/types are in Go handlers and frontend model.ts.

Paper lifecycle: PROPOSED → ACKNOWLEDGED → FILLED, or CANCELLED/EXPIRED. Rejected candidates cannot be acknowledged or filled. Reservations expire after 15 minutes, processed by API reads/writes. There is no background scheduling promise. A stale quote blocks entry and fill; expiry does not close positions. One database transaction commits financial state, journal, audit, local notification preview and idempotency receipt.

Local test policy: 1% per-trade planned stop risk, 3% aggregate planned open risk and 20% single-symbol cost/reservation exposure. These values are engineering test fixtures only, not owner-approved investment limits. No risk increase based on goals or losses; no goals/returns calculated. Intraday candidates are blocked pending eligibility validation. Fees/slippage/corporate actions/settlement are not modeled. Manual paper sells are journal observations with caller-reported prices, not market executions. No price is live.

Storage uses a locked PostgreSQL JSON aggregate for a private bounded prototype. It intentionally serializes writes. GET requests (including health) read the latest committed document without the row lock and never rewrite it; if a reservation is due to expire, the expiry, refund, journal, audit and notice are committed first under the lock, and only when that latest state still has expiry work. Production relational schema, NATS/outbox delivery, retention/archiving, multi-account identity, immutable external audit, HA/backups and governance remain original roadmap work. Telegram previews never make network calls, even if a token is present.

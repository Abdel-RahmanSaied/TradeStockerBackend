# Research API v1 — SEC annual filings

Implemented contract for S07-T07. Base: `http://127.0.0.1:8080/api/v1`. Same auth as `local-paper-api.md`: every endpoint needs `Authorization: Bearer <TS_LOCAL_DEV_TOKEN>`, and only the local dashboard origin is allowed. Research is **read-only**: it never reads or writes paper financial state, and no report makes an instrument eligible for entry or backtesting.

The backend contacts only `https://data.sec.gov`, for two documents per attempt. It needs `TS_SEC_USER_AGENT` in the form `Application Name contact@example.com` (SEC asks automated tools to declare a user agent). Without it, status reports `configured: false` and new jobs are refused.

## Endpoints

| Method / path | Purpose |
|---|---|
| GET `/research/status` | `{configured, source:"SEC_PUBLIC", mode:"READ_ONLY_RESEARCH", parser_version, max_attempts, provider_cooldown_until, live_advisory_enabled:false}`. `provider_cooldown_until` is set while every job is paused after an SEC access limit. |
| POST `/research/jobs` | Body exactly `{"symbol":"AAPL","cik":"0000320193"}`. `Idempotency-Key` header required (8–128 of `A-Z a-z 0-9 . _ : -`). Returns **202** with the job. |
| GET `/research/jobs` | `{jobs:[…]}`: newest 50 jobs, without `result`. |
| GET `/research/jobs/{id}` | `{job, sources, events}`. `job.result` holds the report once `SUCCEEDED`. |
| GET `/research/jobs/{id}/sources/{kind}` | Exact original bytes (`kind` is `submissions` or `companyfacts`), latest attempt. With `?sha256=<64 lowercase hex>`, only a matching copy is returned. Sent as an attachment. |

Idempotency: the same key with the same body returns the same job in its **current** state (it may have moved on). The same key with a different body is `409 IDEMPOTENCY_CONFLICT`. Replays still answer when the SEC contact is not configured.

## Job

`{id, symbol, cik, status, attempts, max_attempts, created_at, updated_at, next_attempt_at?, error_code?, last_error?, result?, trace_id?}`

- `trace_id`: the W3C trace ID of the request that created the job (also in the `REQUESTED` event). The worker's log lines for the job use it. A replay of the same `Idempotency-Key` returns the job without changing it.
- `status`: `QUEUED` → `RUNNING` → `SUCCEEDED` or `FAILED`. A retry goes back to `QUEUED` with `next_attempt_at` and `last_error`.
- At most **3 attempts**. Retryable failures wait 30 s, then 2 min. `SEC_ACCESS_LIMITED` (HTTP 403/429, our classification) waits 10 min **and pauses all jobs** for 10 min, a conservative use of SEC's statement that access may resume after 10 minutes below its threshold; it does not guarantee an unblock. A crashed attempt is recovered after its 150 s lease. A graceful shutdown re-queues the attempt at once; an interrupted last attempt fails as `WORKER_ATTEMPTS_EXHAUSTED`. A shared retry budget lets at most 6 retries start in any 10 minutes across all jobs, counted from the stored `ATTEMPT_STARTED` events, so it holds across restarts; a held retry waits in the queue and is never dropped. Any attempt after a job's first counts, including recovered and re-queued ones. A stop that arrives after a valid report still records it.
- `sources[]`: `{attempt, kind, sha256, url, received_at, size_bytes}`. Originals are stored before analysis and kept even if analysis fails.
- `events[]`: `{at, action, detail}`. Actions: `REQUESTED`, `ATTEMPT_STARTED`, `LEASE_RECOVERED`, `SEC_SOURCES_STORED`, `ATTEMPT_FAILED_RETRY_SCHEDULED`, `PROVIDER_COOLDOWN_STARTED`, `ATTEMPT_INTERRUPTED`, `WORKER_ATTEMPTS_EXHAUSTED`, `SUCCEEDED`, `FAILED`.

Job error codes: permanent — `INVALID_ISSUER`, `SEC_CONTACT_NOT_CONFIGURED`, `SEC_NOT_FOUND`, `SEC_SYMBOL_CIK_MISMATCH`, `SEC_IDENTITY_MISMATCH`, `SEC_UNEXPECTED_SHAPE`, `SEC_INVALID_JSON`, `SEC_RESPONSE_TOO_LARGE` (16 MiB per source), `SEC_REDIRECT_REFUSED`, `RESEARCH_ANALYSIS_FAILED`, `RESEARCH_PROVIDER_FAILED`, `WORKER_ATTEMPTS_EXHAUSTED`; retryable — `SEC_ACCESS_LIMITED`, `SEC_HTTP_ERROR` (5xx), `SEC_REQUEST_FAILED`, `SEC_REQUEST_TIMEOUT`, `SEC_BODY_FAILED`. `last_error` may also be `WORKER_STOPPED`.

## Report (`parser_version: "sec-annual-v2"`)

Top level: `schema_version, version, symbol, cik, issuer, source, data_kind:"OFFICIAL_FILINGS", status ("PARTIAL" | "INSUFFICIENT_DATA"), outcome:"INSUFFICIENT_DATA", eligible_for_entry:false, eligible_for_backtest:false, financial_state_write_access:false, as_of, accounting_framework, scope, currency:"USD", periods[], warnings[], excluded_facts{reason:count}, countercase[], coverage, sources[{kind, sha256}]`.

Each of up to five `periods[]` comes from that fiscal year's **original annual report** (the 10-K whose report date equals the period end; a 10-K/A is used only if no 10-K exists):

- `fiscal_period_end`, `start`, `filing{accession, form, filed, report_date, report_date_basis, accepted_at, available_at, availability_basis, filing_url}`.
- `facts[]`: `{metric, value, unit:"USD", start, end, filed, available_at, availability_basis, accession, form, tag, source_sha256, pointer, filing_url}`. `value` is a whole-dollar **decimal string**; `pointer` is a JSON pointer into the stored companyfacts source.
- Metrics: `revenue`, `net_income`, `operating_cash_flow`, `assets`, `liabilities`, `equity`, `liabilities_and_equity`.
- `derived`: `net_margin`, `cash_conversion`, `liabilities_to_assets` as `{value (6-decimal string or null), inputs, status}` with status `DERIVED`, `MISSING_INPUT` or `UNDEFINED_NONPOSITIVE_DENOMINATOR`. Withheld for financial issuers (SIC 6xxx).
- `checks`: `BALANCE_TOTALS` (assets vs liabilities-and-equity: `PASS`, `FAIL`, `MISSING_CONTEXT`) and `LIABILITIES_PLUS_EQUITY` (`PASS`, `UNEXPLAINED_DIFFERENCE`, `MISSING_CONTEXT`), with `difference_usd`.
- `missing_metrics[]`: metrics with no usable value for that report, whether absent from the filing, excluded (see `excluded_facts`) or withheld. A withheld metric appears in both lists. `withheld_metrics[]`: `{metric, tag, reason, values}` where reason is `CONFLICTING_VALUES`, `AMBIGUOUS_DURATION` or `MISMATCHED_FISCAL_YEAR`. Missing or withheld is never shown as zero.
- `later_filing_differences[]`: `{metric, tag, original_value, later_value, later_accession, later_form, later_filed}` — counter-evidence when a later filing reports a different value.

Timing rules: a fact is used only if SEC made it available by `as_of`. Availability is approximated by the SEC acceptance time (never earlier than the filing date); acceptance is not proof of first public availability, which can come later. Without an indexed acceptance time it is estimated as the next UTC day (`FILING_DATE_PLUS_ONE_DAY_ESTIMATE`, warning `FILING_TIME_ESTIMATED`). A fact whose period ends after its filing date is excluded (`PERIOD_ENDS_AFTER_FILING`).

## Errors

`{"error":{"code","message"}}`: 401 token, 403 origin, 404 `RESEARCH_NOT_FOUND`/`SOURCE_NOT_FOUND`, 409 `IDEMPOTENCY_CONFLICT`, 413 `BODY_LIMIT` (4 KiB), 415 `JSON_REQUIRED`, 422 `IDEMPOTENCY_KEY_REQUIRED`/`US_SYMBOL_AND_TEN_DIGIT_CIK_REQUIRED`/`INVALID_SOURCE_HASH`, 429 `RESEARCH_QUEUE_LIMIT` (20 active or 10,000 stored jobs), 503 `SEC_CONTACT_NOT_CONFIGURED`/`RESEARCH_STORAGE_UNAVAILABLE`.

## Not covered

Quarterly reports, notes, auditor opinions, segment and custom-taxonomy facts, SEC's older supplemental submissions pages (not fetched; facts from those older filings are still analyzed from companyfacts, with an estimated availability time and report date), EGX issuers, prices of any kind, and point-in-time archives (companyfacts is today's snapshot). These are reported as warnings, not filled in.

# Independent paper-read review — S00-T09

Reviewer: `independent-review-agent`, separate from implementation owner `claude-code-agent`. Original review date: 2026-10-23. Reconstructed after the backend deletion incident on the same date.

**Recovery notice.** This is a new, faithful reconstruction from the review's recorded commands, outputs and findings. It is not claimed to be a byte-identical recovery of the lost report. The original backend Git objects were deleted; this report does not recreate them or invent a review commit. The exact source snapshot independently tested before the incident survives in the reviewer archive for `55d98746e24dca141b2985aa73d5b65ff60f96b7`. A hash-verified copy and manifest are preserved beside this report. Any restoration or subsequent S01 implementation needs its own exact-tree comparison and review.

## Decision and exact checkpoints

**S00-T09 was independently accepted at backend `55d98746e24dca141b2985aa73d5b65ff60f96b7` and frontend `9c75c9183df2ed9457012260bb904a9ea404a960`.** The accepted source content and observed pre-incident tests support that bounded historical decision. They do not establish that the deleted Git commit object or later local commits have been restored.

Initial backend checkpoint: `1aec76cd3678a4c336a50e34caf2583c46321afe`, containing implementation `dcbfd2658348da10a9b0e545b14173be7bdc85f8` plus evidence. The frontend change only made the Playwright port configurable. Tests used complete archived snapshots and external reviewer overlays, not the changing development tree.

The initial checkpoint required a correction for implicit HEAD requests using the write path. The GET optimization, locked expiry recheck, financial regressions and isolated browser workflow otherwise passed. Both the HEAD and ADR wording findings were resolved in the final checkpoint.

## Findings and resolution

1. **P2 — HEAD retrieval rewrote financial state at 1aec76c.** `internal/httpapi/server.go:199` selected `Store.Read` only for GET. Go's GET route patterns also accept HEAD, so those requests selected `Execute`. An independent real-PostgreSQL HTTP regression sent unchanged HEAD requests to health, dashboard and risk policy. All returned 200, but a private trigger counted three financial UPDATEs and the row fingerprint changed. This failed the unchanged-read requirement. **Resolved in 55d98746:** only POST selects Execute; GET and implicit HEAD select Read.
2. **ADR-017 wording was stale.** It said every read/write serialized. **Resolved in 55d98746:** it now describes serializing writes, committed-snapshot reads, and the locked/rechecked due-expiry fallback. This did not expand the architecture's approved scope.

## Independent initial checks

External probes survive under `work/reviewer-research/readfix/`. `run_review.py FULL_COMMIT_SHA` archived the supplied commit, injected `review_readfix_test.go` through Go's overlay option, ran with the race detector, and created/removed only a uniquely named `tradestocker_test_review_readfix_*` database. The handler tests did not bind shared API ports.

| Check on initial backend 1aec76c / frontend 9c75c91 | Observed result |
|---|---|
| GET health/dashboard/policy while another transaction holds the financial row lock | PASS: all returned 200 within the bounded request time; zero UPDATEs; unchanged document hash, xmin and updated_at |
| Four expired-snapshot readers behind a held row lock, observed as four blocked database sessions before release | PASS: exactly one UPDATE, one refund, one expiry journal entry and one expiry audit; expiry at exact timestamp equality; later reads did not rewrite |
| Concurrent GETs and same-key EGP account creation, conflict, rollback, unauthorized due-expiry read and receipt privacy | PASS: one additional account; conflict refused; failed mutation and unauthorized read left the row unchanged; authorized read persisted expiry; private receipts absent from the dashboard |
| HEAD health/dashboard/policy | FAIL: three unnecessary UPDATEs; the concrete finding above |
| `go vet ./...` | PASS |
| `go test -race -count=1 ./...` without database configuration | PASS for every tested package; database tests skipped as designed |
| `go test -race -count=1 -v ./internal/store` against real private PostgreSQL | PASS, two top-level tests: includes migration replay, reconnect/receipts, 24 concurrent reservations, 16 competing fills, rollback, 400 unchanged reads, 12 queued expiry readers and expiry racing writes/replays |
| `go test -race -count=1 -v ./internal/research` against another real private PostgreSQL database | PASS, 23 top-level tests: durable jobs, retained originals/reports, recovery, cooldown and output gates; mocked provider, no new SEC request |
| Actual API/browser E2E with `TS_E2E_API_PORT=18090 TS_E2E_FRONTEND_PORT=15173` | PASS, seven passed and one intentional real-SEC skip: four controlled timing/rendering tests, disabled research without contact, paper lifecycle, mobile/access behavior |

Store and research integration suites used different disposable databases and ran sequentially, avoiding interference from cross-package state resets. E2E used exact backend/frontend archives, a fresh token and a new disposable database. Existing installed frontend dependencies were linked into reviewer scratch. No normal application checkout was edited by these checks.

Source review confirmed that ordinary reads load and validate one committed PostgreSQL document without a row lock or an in-memory cache. When expiry is due, expireLocked loads the latest row FOR UPDATE, rechecks time/due status, validates and persists the effect only when needed, then commits. Stale contenders return the freshly locked state without redundant UPDATEs. The mutation implementation was unchanged. Read handlers return state or constant policy/health data without mutation.

The startup origin override uses an anchored loopback HTTP origin pattern with an explicit port. API origin matching remains exact; a configured origin replaces defaults. Protected no-origin requests still need the owner token. There is no wildcard or credentialed CORS response. Origin/authentication, malformed-body and absent-broker tests passed. The frontend port change retained strict binding.

## Independently reproduced fixed-history measurement

The external benchmark helper built before `9b3afb0ea62aca0555fc3aec8f19e070ad73b098` and after `1aec76cd3678a4c336a50e34caf2583c46321afe`. It seeded a separate fresh database for each through the real API, then ran the checkpoint's paperbench client. Both histories contained 400 evaluations, 400 recommendations and 402 audit events, with roughly a 1.13 MB document and 682 KB dashboard response.

Each phase ran ten seconds, with eight dashboard readers and one health poller, on reviewer API port 18090. Tokens were random, SEC disabled, and databases private on PostgreSQL 16.15. This differed from the author's dedicated-container environment; these are the independent run's actual values, not substituted author ratios. The exact result JSON survives beside this reconstructed report as `benchmark-review-results.json`.

| Measure | Before | After |
|---|---:|---:|
| Dashboard requests / errors | 344 / 0 | 1,601 / 0 |
| Dashboard requests/s | 33.6 | 159.4 |
| Dashboard p50 / p95 / p99, ms | 228.396 / 280.017 / 337.802 | 46.964 / 76.260 / 99.714 |
| Health requests / errors | 43 / 0 | 209 / 0 |
| SQL UPDATEs counted by trigger | 387 | 0 |
| Row version / updated_at changed | yes / yes | no / no |
| Table bytes before → after reads | 30,310,400 → 99,246,080 | 36,462,592 → 36,470,784 |

This confirms removed financial rewrites and a local read improvement. The after-table increase of 8 KB means zero financial UPDATEs must not be described as a guarantee of zero physical database-file growth. These results do not establish production capacity, a production SLO or returns. The author's two other benchmark runs were inspected as author evidence, not claimed as independent executions.

## Final exact-checkpoint re-review

The correction at `55d98746e24dca141b2985aa73d5b65ff60f96b7` changed the HTTP dispatch guard, added handler/database regressions, and corrected ADR-017 and task evidence. Unsupported PUT/DELETE/PATCH/OPTIONS are rejected by the router without reaching the store.

From a fresh complete archive of that final checkpoint:

- `python3 run_review.py 55d98746e24dca141b2985aa73d5b65ff60f96b7`: PASS, all four independent real-PG overlay regressions with the race detector. The identical HEAD probe that failed initially now recorded zero UPDATEs and unchanged fingerprint. Locked-row reads, four stale expiry contenders and concurrent receipt/conflict/rollback/auth/privacy checks passed again.
- `go test -race -count=1 -v ./internal/httpapi ./cmd/platform-core`: PASS. Includes GET/HEAD/POST dispatch, unsupported methods, configured-origin replacement, negative origin/startup cases, auth and request validation.
- `go test -race -count=1 -v ./internal/store` against another fresh private review database: PASS, three top-level tests. The new real-handler check proved 60 GET/HEAD requests and three refused methods caused zero UPDATEs, while one POST caused exactly one, alongside the existing financial/expiry regressions.

The correction did not change GET/POST business behavior, browser configuration, research, dependency pins or benchmark code. The completed full race/vet suite, research real-PG suite, browser E2E and benchmark therefore remained relevant. They were not rerun or claimed as new executions on 55d98746. The final targeted tests directly exercised the changed dispatch.

All four bounded S00-T09 criteria were met: unchanged retrievals avoid financial writes; due expiry is durable and occurs once with stale contenders and competing writes; financial/auth/idempotency behavior remains guarded; and a fixed-history comparison plus isolated API/browser workflow supports the local performance result.

## Preservation and limits

Final pre-incident reviewer checks found zero remaining reviewer read-fix databases and no listeners on 18090/15173. The normal API remained PAPER/PostgreSQL/live=false and frontend HTTP 200. Normal paper document MD5 stayed `51285359c2d98893a4d47ead4c2fb4da`. Normal SEC storage stayed one job/two originals, with job-row digest `3c0d1aec0b92becfac3b1148ebaf1b54`. Planning validation and generated-document checks passed. These are observed pre-incident results, not a new claim that incident recovery is complete.

Writes and idempotent write replays still lock and rewrite the aggregate; history, receipts and decoding cost grow. Reads reflect a committed snapshot, not later concurrent commits. This change does not implement the relational ledger, production identity/hosting, historical qualification, financial suitability gates or a real entry recommendation. No new SEC fetch, external model, paid resource, broker action, Telegram send, live activation or risk-policy change occurred during the review.

The original reviewer changed only the review evidence file in the repositories. During this reconstruction only new external recovery files were written. The original backend, running processes, normal database and shared staging were not modified by the reviewer. Product restoration and execution-state decisions remain with the coordinator and author.

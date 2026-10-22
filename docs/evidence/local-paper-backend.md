# Local paper backend evidence - S00-T07

Date: 2026-10-23. Implementer/task owner: codex-agent, session 2026-10-23-codex-increment. Independent reviewer: pending. This is not production approval.

## Ownership and authorization

Human Work request explicitly authorized local backend/frontend edits, tests, app runs and protocol-compliant local commits. Existing Claude session 8da61cee-0de9-4193-af9a-cb914502dcee was recovered after a VS Code extension restart. Its CODEX-BRIDGE-RESPONSE refused implementation/review without a direct owner line inside its session. It confirmed zero commands/edits, acknowledged Codex backend ownership and agreed not to start conflicting work. Reassignment is recorded at state revision 12. No replacement conversation was used for project work. Claude did not review this implementation; do not invent review approval.

## Implemented behavior

Go API and fixture-only ingestion executable; PostgreSQL migration and durable paper state; integer minor-unit financial arithmetic; currency-separated synthetic accounts; cash reservations, separate acknowledgement/full fill/cancellation/expiry and human-reported paper reductions. Deterministic fail-closed checks cover missing/old/future inputs, fixture assessment provenance, mode/market/currency mismatch, entry pause, per-trade and shared aggregate risk, symbol exposure, and available cash. Intraday is blocked. All live lanes remain disabled and ACTUAL/LIVE startup fails.

One PostgreSQL transaction commits journal, state, audit, dry-run outbox and exact-request idempotency receipt. Repeated identical keys replay prior responses; changed requests conflict. The dashboard omits private receipts. Only the Go service writes state. Research Python/HTTP stubs return STUB/INSUFFICIENT_DATA; no real research or model claims. Telegram code has no network sender; previews always DRY_RUN_NOT_SENT.

Local bearer token, loopback-only default, origin checks, body bounds, unknown-field rejection, server timeouts and no-store responses are implemented. Random credentials are generated in ignored mode-0600 .env.local. Actual token was checked absent from frontend bundle. Compose PostgreSQL digest is pinned to the existing inspected image. Project Go toolchain is 1.26.10; dependencies are pinned in go.mod/go.sum.

## Actual verification

- `go test -race -json ./...`: 39 passing test records (including subtests), 1 deliberately skipped PostgreSQL integration test without TEST_DATABASE_URL. No failures. Log: .local/go-tests.jsonl (ignored local output).
- `scripts/test-integration.sh`: PASS, 1 real PostgreSQL integration test. Repeatable migration, persistent idempotency receipts, second connection/reconnect, 24 simultaneous reservation requests (exactly 10 accepted within exposure budget), 16 concurrent fills (exactly one financial effect), rejected mutation rollback and journal reconciliation.
- `go vet ./...`: PASS.
- Python research tests: 2 PASS, including subcases rejecting ACTUAL/LIVE, missing symbols and financial authority fields.
- Planning tests: 20 PASS. Installer tests: 10 PASS. Tests now use disposable state/package fixtures. Original manifest was not regenerated, safety checks not disabled, real execution state not reset. Initial 10 planning failures were independently reproduced before this fix.
- `planctl validate`: PASS, 17 sprints / 104 tasks; `render --check`: PASS, 18 documents unchanged.
- Browser E2E with real Go/PostgreSQL in disposable database: 2 PASS; desktop buy/reduction lifecycle, risk rejection, duplicate-account error visibility, pause/audit/dry-run; mobile navigation and token clearing on reload. No HTTP interception or fake backend used.
- `scripts/dev.sh`: actually started PostgreSQL, API 8080 and Vite 5173; HTTP health and frontend both 200. Seed script created two explicitly synthetic accounts and five fixture observations. Actual API process stop/restart preserved accounts, quotes and journal byte-for-byte in comparison. Seed retries do not add cash.
- Initial govulncheck reported 12 reachable findings in installed Go 1.26.6 and x/text 0.29.0. Updated project toolchain to 1.26.10 and x/text 0.41.0 / x/sync 0.24.0. Re-run govulncheck v1.9.0: `No vulnerabilities found.`
- Frontend npm audit: 0 vulnerabilities. CI workflow source uses verified action tag commit hashes. No remote CI run is claimed.

Commit identity is recorded in the final session handoff after these files are committed; no guessed hash is used. Tests above ran on the implementation tree before that commit. Documentation-only handoff/status updates follow it.

## Limits

See ADR-017 and contract. Locked JSON aggregate serializes all access and has no bounded retention, production relational schemas, NATS delivery, production identity, immutable external audit, recovery/HA/load evidence or background expiry scheduler. Full paper fills only; zero-fee assumptions; no strategy/return evidence. D-01 through D-11 unresolved. Real provider licensing, accounts/settlement, real financial/history research, approved live risk, Telegram pairing and independent release review remain blockers. Existing original production tasks are not claimed DONE.

No push, paid resources, production deploy, real external notifications or brokerage actions occurred. No real credentials/provider data were requested. Review remains pending.

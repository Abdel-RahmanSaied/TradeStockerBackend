# Independent review — local paper increment

Date: 2026-10-23. Reviewer: independent-review-agent (separate delegated reviewer; not codex-agent, the implementation owner). Initial decision: changes required for S00-T07 and S00-T08. S00-T02 accepted within its bounded criteria. This review grants no production, deployment, data-provider, risk-policy or lane-activation approval.

## Exact reviewed trees and scope

Backend HEAD: `4d419a01baf2da76628c95c277d9a758d691763b`, including product commit `280cb80076af2753fdb2065245a2855354bd4380` and follow-up `32ade29`. Frontend HEAD: `b36a34d33587816523dcc3bd93d896746493b9f7`, sibling TradeStockerFrontend. Both working trees were clean at review start. No implementation, state, generated sprint document or shared handoff was edited by this reviewer. This report is the sole repository write.

Read root/docs AGENTS, SOURCE_OF_TRUTH, AGENT_PLAYBOOK, relevant catalog/state records, latest handoff `sessions/2026-10-23-codex-increment.md`, requirements traceability, scope-review, local increment evidence, README, local API contract and ADR-017. Reviewed Go financial/state/HTTP/startup code, migrations and tests; frontend API/state/lifecycle/components and tests; Python research boundary. Acceptance is assessed against S00-T02/T07/T08, not the unimplemented production roadmap.

## Actionable findings

### R1 — P1: position merging bypasses the shared paper risk limit

Locations: `internal/paper/service.go:160` (risk projection in Evaluate) and `internal/paper/service.go:256`–`260` (Fill merges quantity/cost and takes the minimum stop without checking resulting risk). Relevant full-exit behavior: `internal/paper/service.go:291`–`293` leaves the old stop on a zero-quantity row.

With a USD account of 1,000,000 minor units and fresh bundled fixtures:

1. Evaluate, acknowledge and fill 9 AAPL at entry/fill 20,000, stop 19,999.
2. Evaluate, acknowledge and fill 1 AAPL at entry/fill 20,000, stop 10,000.
3. Both candidates are PROPOSED. The final position has quantity 10, cost 200,000, stop 10,000, and planned risk 100,000. The documented 3% aggregate limit is 30,000. `State.Validate()` returns nil.

Each candidate alone fits the per-trade limit and total symbol exposure is exactly 20%; the defect arises when the later stop is applied to all existing shares. The same problem can arise with two pending reservations filled in either order, so merely rejecting one sequential example is insufficient. Projection must account for all active reservations and the position semantics used at fill, and Fill must validate the projected current state before any financial effect.

Also reproduced: buy 1 AAPL with stop 10,000, fully sell it at 20,000, then buy 9 with requested stop 19,999. The new position silently inherits the old 10,000 stop and 90,000 risk. Reset or replace the closed position when reopening; do not apply an exited trade's stop to a new position.

Evidence: two temporary Go overlay tests invoke the real domain operations, require the unsafe outcomes above, and pass on the reviewed tree. The first scenario also reproduced through the actual authenticated HTTP API and PostgreSQL in a disposable database. No direct database mutation was used. Fix with meaningful regressions for stop direction, pending fill order, full exit/re-entry and changing account equity before fill. Do not raise paper risk limits.

### R2 — P2: incomplete successful response discards the financial retry key

Locations: sibling frontend `src/api.ts:13`–`15` and `src/api.ts:25`–`29`.

`request()` catches every JSON-body read/parse failure and converts it to null. For HTTP 200 it returns that null as a success; `mutation()` then deletes the pending idempotency key. A server may have committed a paper reduction or reservation before the response body is lost or truncated. Retrying the same operation then generates a new key and can create a second financial effect. The UI can also announce success without a confirmed result.

Independent runtime reproduction loaded the actual `src/api.ts` through Vite's `transformWithOxc` in memory, replaced fetch with an HTTP 200 Response containing truncated JSON, and called the same sell mutation twice. Output: `{"firstResult":null,"secondResult":null,"sameIdempotencyKey":false,"requestCount":2}`. This is a controlled transport-boundary reproduction; it did not send a real sell or external request.

Treat failed/incomplete/malformed successful JSON as an ambiguous failure and preserve its operation key. Cover a response-body read failure after headers, invalid/truncated JSON, and retry with the original key. Keep useful fallback messages for non-JSON error responses.

## Independent verification

All commands ran locally on the exact trees above unless described as a temporary overlay. No remote CI result is inferred.

- `python3 docs/tools/planctl.py validate`: PASS, 17 sprints/104 tasks. `status` and `ready` inspected. `render --check`: PASS, 18 generated documents unchanged.
- `go test -race ./...`: PASS all packages; integration test intentionally skipped without its database variable. `go vet ./...`: PASS.
- `./scripts/test-integration.sh`: PASS, 1 real PostgreSQL test, repeatable migration, durable exact-request receipts, second connection, 24 concurrent reservations, 16 concurrent fills with one effect, rollback and journal reconciliation. Unique test database cleaned by script.
- `python3 -B -m unittest discover -s research -p 'test_*.py'`: PASS, 2 tests.
- `python3 -B docs/tools/test_planctl.py`: PASS, 20 tests. `python3 -B test_install_docs.py`: PASS, 10 tests. Their disposable fixtures do not reset live execution state.
- Frontend `npm test`: PASS, 6 tests. `npm run build`: PASS TypeScript/Vite.
- `./scripts/test-e2e.sh`: PASS, 2 Playwright browser tests, real Go process/PostgreSQL and desktop/mobile frontend; no HTTP interception. It covered normal paper lifecycle/rejection/pause/audit and token reload. Parent stopped its local app first. Temporary database/API/frontend cleaned up.
- `go test -overlay /var/folders/pm/4d5lwn9n32b3tq8sdkhlq4qm0000gn/T/tradestocker-review-ezkryn7i/overlay.json -run TestIndependentReview -v ./internal/paper`: PASS, 2 deliberate defect reproductions. They assert the unsafe original outcomes, so they must be replaced/inverted when validating a fix. Overlay files are outside the repository and preserved for re-review.
- A separate inline Python HTTP check created a uniquely named `tradestocker_test_*` database, started the built exact-tree API on 127.0.0.1:18081, reproduced R1 through ordinary authenticated endpoints, stopped and restarted the actual API process, and compared accounts, quotes, positions, recommendations, journal. All persisted exactly; replaying the original account request returned its exact durable receipt. API and disposable database cleaned in finally.
- Frontend transport reproduction used `node --input-type=module`, Vite `transformWithOxc`, and an in-memory HTTP 200/truncated-JSON Response. It confirms R2 as described. Two initial attempts using TypeScript/Node transform APIs were unavailable; the Vite transform reproduction then ran successfully. These tooling failures are not product findings.

Existing suites passing do not resolve R1/R2; they omit these cases.

## Task acceptance decisions

- **S00-T02: ACCEPT.** All 15 requirements are mapped; SOURCE_OF_TRUTH section 7 defines OFFLINE_REPLAY/SHADOW/PAPER/LIVE_ADVISORY and the private single-owner increment. Product approval is separated from implementation/release approval. Production/live defaults are disabled. Broker orders, leverage, shorts, derivatives/options and scalping remain excluded. No conflicting scope statement found that blocks this bounded task.
- **S00-T07: CHANGES REQUIRED.** Durable storage/migrations, synthetic provenance/freshness, acknowledgement distinct from human fills, transactional journal/audit/idempotency, local dry-run notifications and research stub are present and independently exercised. R1 violates the claimed shared paper risk safety within this increment, not a deferred production feature. Re-review required after correction; leave IN_REVIEW or return through the documented review-changes path.
- **S00-T08: CHANGES REQUIRED.** Real-API dashboard views, synthetic/paper labels, human lifecycle controls, visible errors/loading, startup instructions and passing unit/build/browser evidence satisfy the basic surface. R2 violates the increment's claimed ambiguous-write retry protection. Re-review the corrected API client and regression tests before acceptance. No need to add unrelated production identity or full S13 scope.

## Explicit limits

This was a source/test/runtime review of a bounded local synthetic PAPER prototype. It is not proof of market-strategy quality, profit, licensed data, approved risk values, real account/settlement behavior, actual trading, Telegram delivery, production authentication, HA/recovery, event-stream/relational production architecture, remote CI or deployment. Original roadmap tasks and decisions D-01 through D-11 retain their own gates. History/receipt retention, serialized aggregate throughput, API-access expiry, full fills and zero-fee assumptions remain documented prototype limits, not newly invented acceptance blockers. No push, deploy, trading, external message or paid-resource action occurred. No secrets are included in this report.


## Re-review addendum — both findings resolved

Date: 2026-10-23. Same independent reviewer: independent-review-agent. Exact backend reviewed: `e699bbf2eff6ada9bf6c82da03c662dde00ec6f9`, including fix `0ff5af4`. Exact frontend reviewed: `245abec457c4571fcdae28235fb1d0ffd5cb9002`, including fix `48468f0`. Both working trees were clean before this addendum. The original findings above are retained as historical review evidence; this addendum supersedes their unresolved status and the initial S00-T07/T08 decisions.

R1 is resolved for the bounded increment. Independently inspected the projected risk calculation: it groups held positions and every active reservation by symbol, uses the same minimum-stop merge semantics as Fill, and rejects excessive projected aggregate risk at evaluation. Fill rechecks current equity, per-trade risk, aggregate projected risk and symbol exposure before changing financial state. Reopening a zero-quantity position sets the new stop. The fix retains the existing paper test limits. Read the new regressions for held/pending widening, safe mixed-stop fills in either order, legacy pending proposals, full exit/re-entry and equity loss before fill.

Independent assertions now require the safe outcomes instead of the original bugs. `go test -race -count=1 -overlay /var/folders/pm/4d5lwn9n32b3tq8sdkhlq4qm0000gn/T/tradestocker-rereview-jk1axwzx/overlay.json -run TestIndependentFix -v ./internal/paper` passed two reviewer-authored tests: held and pending risk rejection without changing account cash/reservations; full exit/re-entry using stop 19,999 and planned risk 9 minor units. Temporary overlay files remain outside the repository.

An additional real authenticated HTTP/PostgreSQL run used a new disposable database and exact rebuilt API on 127.0.0.1:18081. It reproduced the original entry sequence: the widening candidate now returns NO_TRADE with PAPER_TOTAL_OPEN_RISK_EXCEEDED and leaves accounts unchanged. It fully exited, opened/closed the low-stop position and re-entered 9 shares at stop 19,999; the persisted position uses the requested stop and planned risk 9 minor units. Stopping and restarting the actual API process preserved accounts, quotes, positions, recommendations and journal exactly; the original account idempotency receipt replayed exactly. The API and disposable database were cleaned up.

R2 is resolved. Independently inspected the client: unreadable/invalid successful JSON now throws an ambiguous-result error, as do null or primitive success bodies; these errors retain the pending mutation key. Non-JSON error responses still yield a useful error. Independently loaded the actual fixed API client through Vite's in-memory transform and checked truncated JSON, null JSON, primitive JSON, and a ReadableStream that fails after successful headers. All four rejected the uncertain first result, reused the original key on retry, and returned the confirmed retry object.

Repeated independent verification on the exact fix trees:

- `go test -race -count=1 ./...`: PASS, all packages; intentional integration skip without TEST_DATABASE_URL. The count flag forced a fresh run.
- `go vet ./...`: PASS.
- `./scripts/test-integration.sh`: PASS, real PostgreSQL migration/replay/concurrency/rollback/reconciliation test, unique disposable database cleaned.
- Frontend `npm test`: PASS, 10 tests. `npm run build`: PASS TypeScript/Vite.
- `./scripts/test-e2e.sh`: PASS, 2 real-API desktop/mobile browser tests. Separate API/database cleaned; normal workspace data untouched.
- `python3 docs/tools/planctl.py validate`: PASS, 17 sprints/104 tasks. `render --check`: PASS, 18 generated documents unchanged.

Final acceptance decisions: **S00-T07 ACCEPT** at backend `e699bbf2eff6ada9bf6c82da03c662dde00ec6f9`; **S00-T08 ACCEPT** at frontend `245abec457c4571fcdae28235fb1d0ffd5cb9002`, with the reviewed backend API tree above. **S00-T02 remains ACCEPTED** under the original scope assessment. No remaining actionable finding was identified in this re-review. No implementation or execution-state edits were made by this reviewer. These decisions cover only the defined local synthetic PAPER increment. All explicit production, licensing, risk-approval, strategy-evidence and live-activation limits above continue to apply.

# Independent SEC research backend checkpoint review

Date: 2026-10-23. Reviewer: independent-review-agent, distinct from the original Codex draft author and the Claude rework author. Exact backend checkpoint: `7fa1281cfe532208622ce5d493455adeff42d587`. Working tree was clean at review start. Decision: **CHANGES REQUIRED for this backend checkpoint** because of R1/R2 below. This is not acceptance of S07-T07; frontend work, real-API browser coverage, the API contract, implementation evidence and architectural documentation remain outside this checkpoint assessment.

## Scope examined

Read the current task criteria and governing AGENTS/source/playbook, then reviewed the full checkpoint diff, SEC provider, durable job handlers/worker, Go parser boundary, migration 002, Python sec-annual-v2 and tests, HTTP authentication integration, startup and integration-test script. No product or state edits were made by this reviewer. This report is the sole repository write. External scratch tests reside under the projectless chat's work/reviewer-research directory. No shared application ports or normal paper database were used.

The checkpoint meaningfully implements authenticated idempotent queueing, fixed official SEC endpoint construction, redirect refusal, source-body limits and pacing, issuer verification, durable raw sources and append-only audit, retry/backoff/leases/fencing, parser isolation, and non-actionable financial reports. Earlier findings about duplicate conflicting accounting contexts, period ends after filing, quoted zero-padded CIKs, last-attempt shutdown and ordinary provider-wide cooldown are addressed in the reviewed source and normal regressions. These positive results do not resolve the two additional cases below.

## Actionable findings

### R1 — P2: parser output limits are bypassed by a promoted buffer method

Location: `internal/research/analyze.go:19`–`28`, used at lines 66–77.

`boundedBuffer` embeds `bytes.Buffer`. This promotes Buffer.ReadFrom, which the subprocess output-copy path can call instead of the overridden Write method. Consequently the nominal 1 MiB stdout and 4 KiB stderr bounds are not enforced by the actual process-copy path. There is also no independent raw-length check before accepting a report.

Independent external-overlay reproduction uses the real Service.analyze and Python process with a valid report containing the required issuer/source/authority metadata and a 2 MiB padding field. On this exact checkpoint it returns 2,097,620 bytes with `err=nil`, while reportLimit is 1,048,576. This report passes the Go gate and could be persisted. This is a controlled parser-boundary test through the existing Script test seam; it did not replace the product parser or submit any external data. It demonstrates the absent bound, not a claim of a public code-execution vulnerability.

The existing oversized-output regression writes only x characters, so it passes when JSON validation rejects the unbounded output, without proving that resource limits worked. Use a named inner buffer or implement a bounded ReadFrom as well as Write; add valid-JSON oversize and stderr-copy regressions and a final report-length check. Ensure excessive output terminates/fails the analysis with bounded retained memory.

### R2 — P2: concurrent shutdown discards an observed SEC access-limit cooldown

Location: `internal/research/worker.go:173`–`188` in afterFailure and its shutdown release path.

The cancellation check runs before the ProviderError classification. If SEC has returned SEC_ACCESS_LIMITED while the parent context becomes cancelled, the code releases the job using the shutdown path and never persists the provider cooldown.

Independent real-PostgreSQL overlay reproduction supplied a fetcher that cancels the parent immediately before returning retryable SEC_ACCESS_LIMITED. ProcessOne settles successfully through release. The resulting job is QUEUED with WORKER_STOPPED; research_provider_state contains no active cooldown. Replacing the service provider with a fresh test fetcher then processes the job immediately. No real SEC traffic was used. This models cancellation arriving after an access-limit response has been observed, before afterFailure records it.

Persist a known access-limit response and its provider-wide cooldown using a bounded cleanup context even when shutdown concurrently cancels the parent. Preserve ownership fencing and attempt exhaustion. Cover an ordinary attempt and the last attempt. A cancellation where no access-limit response was observed can retain the existing shutdown behavior.

## Independent checks and concrete results

- `go test -race -count=1 ./...`: PASS, fresh full package run. Real database tests intentionally skip without TEST_DATABASE_URL.
- `go vet ./...`: PASS.
- `python3 -B -m unittest discover -s research -p 'test_*.py'`: PASS, 25 tests, including 23 financial parser tests and 2 existing research-boundary tests.
- `python3 docs/tools/planctl.py validate`: PASS, 17 sprints/105 tasks. `render --check`: PASS, 18 generated documents unchanged.
- In one uniquely named disposable tradestocker_test_checkpoint_* database, `go test -race -count=1 -p 1 -v ./internal/store ./internal/research`: PASS. This independently ran the store persistence/concurrency test and all 10 research PostgreSQL tests, alongside research unit/provider tests. Coverage includes 12 simultaneous same-key creates, changed-request conflict, queue bounds, raw originals/hash downloads and append-only triggers, actual Python analysis, retry/permanent errors, lease recovery and stale-attempt fencing, shutdown/restart, final-attempt exhaustion, durable provider-wide cooldown and leader exclusivity. Packages were sequential within this review database. The database was dropped afterwards.
- In a separate uniquely named disposable tradestocker_test_checkpoint_extra_* database, reviewer external overlay `checkpoint-overlay.json` ran TestIndependentCheckpointCooldownAndFinalRelease and TestIndependentCheckpointAccessLimitDuringShutdown. The first verifies normal provider-wide cooldown, no extra claim/attempt during cooldown, final-attempt release to FAILED and unchanged raw paper workspace bytes: PASS. The second deliberately asserts the defective R2 outcome: PASS reproducing no cooldown and immediate provider call after restart. Database dropped afterwards.
- `go test -race -count=1 -overlay <projectless-work>/reviewer-research/checkpoint-overlay.json -run TestIndependentCheckpointOutputLimit -v ./internal/research`: PASS as a deliberate R1 reproduction, reporting accepted bytes 2,097,620 versus limit 1,048,576 with nil error. Reproduction scripts remain outside the repository; these tests assert original defects and must be inverted for fix verification.

No additional real SEC request was made by this reviewer for this checkpoint; the coordinating agent separately reported a successful official fetch. This review does not claim that as its own observed runtime result. The provider's fixed URLs, identities, bounds, errors and pacing were independently exercised with its controlled transport tests. No final frontend, browser or complete-task approval is inferred from backend tests.

## Remaining scope and limits

Re-review R1/R2 against new exact commits before backend acceptance. Even a subsequent backend acceptance will not mark S07-T07 DONE: its actual frontend/API workflow, contract, evidence, architectural notes and required real-fetch evidence must also be complete and reviewed. This report is not production authorization, a data-license decision, financial-health certification, price-history analysis, strategy/performance evidence, live advice or trading approval. Reports remain INSUFFICIENT_DATA and ineligible for entry/backtest/write authority. No push, deploy, paid resource, brokerage action, external message or shared-app operation was performed by this reviewer. No secrets are included.


## Independent fix re-review — backend checkpoint accepted

Date: 2026-10-23. Reviewer: independent-review-agent. Exact backend fix commit: `7ab44180b15cafbb16f7f3a5141bd8dfcdada440`. Decision: **ACCEPT the backend-only S07-T07 checkpoint at this commit**. R1/R2 above are resolved; the initial findings remain as historical evidence. This does not accept or complete the full S07-T07 task. Frontend/API browser workflow, final contract, feature evidence and architectural documentation still require completion/review.

Tests below ran from a git archive of the exact fix commit in projectless work/reviewer-research/backend-7ab4418, excluding ongoing working-tree changes. The reviewer did not modify product code or execution state.

R1 fix independently verified: boundedBuffer uses a named inner buffer, so it does not expose bytes.Buffer.ReadFrom. The subprocess pipes now pass through the bounded Write method, and a final report-length gate provides another check. A reviewer-authored inverted overlay first accepted a small valid report as its control, then rejected the same valid metadata with 2 MiB padding and returned no report bytes. It also checked absence of io.ReaderFrom and ran a 1 MiB stderr flood with a 3-second safety deadline: the process failed promptly, retained zero bytes (within 4096), and did not reach the deadline. Author regressions also cover valid oversized reports, io.Copy and stderr flooding. No live SEC request or product parser substitution occurred; this is an isolated process-boundary test.

R2 fix independently verified: afterFailure distinguishes an observed ProviderError before treating cancellation as an interrupted fetch; a bounded 5-second context outlives cancellation while recording the response and provider-wide cooldown transactionally. The reviewer-authored inverted PostgreSQL overlay cancelled the parent immediately before returning SEC_ACCESS_LIMITED, on both the first and third attempt. It verified the retained SEC_ACCESS_LIMITED error, correct QUEUED/FAILED outcome, at least nine minutes of durable cooldown remaining, and no provider call or consumed attempt from a fresh worker using a new connection pool. Both cases passed. Standard ownership fencing remains in the transactional settle path.

The timestamp fix was also reviewed and exercised. Go now emits UTC with exactly six fractional digits. Python normalizes fractional seconds before its timezone-aware datetime parse, preserving microseconds and UTC conversion. Actual review interpreter: Python 3.9.6. All nine fractional lengths 1–9 were independently checked with a +03:00 offset. The full Python suite and repeated embedded-parser PostgreSQL runs passed on that interpreter.

Independent fix verification results:

- Archived exact commit: `go test -race -count=1 ./...` PASS; `go vet ./...` PASS. Database tests deliberately skip without TEST_DATABASE_URL in this first command.
- `python3 -B -m unittest discover -s research -p 'test_*.py'`: PASS, 27 tests on Python 3.9.6.
- `planctl validate`: PASS, 17 sprints/105 tasks. `render --check`: PASS, 18 generated documents unchanged.
- Own uniquely named disposable tradestocker_test_fix_* database: real PostgreSQL store test PASS with count 1, then the complete research package with `go test -race -count=3 -v ./internal/research` PASS for all three runs. This repeatedly exercised the actual embedded Python parser, recovery, ordinary/cancelled access-limit cooldown, source storage, idempotency and last-attempt shutdown. Sequential package execution avoided shared-test-data contention. Database dropped afterwards.
- External `fixed-checkpoint-overlay.json`, TestIndependentFixedOutputLimits: PASS, inverted valid-JSON stdout and stderr resource-bound checks against the archived code.
- Separate own disposable tradestocker_test_fix_extra_* database, same external overlay, TestIndependentFixedCanceledAccessLimit: PASS for prior-attempt counts 0 and 2 with a new connection pool. Database dropped afterwards.
- Direct independent Python check of fractional lengths 1–9: PASS, correct UTC hour and expected microseconds on 3.9.6.

The coordinating reviewer separately reported a full live SEC job flow from the same archived fix commit, including actual HTTP job handlers/PostgreSQL/SEC/Python and source downloads with unchanged paper state. This addendum does not claim that separate run as this reviewer's own observation, and avoids redundant external requests. Its exact evidence remains the coordinating agent's responsibility in the complete feature evidence pack.

No remaining actionable backend finding was identified within this bounded checkpoint review. No shared ports 8080/5173/18080, normal paper database, push, deployment or trading action was used. Only this report was edited in the repository. All original scope and production/live exclusions above remain in force; **the full S07-T07 status must not be marked DONE on this backend-only acceptance**.


## Independent frontend checkpoint review — accepted

Date: 2026-10-23. Reviewer: independent-review-agent, separate from frontend author Claude. Exact frontend commit: `7c739f5beb51089280598804e1499d4691cc05c4`. The frontend tree was clean at start and remained on that exact clean commit after tests. Decision: **ACCEPT the frontend checkpoint** with reviewed backend `7ab44180b15cafbb16f7f3a5141bd8dfcdada440`. No new actionable frontend finding identified in this bounded review. This is not full S07-T07 acceptance; the coherent cross-repository runner, final API contract, feature evidence, ADR and final real-API E2E remain pending.

Read the full frontend checkpoint diff, Research component, research types/formatting/cell/poll/error helpers, source download helpers and API retry behavior, App integration, unit tests and both controlled/real-API research browser test files. Types and displayed fields match the reviewed backend v2 report structure. Reported dollar values stay strings/BigInt; withheld and missing values are distinguished; original filing identity, timestamps, citations and hashes are visible; later-filing differences and limitations remain explicit; entry/backtest outcomes are read from the gated report. Downloads require SHA-256 verification before a browser file is offered. The local token stays in memory.

The prior wrong-job race is addressed: selection is stored in a ref before refresh starts, stale responses cannot apply details to a different selection, and response sequencing prevents an older completed response from replacing a newer applied response. Poll scheduling avoids an older completion installing a new timer over the current generation. Action errors are distinct from recoverable polling errors.

Independent verification:

- Frontend `npm test`: PASS, 18 tests across 2 files, including source SHA refusal, exact amounts beyond floating-point precision, missing/withheld cells, ratio formatting, polling/error helpers and existing ambiguous-mutation retry regressions.
- `npm run build`: PASS, TypeScript and Vite.
- All four tests in `tests/research-ui.spec.ts`: PASS on private Vite port 51590, with a reviewer-created external Playwright configuration and cache/artifacts under projectless work/reviewer-research. No shared ports or backend were used. The controlled API double covered slow old selection responses, creating/selecting a new job while the old request remains pending, persistent action errors alongside polling failure/recovery, and withheld/missing/restated data plus hash-verified download/refusal. The first two record every observed heading so a transient wrong report cannot be hidden by eventual assertions. This is controlled-timing UI evidence, not real-backend evidence.
- The reviewer's earlier external frontend-race.mjs reproduction was rerun against this exact frontend on separate private port 51627. Its final selected job was BBB and its report heading was BBB; the original buggy draft had produced a BBB selection with AAA detail. This confirms the originally observed mismatch is resolved.
- Git HEAD and clean frontend status rechecked after tests. Both private Vite/browser sessions were closed. No real SEC request, database operation, shared E2E runner or production action was performed in this frontend checkpoint review.

The author/coordinator separately reported successful offline and live SEC E2E runs. This addendum does not count those as this reviewer's independent final cross-repository verification. Run/review the coherent final runner and documentation commits before accepting the full task. Backend and frontend checkpoint acceptances do not activate any live lane or approve financial advice.


## Runner and documentation review — final milestone still pending

Date: 2026-10-23. Reviewed backend runner/docs checkpoint `5da12da73cd66f52b53dc44ff08cd1260329c271` with frontend checkpoint `7c739f5beb51089280598804e1499d4691cc05c4`. Core backend changes since accepted 7ab4418 are comments only; previous code checkpoint acceptances remain scoped as above. The new runner P2 below prevents final milestone acceptance until fixed and independently verified. This section does not rerun or revoke already-passed backend/frontend suites.

**R3 — P2: runner readiness can target a preexisting API instead of its disposable child.** `scripts/test-e2e.sh:29`–`41` and `scripts/research-smoke.sh:27`–`56` start on fixed port 18080, then accept any health response on that port without proving it belongs to the launched child. If an existing instance accepts the same development token, research or paper test mutations can reach that instance even though the intended child never bound its port.

Independent external-only reproduction: copied the exact scripts into a temporary fake repository, replacing only literal port 18080 with private port 51668. An already-running fake server owned that port. Fake go produced a child that exits 1; fake docker performed no database operations; fake .env contained invented credentials only. Both runners exited 0 after the old server answered health. The smoke runner sent authenticated POST/research/jobs to the old server. The E2E runner reached the controlled fake npm driver, which sent authenticated POST/paper/accounts there. The private server/resources were cleaned; no real credentials, SEC request, database or shared port was used. Reproduction is retained outside the repositories as projectless work/reviewer-research/runner-isolation-review.py. Fix with occupied-port refusal before setup, child liveness checks, and positive per-run readiness identity (for example a fresh token with an authenticated read); a generic health response alone does not establish isolation. Re-run this adverse test against the exact fix commit.

Documentation/source discrepancies to correct before final acceptance:

- **D1 — source accuracy:** ADR-018 context, provider UNKNOWN row, worker comment and draft feature evidence assert that SEC publishes no block duration. The cited [SEC Internet Security Policy](https://www.sec.gov/about/privacy-information) actually gives ten minutes below the request threshold as its resumption guidance. Describe that conditional guidance accurately; applying a blanket ten-minute pause to 403/429 remains a conservative client choice, and that status-code classification is still ours. The official pages were read during this review; no company-data API request was needed.
- **D2 — contract field meaning:** contracts/research-api.md:41 defines missing_metrics as not reported in the filing. The parser includes withheld metrics in that array too: an independently inspected synthetic conflict returns missing_metrics containing revenue while withheld_metrics explains its conflicting reported values. Define the field as unavailable in the selected report, including absent, excluded or withheld inputs. The frontend's corresponding not-reported caption has the same minor wording issue; its explicit Withheld cells and evidence remain correct.
- **D3 — older context scope:** contract line 52 places filings older than the recent index under not covered, but parser/evidence can use older companyfacts contexts with estimated times. State that supplemental older submissions pages are not fetched; older companyfacts contexts can still appear with estimates.
- **D4 — availability terminology:** contract line 44 should describe SEC acceptance as a filtering proxy rather than certified first public availability. The [SEC Webmaster FAQ](https://www.sec.gov/about/webmaster-frequently-asked-questions) distinguishes acceptance from later website availability and says the latter has no timestamp. The current API analyzes already-retrieved current sources and reports backtest eligibility false, so this is a documentation clarification, not a request to expand historical eligibility or change the accepted parser scope.

The API fields, ordinary queue/retry/lease/cooldown behavior, source hashes, gated report schema and financial-state separation otherwise align with the reviewed implementation. The provider record appropriately keeps market-feed decisions D-01/D-02, commercial review and all live activation outside this increment. Feature evidence was still being written; its uncommitted status and any temporary missing links are acknowledged, not treated as proof of task completion. Wait for coherent exact runner/docs commits and final real-API E2E evidence before full S07-T07 acceptance.


## Final independent milestone review — S07-T07 accepted

Date: 2026-10-23. Reviewer: **independent-review-agent**, distinct from both the original draft author and Claude, the development/rework owner. Final exact backend: `4db63e550d6042ef1869dfe3487522041bade902`, including runner/docs fix `7cf3123ed47ded23f3f74968c98f5bd672855f9c`. Final exact frontend: `2e1841916eecefbb954a3c304d965cff3cf93744`. Decision: **ACCEPT S07-T07 within its actual bounded local US annual-filings research criteria**. This final section supersedes earlier pending/changes-required milestone decisions. R1, R2, R3 and D1–D4 are resolved; earlier sections remain historical review evidence. No remaining actionable finding was identified in this scope.

The functional Go/Python implementation is unchanged from the independently accepted 7ab4418 checkpoint except comments. Frontend differs from accepted 7c739f5 only by the corrected no-usable-value caption. Those exact diffs were checked; broad accepted application suites were not needlessly repeated. The final coherent runner, contract, ADR, provider due-diligence record and feature evidence were reviewed together with the earlier substantive code/tests.

### Runner isolation fix — independently verified

The shared helper refuses occupied ports before creating a test database or building, generates a fresh per-run token without changing .env.local, rejects a dead/zombie child, checks listening-process ownership when lsof is available, and requires an authenticated research-status response with the expected safe API shape. With lsof absent it still checks child liveness and the fresh token, which a real stale TradeStocker API does not know. Both scripts use the helper; default E2E explicitly clears the SEC contact, while live access is opt-in.

- `python3 -B scripts/test_disposable_api.py`: PASS, 8 tests. The new suite is wired into Makefile and CI; no remote CI run is inferred.
- `sh -n scripts/disposable-api.sh scripts/test-e2e.sh scripts/research-smoke.sh`: PASS.
- Reviewer's external original occupied-port reproduction, adapted only to copy the new helper and use private port 52692: both fixed scripts exit 1 before child startup, with zero requests to the old server. Earlier vulnerable copies exited 0 and sent authenticated mutations there.
- Reviewer's external post-preflight race reproduction creates a stale server during the fake build stage, after the preflight has passed. For each runner, tested both a stale server that rejects the fresh token and one that ignores authentication but is owned by another PID. All four cases exit 1 with no POST and without starting the E2E driver. Private ports 52734/52740/52747/52752 and fake credentials/commands only; no real database or SEC calls. All fake resources were closed. Scratch programs remain outside both repositories as runner-isolation-fixed-review.py and runner-wrong-instance-fixed-review.py.

### Final independent real-API browser gate

After explicit coordination confirmed shared ports free, this reviewer ran sequentially from the exact final backend against the exact final frontend:

1. `./scripts/test-e2e.sh`: **7 passed, 1 intentionally skipped** in 17.5 s. This runs the actual Go/PostgreSQL paper lifecycle/mobile tests and contact-not-configured research refusal, together with controlled-timing UI regressions. The real SEC test is skipped in this mode.
2. `TS_E2E_REAL_SEC=1 ./scripts/test-e2e.sh`: **7 passed, 1 intentionally skipped** in 23.2 s. The actual SEC browser test passed: real Apple request → durable Go/PostgreSQL job → official SEC documents → isolated embedded Python report → five original annual-report rows, entry/backtest ineligible, original companyfacts downloaded and independently hashed by the browser test, followed by a visibly failed ticker/CIK mismatch with no fallback. The unconfigured-contact test is skipped in this mode. This live test uses real local API traffic, without HTTP interception; the four separate controlled-API tests are explicitly labelled as such.

No extra live smoke run was needed: the final browser run supplied this reviewer's own real-SEC workflow verification, while the coordinator's separately documented exact 7ab4418 API/PG/parser test in sec-research-coordinator-check.md establishes both raw source hashes and byte-for-byte paper isolation. That coordinator evidence is attributed to its actual observer rather than represented as this reviewer's run. Author tests remain separately attributed in the feature evidence.

After both final browser runs, this reviewer confirmed ports 18080 and 5173 were FREE and told the coordinator they could be reused. Both scripts returned successfully through their cleanup paths. Read-only checks before/after these browser runs found the normal paper workspace document MD5 unchanged at `51285359c2d98893a4d47ead4c2fb4da` and its schema migrations still only 1. This observation is before the coordinator's subsequently authorized normal app launch, which may apply migration 2. Only disposable databases and paper balances were used by tests; no normal financial state was mutated by the reviewer.

### Acceptance against the defined task

| Criterion | Independent assessment |
|---|---|
| Authenticated idempotent durable jobs survive retries/restart; bounded official-source fetches | Met: earlier exact-code provider/HTTP/real-PostgreSQL reviews and repeated tests, additional adverse cancellation/output tests, final runner isolation and real-API browser runs. |
| Source hashes, retrieval/filing times, units/periods; incomplete data blocks entry/backtest | Met: original-byte/hash APIs, citations and timestamps reviewed and exercised; current snapshot/acceptance proxy explicitly disclosed; false eligibility enforced by Go and Python. |
| Compatible annual Python metrics, explicit missing data/counter-evidence; no financial write access | Met for the bounded structured annual scope: exact integers/ratios, original-filing contexts, ambiguity withholding, temporal rejection, later differences, environment isolation and unchanged paper-state evidence. |
| Frontend launches/polls/displays reports, errors, provenance and limitations through real APIs | Met: prior 18-unit/build checkpoint, controlled slow-response/error/hash cases, corrected caption and both final real-API E2E modes. |

The corrected API contract explains missing/withheld overlap and older companyfacts contexts. Contract/ADR/provider evidence now distinguish acceptance-time proxy from first public availability, accurately cite SEC's conditional ten-minute recovery guidance, and describe 403/429 handling as this client's classification. The provider record qualifies only local read-only filing research; it does not resolve licensed price-feed decisions or live activation. The ADR remains a local prototype decision record, not production architecture approval.

Final planning checks also passed: planctl validate (17 sprints/105 tasks) and render --check (18 documents unchanged). Execution-state changes remain the coordinating owner's responsibility; this reviewer made no task-state or product edits. The only repository file edited by this reviewer is this report.

### Continuing limits

Acceptance is for this bounded increment, not the original full financial/history roadmap. Annual structured US-GAAP/USD facts are partial research; quarters, notes, auditor opinions, segments/custom taxonomies, valuation, licensed price history, EGX, point-in-time archives, strategy performance, production identity/operations and commercial/LLM use retain their own tasks and approvals. Reports remain INSUFFICIENT_DATA and ineligible for entry/backtest/write authority. One issuer was checked against real SEC; other adverse cases and financial issuers use explicit synthetic fixtures. No live lane, broker action, production deployment, paid resource, Telegram delivery or guaranteed-return claim is authorized by this acceptance.


## Bounded normal-launcher follow-up — accepted

Date: 2026-10-23. Reviewer: independent-review-agent. Exact follow-up commit: `8f867dcd08d25ce19669b31c2971d067dcf74ea7`. Decision: **ACCEPT this two-file normal-launcher fix**. It does not change the earlier full S07-T07 acceptance baseline of backend 4db63e550d6042ef1869dfe3487522041bade902/frontend2e1841916eecefbb954a3c304d965cff3cf93744.

Read the exact scripts/dev.sh and scripts/test_disposable_api.py diff. The launcher now uses the already-reviewed connection-based preflight for 8080/5173, avoiding false occupied-port errors caused by a closed socket in TIME_WAIT. It sources only the shared definitions and does not invoke the fresh disposable-token helper; the normal .env.local token remains in use. Vite is explicitly pinned to 5173 with strictPort, so a bind failure cannot silently move the UI elsewhere.

Independent verification: `python3 -B scripts/test_disposable_api.py` PASS, 10 tests with no skips, including the real TIME_WAIT regression and existing occupied/dead/wrong-instance/token cases. `sh -n scripts/dev.sh` PASS. No application/E2E/full-suite rerun or shared port use was needed for this bounded change. Normal app launch and migration remain the coordinator's operation. No product or state edits were made by this reviewer; only this report was appended. No remaining actionable finding identified in the two-file fix.

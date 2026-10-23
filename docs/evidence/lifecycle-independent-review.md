# S01-T05 independent lifecycle review

Reviewer: `independent_review`. Product author: `claude-code-agent`. Date: 2026-10-23. These are my actual independent runs; author runs are not counted here.

## Current decision

**ACCEPT S01-T05 at backend `b0d6c2119d218de54a7de5bd98078bcfbfd9d780`, tree `127d50ff5bccddf68cbc1b70ca52f5a3bbf529fa`, with frontend `9c75c9183df2ed9457012260bb904a9ea404a960`.** Runtime-tested product code is exact `55aefbaf5618c2662a98b2cea0c12c35fa8708b4`, tree `2b9eedd39f0551ca02c925ce40f678a60ed36e85`. The final direct-child commit changes only README, author evidence and the identical browser runner already independently exercised; all product bytes/modes were re-matched and the new committed history passed the secret gate. No DONE or planning/state change was made by this reviewer.

Frontend was independently cloned at `9c75c9183df2ed9457012260bb904a9ea404a960`. The real API/browser run used the product code at exact55aefba plus one explicitly documented **uncommitted test harness**, `scripts/test-e2e-isolated.sh`, SHA-256 `168f214ea639aa113f2e1b0543909a6de5fc3d28f898410392118b3e0b5a9bd2`. That harness was statically reviewed before copying it into the private clone. It was the sole untracked source file; all239 tracked backend files remained byte/mode identical. The browser binary's Git metadata can consequently report dirty because of this test file; no claim is made that it was built from a completely clean final Git commit.

No remaining blocking defect was observed within the exercised bounded behavior. This does not establish production reliability, all outage/commit races, market-data rights, investment performance or live eligibility.

## Source and test isolation

A private history-preserving clone checked out exact55aefba using private HOME/XDG/Git config/templates/hooks, no prompts, no credential helper or signing, and origin removed before checkout. No canonical/global Git configuration, staging, source or planning file was changed. The exact runner and kill-harness hashes were matched before execution:

- `scripts/test-isolated.sh`: `a4998fd7ec9823630c056d8104638826cda0c58ac5c0999111714725c1e155ef`.
- `cmd/platform-core/kill_test.go`: `31034f8d552ac77cd05cdca10b2eae5081f6ab2609b5d128157cc6afe8703e8c`.

All inherited `TS_*`, database configuration and injected Git/tool environment values were removed from test children, including the five `TS_T05_HELPER/PARENT_PID/FETCH/KEY/LEASE_SECONDS` role controls. Only the test harness supplied helper roles. Official pinned CPython3.14.8/uv0.13.4 were reused from the previously verified reviewer runtime in new owned storage; locked sync passed. Go was pinned through `GOTOOLCHAIN=go1.26.10`, with external reviewer module/build caches and `-mod=readonly`.

The isolated runner created a fresh randomly named, labelled PostgreSQL16.15 container for each run, published only at a random loopback port and using tmpfs data. It never selected the normal55432 cluster as a test target. Its cleanup stopped only the returned container ID after matching the run label; `--rm` removed only that newly created container. No existing container/image/volume was deleted or pruned.

The browser arrangement was deliberate: the private backend clone had its own real `.local` directory and a real sibling `TradeStockerFrontend` private no-remote clean clone. Previously pinned matching-lock dependencies were copied into that owned frontend. All proxy variables, FRONTEND_URL and SCREENSHOT_PATH were removed; the runner supplied fresh local token/ports and explicitly disabled SEC and NATS. Playwright artifacts were confined to a new `.local/t05-e2e.2QInkn/playwright` child folder. No normal frontend/source/output folder was cleaned.

## Actual independently executed checks

| Scope | Result |
|---|---|
| Five relevant Go packages, race, real disposable PostgreSQL | **55 top-level PASS, zero SKIP/failure** |
| Exact three build-tagged process tests, race, a second fresh PostgreSQL | **3 PASS, zero SKIP/failure** |
| Changed dependency-direction/architecture checks, race | **4 top-level PASS** (including their synthetic negative subcases) |
| Python research/worker/telemetry tests | **43 PASS**, zero SKIP/failure/error |
| Built wheel, fresh install/import and installed console | PASS; telemetry/worker/parser modules match exact source; 3 response + 3 safe log lines |
| Pinned Python ruff and mypy | PASS |
| Go vet over changed runtime packages | PASS |
| Real API + private frontend browser suite | **7 PASS, 1 intentional external-SEC SKIP**, zero failure |
| Full committed-history gitleaks and reviewed-waiver gate | **2 findings, 2 exact reviewed waivers; zero open/unused** |
| govulncheck including test dependencies | **Zero imported/reachable affected-package/symbol findings**; one module-only advisory documented below |

The55 Go tests were the actual top-level outcomes across `internal/store`, `internal/research`, `internal/natsbus`, `internal/httpapi`, and `cmd/platform-core`; they are not a claim that the entire repository Go suite was rerun. The four architecture tests are separate; the tagged three are separate unique tests, not all untagged tests counted again. The43 Python outcomes include the existing parser/worker tests and eight new telemetry tests. Other unchanged old comprehensive CI suites were not needlessly repeated.

The earlier owned redaction snapshot's **six independent group probes plus two nested-Licensed probes and eleven authored telemetry top-level tests** had already passed with race checks. All four telemetry files in exact55aefba matched that tested snapshot byte-for-byte, including the tests. Those results were reused and are not silently added to the55/4/3 current Go counts. Earlier genuine failures remain preserved.

Actual commands were the cleared five-package invocation of `sh scripts/test-isolated.sh`, then the same runner with `GOFLAGS=-mod=readonly -tags=t05kill` and the exact three-test TEST_RUN pattern, targeted architecture race/vet commands, pinned Python unittest discovery/ruff/mypy, pinned gitleaks and `govulncheck -test -format json ./...` with their checked gates, and the separately reviewed browser runner.

## Behavior demonstrated

### Durable retry admission, clock and concurrency

The previous nominal reservation budget could admit overdue retries in a burst. The corrected version serializes actual claims using a PostgreSQL transaction lock, counts durable retry ATTEMPT_STARTED events and leaves excess due retries queued. The independently run test admitted exactly the configured capacity from four overdue retries, retained that admission count across a new service/pool, permitted first attempts independently, then allowed the next retry after its real window. The event-time sliding-window assertion passed.

The earlier transaction-start clock issue was also exercised: a waiting claim was held behind its own test database lock for2.5seconds, longer than its one-second budget window and two-second lease. After release, admission time/event and lease were current, the lease remained live, and the next retry stayed held. The code samples a database `clock_timestamp()` after the claim lock and passes explicit event/lease timestamps.

The slow-provider fixture reached exactly two in-flight fetches for a two-worker pool. Stopping while both workers were blocked released their jobs, left no RUNNING job, and recorded two interruption events. The SEC-provider concurrency test used a fake transport and verified serialized, spaced requests; worker-count limits are not substituted for the provider's rate limiter. The retry budget's documented default is six retry starts per ten minutes per shared database; first attempts are excluded and each job still has a three-attempt ceiling.

### Cancellation, lease recovery and paper rollback

The deterministic stop-after-analysis test canceled immediately after a valid report and still committed SUCCEEDED using the bounded uncancelled settlement path. A fenced release after success changed nothing; failed analysis interrupted at the same boundary requeued. Existing provider-cooldown, last-attempt stop, retry/crash recovery, source preservation and leadership tests also passed in the same actual database run.

Three tagged tests started only children of their own test binary, verified parent/child identity and signalled only the returned child process objects:

1. SIGKILL during a synthetic blocked fetch left the job RUNNING/attempt1 with no false settlement. The restarted worker recovered after the lease and completed attempt2 with the expected ordered events.
2. SIGTERM during a synthetic blocked fetch immediately requeued the job; restart finished well before its long lease, without a lease-recovery event.
3. SIGKILL inside an owned paper transaction's precommit closure left the stored document byte-identical. Retrying the same operation key applied one account creation and replay returned the same response.

**Limit:** the financial process test kills before commit, inside the open row-locked transaction; it does not prove every ambiguous network/commit boundary or kill timing. No real financial account, trading action or normal paper workspace was used. The subprocess cleanup waits are bounded in the harness, and every actual child completed/was reaped in this run.

### Real HTTP/NATS boundary and readiness

An in-process NATS server and real loopback HTTP server, both owned by the test, carried the HTTP request trace through the stored job, NATS header/subscriber log and worker attempt logs. Idempotent replay retained the original job trace. A NATS outage degraded optional readiness without failing a committed job; PostgreSQL polling still completed it, and NATS reconnected.

A test-only TCP relay cut the application's database connections and refused new ones, with the fixed post-Dial cut check. Readiness became503 while liveness stayed200. The outage was held until the worker reported its lost database session; after restore, readiness recovered and a new leader event was required. Graceful stop logged ordered stop events. Startup and secret-file/origin/authorization boundary tests passed. No public readiness answer exposed dependency error details.

NATS is explicitly a best-effort notification hint after the database commit, not a transactional outbox or job authority. The tested post-commit notification failure never became a retryable HTTP failure for the committed job.

### Actual Python path and privacy

The Go subprocess path now runs the embedded launcher, exact embedded unchanged Financials source and embedded Telemetry source, with a validated trace argument and fixed credential-free process environment. It emits the parser report only on stdout. The actual-launcher regression proved report byte parity with Financials alone and correlated parser-event relay. Rejected input and non-event stderr were logged by outcome/omitted-byte count, not source/error text.

The relay validates the exact task version, non-zero lowercase span, same job trace, fixed message/level/status, timestamp, nonnegative duration, size and object shape. Short one-word task lookalikes, older versions, zero/uppercase spans, extra fields, malformed data and raw source-like text were rejected. The Python JSONL worker also passed trace/rejection/recovery tests. Ruff/mypy changes add launcher coverage; they do not weaken existing lint rules.

## Installed-artifact verification

The accepted package/output guard and toolchain scripts were statically read and byte-matched before execution: package.sh `86f967453764d8e3221d08d09c5dc63e1a511a1feee5efa193ed4f9d0be00ac3`, package-out.sh `a989bff85d54eeef3676f53bc581afcda80e28141c48defc463daf2faf1d4962`, and the previously verified uv0.13.4 toolchain helpers. I ran the ordinary non-deleting package command once into a new reviewer-owned path. No output-guard negative/destructive control or broad reproducibility repeat was run.

Package checks exited0, built both Go binaries and wheel, and installed the wheel offline in a new isolated venv. Wheel SHA-256 is `660709e843ca2b5beeec1a689b0dab519b8a572e844c47ac989c9abcc94313a2`. The wheel's telemetry.py, worker.py and financials.py bytes each exactly match the source manifest. An isolated import resolved telemetry from that fresh installed venv, rather than the repository. Its installed `research-worker` console accepted a valid traced request, rejected an unknown task containing a short owned fragment, and accepted the next valid request: three JSON responses and three safe correlated log lines; the fragment was omitted and the invalid request did not lose stream synchronization. This proves the new module/relative import/console path in the installed artifact, not merely source-only execution. The root parser launcher is intentionally embedded in the Go artifact, separate from the Python wheel.

## Dependency/security evidence

The actual pinned govulncheck run included tests and emitted **one raw module-only finding**, `GO-2026-5932`, for `golang.org/x/crypto v0.57.0`: the unmaintained `openpgp` family. Its trace contained only module/version, no affected imported package or reached symbol. A separate `go list -deps -test ./...` enumerated334 packages and imported none of `golang.org/x/crypto/openpgp` or its subpackages. The existing reachable-code gate correctly had zero actionable findings. No new waiver, exclusion, version rollback or security-setting change was introduced. This report does not describe the module as globally free of advisories.

The redacted committed-history scan retained only existing W-001/W-002 exact historical fingerprints and no open or unused finding. Unchanged Python dependency scans from the independently accepted S01-T03 baseline were not repeated merely to inflate coverage. Scanner results are time-specific; they are not a certification of every ignored/untracked file, unsupported platform or future dependency update. The full Go vulnerability JSON and package list are retained.

## Browser result and deliberate limitation

The realAPI/privatefrontend run passed four controlled-timing/report UI regressions, refusal when no SEC contact is configured, the actual paper lifecycle/rejection/pause/audit/no-broker test, and mobile/access-reset coverage: **7 passed**. The real SEC Apple test was **intentionally skipped** with TS_E2E_REAL_SEC=0. No new SEC/provider request was needed or made. This is an actual frontend/API regression run, not only a mocked UI or health check.

At the time of browser execution, the runner was a reviewed copied untracked harness, not part of exact55aefba Git contents. Final commit b0d6c21 now contains exactly those independently exercised runner bytes/mode. The final-commit re-review below closes that provenance gate without pretending that the broad suites were rerun on changed documentation.

## Preservation and retained resources

Final read-only checks: normal API/Vite PIDs78497/78502 remain; normal `/api/v1/health` returns200. Protected configuration values privately match the saved backup, are ignored and mode600. Normal paper MD5 stays `51285359c2d98893a4d47ead4c2fb4da`; same one-job aggregate `3c0d1aec0b92becfac3b1148ebaf1b54` and two stored originals remain. Canonical frontend9c75c91 is clean. Reviewer backend239 tracked files and private frontend24 tracked files retain exact bytes/modes and no remotes. The only backend untracked source is the documented test harness.

The three newly created runner containers (`tradestocker-t05-test-pg-ff23e6699922`, `tradestocker-t05-test-pg-07d85b326803`, `tradestocker-t05-e2e-pg-1a252c7a6d5d`) were each stopped by their own runner and removed through their own --rm configuration; absence was independently verified. Browser API child45083 is absent. Review ports18090/15173 are free. The earlier retained CI PostgreSQL container on18532 remains stopped/retained, and18532 is free. No existing volume/image/container cleanup or normal process restart occurred. Reviewer source clones, binaries, logs, browser artifacts and caches remain retained.

No push, hosted GitHub CI dispatch, paid service, external model/provider, Telegram delivery, broker action, production deployment, risk-limit change or live-lane activation occurred. Open identity/market-data/production/evaluation gates remain unresolved. This lifecycle work supplies no first real investment recommendation or claim of returns.

## Evidence paths

All are relative to this reviewer directory:

- `runtime-results.json`, `counts-and-resources.json`, `source-manifest.json`
- `isolated-five-packages.log`, `isolated-three-kill-tests.log`
- `focused-checks.json`, `python-research-tests.log`, `python-{lint,types}.log`, `go-{vet-changed,architecture}.log`
- `telemetry-prior-rematch.json` (links the earlier preserved passing/failing telemetry snapshots)
- `security-results.json`, `gitleaks-{history.json,history.log,gate.log}`, `govulncheck-with-tests.json`, `govulncheck-gate.log`
- `advisory-reachability.json`, `go-test-dependencies.txt`
- `installed-wheel-package.log`, `installed-wheel-results.json`, `installed-console.{stdout,stderr}.jsonl`, `package-wheel-0c74c6fef3/` and its retained check venv
- `browser-e2e.log`, `browser-e2e-results.json`, `e2e-cleanup.json`, `frontend-source-manifest.json`
- `normal-start.json`, `final-preservation.json`
- `clone/.local/t05-e2e.2QInkn/` API log, unique binary and browser artifacts

## Final exact-commit acceptance

Final backend `b0d6c2119d218de54a7de5bd98078bcfbfd9d780`, tree `127d50ff5bccddf68cbc1b70ca52f5a3bbf529fa`, is a direct child of runtime55aefba. The exact changed paths are `README.md`, `docs/evidence/lifecycle-tests.md`, and new `scripts/test-e2e-isolated.sh`. There are240 final tracked files:237 existing files retain byte/mode identity, two existing documentation files change, and the added executable runner is byte-identical to tested168f214e and mode100755. A complete Git-blob/file/mode manifest was verified; the private clone is clean and has no remotes. The original untracked tested runner was preserved separately before checkout made its matching copy tracked.

I read the complete documentation delta. It separates author and reviewer counts, preserves the author's genuine first failed outage assertion, states the precommit financial-kill limitation, describes NATS as a hint, documents proxy-cleared isolated E2E and records the same tested code/frontend source. Its installed-wheel row was still pending at the author's writing time; the completed independent artifact verification above is authoritative for this review. Its independent-review-pending label predates this report and is not a self-approval.

A new full committed-history redacted gitleaks scan at b0d6c21 scanned27 commits and passed the checked gate: **2 exact reviewed historical findings waived, zero open, zero unused; exit0**. No broader passing suite, dependency scan or browser run was repeated for this documentation/identical-runner delta. Final read-only preservation was repeated successfully, now against all240 final tracked backend files/modes and24 private frontend files; normal data/config/PIDs/health and the earlier stopped retained container remain unchanged, and review ports18090/15173/18532 remain free.

Acceptance maps directly to the original bounded criteria: (1) actual HTTP/NATS/job/parser correlation was exercised; (2) required PostgreSQL and optional NATS outages changed readiness distinctly from liveness with fixed nonsecret answers; (3) corrected redaction, actual subprocess safe-event/omission handling and installed JSONL telemetry were tested; (4) cancellation, graceful stop, synthetic process death/restart and precommit financial rollback/idempotent replay passed. Slow-provider bounded concurrency was demonstrated. The explicit ambiguous-COMMIT timing and production/multi-platform limits remain as stated above, rather than being converted into untested guarantees.

No unresolved blocking finding remains for this task's reviewed scope. Prior static reports, actual failing privacy probes and all successful/failing author/reviewer attempts remain retained; final acceptance does not erase them. Full hosted CI, real SEC, production deployment and future market/identity/live decisions remain outside this acceptance.

Final-delta assets: `final-commit.diff`, `final-commit-review.json`, `final-source-manifest.json`, `final-history-gitleaks.json` (redacted), `final-history-scan.log`, preserved `precommit-runner-168f214e.sh`, and updated `final-preservation.json`.

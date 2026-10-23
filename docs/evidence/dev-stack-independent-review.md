# Independent review — S01-T02 local containers and fixtures

Reviewer: **independent-review-agent**, separate from author **claude-code-agent**. Date: 2026-10-23. Decision: **ACCEPT S01-T02 at the exact bounded local checkpoint below**. No blocking product finding remains from this review. This does not accept production deployment, real data or advice, or later CI changes. The reviewer made no canonical product, Git or task-state changes.

## Exact checkpoint and method

Backend **9fa6c8a4cf1a909fcb0aba446317c34cc1c82941**, tree **ecb9c9e12f51ed912cb078c8b484fcc2c246028f**, parent **fa954355042ba10bd9fdd406e02053aa527ebb8b**. Frontend **9c75c9183df2ed9457012260bb904a9ea404a960** is unchanged. Both were archived into new reviewer-owned folders: **208 backend and 24 frontend tracked files**. The private frontend received its own copy of dependencies and writable browser/Vite outputs. No test used canonical source as an execution root.

Reviewed all changed entrypoint, Compose/Docker, secret, fixture/labeller, guard/client/smoke, configuration, Makefile and evidence paths. New checks focused on those changes; unchanged broad S01-T01 suites were not repeated. Existing S01-T01 acceptance is separate evidence. Later planning/CI/documentation commits are not substituted for this tested source.

Own Compose project: **tradestocker-dev-reviewd229fb7819**. Own API/frontend ports: **18480 / 15373**. Existing normal **8080 / 5173 / 55432** remained untouched. Docker Desktop on macOS arm64 was the actual runtime. No volume removal was authorized or performed.

## Independent execution results

These are this reviewer's actual runs, not author counts:

| Check | Result |
|---|---|
| `go vet ./cmd/platform-core ./cmd/market-ingestion` | PASS |
| `go test -race -count=1 -json ./cmd/platform-core ./cmd/market-ingestion` | **8 top-level PASS, 0 SKIP, 0 FAIL** |
| External secret overlay in each entrypoint, race enabled | **2 additional top-level PASS**, eight boundary cases per package: environment fallback, env/file conflict, preserved spaces/trailing CRLF, exact4096-byte limit, oversize, missing file, directory and empty-file behavior. Only own temporary secrets were used. |
| `python3 -B scripts/test_dev_stack.py` | **13 PASS**, fake repo and record-only Docker/API stubs |
| `python3 -B scripts/test_dev_stack_api.py` with ordinary override unset | **4 PASS**, owned fake API/proxy, redirect trap and pre-network missing-token negative cases |
| Pinned installed research Python, `-I -B fixtures/synthetic/test_fixtures.py` | **7 PASS**: deterministic generation, actual worker/labeller output, provenance, no grants/links, manifest checks |
| Exact-image build and `dev-stack.sh up --no-seed` | PASS: three healthy services, empty PAPER workspace, live advisory false, research unconfigured. Final stdout parsed as exactly one JSON summary. |
| Actual runtime identity and mounted secret reads | PASS: core **uid10001**, **CPython3.14.8**, own secret files readable with mode600; NATS **uid65534**; actual PostgreSQL server processes **uid70** |
| Container boundaries | PASS: core/NATS readonly root filesystems and ALL capabilities dropped; only core **127.0.0.1:18480** published; DB/NATS no host bindings |
| Private archived frontend browser tests against the actual container API | **7 PASS, 1 intentional external-SEC SKIP**, no test failure; browser test runner reports20.4s (whole command21.27s) |
| `dev-stack.sh up` after browser, exact saved research artifact comparison | PASS: adds only missing fixtures; two accounts, five SYNTHETIC quotes; saved JSONL exactly equals labelled golden fixture |
| Second `up`, then real ordinary `down` + `up` | PASS: new core container ID, unchanged paper document digest, research bytes and secret bytes; initialized data not reseeded |
| One bounded synthetic JetStream message across real down/up | PASS: same reviewer stream, subject, sequence1 and invented payload after restart; file-backed storage, max one message and4096bytes |
| Final ordinary down / preservation | PASS: own containers/networks removed, own ports free; all own volumes/image/secrets retained; source bytes/modes and normal data/processes preserved |

The exact persisted paper document MD5 was **eb96ac533f21798d9288642e0dd14d02** before the second up and after the real down/up. The saved synthetic research SHA256 was **e886cd1fa04d10f24c348c93f51c727d7bf0fb6c00c147c31924fc296d489adb**, matching the tracked golden byte for byte. The persistence interval had no pending recommendation expiry; due expiry was not mistaken for reseeding.

The JetStream check used only own stream **REVIEW_D229FB7819**, subject **REVIEW.D229FB7819**, and the text `SYNTHETIC independent local persistence probe; no market or account data`. It proves this local NATS storage survives container recreation. It does not prove application event integration, outbox/inbox behavior or delivery semantics; those remain later work.

## Original acceptance criteria

| Criterion | Assessment |
|---|---|
| One documented command starts dependencies with persistent volumes and health checks | PASS: exact `dev-stack.sh up` built and waited for healthy PostgreSQL, NATS and core. Empty first start, initialized second start and real down/up were exercised. |
| Non-root where feasible, secrets outside tracked files | PASS on this supported local Docker Desktop runtime: core10001/NATS65534/PostgreSQL70. PostgreSQL's official entrypoint and the one-shot volume ownership job retain their documented root requirements. Fresh per-project secret files are outside tracked source, mode700 parent/600 files, retained rather than overwritten. |
| Offline fixtures need no provider or LLM credentials | PASS: actual stack had empty SEC contact, no external provider/model/broker/Telegram configuration. The research job is network-none, and saved outputs explicitly synthetic without SEC source claims or public filing links. |
| Teardown non-destructive by default; volume removal explicit | PASS: ordinary down retained all three volumes. The explicit confirmed destroy branch was tested only against a record-only fake Docker stub. No real volume deletion occurred. |
| Startup/shutdown/restart persistence; no live delivery in defaults | PASS: actual lifecycle/browser/data checks above. Every summary remains PAPER/live false/research unconfigured. Core has an external-capable edge network; the no-delivery result relies on configuration and code, not a claim that the whole core is network-isolated. |

## Review history and honest limits

Earlier static findings were fixed before final acceptance: duplicate API-command argument, inherited proxy authorization leakage, missing config/dev.env ignore, port lower bound, synthetic outputs claiming official SEC evidence, and build stdout corrupting JSON summaries. The NATS command question was independently resolved from the exact pinned image's wrapper; a leading nats-server is correct for that wrapper.

Historical author evidence remains separate: the first author smoke failed on stdout contamination; its ordinary teardown retained volumes. The author disclosed running the changed fake-only13-test harness once before stdout-delta clearance, then again after. Static clearance is not retroactively attributed to that earlier execution. Reviewer static records snapshot2 and stdout delta are retained outside canonical source. The passing runtime results in this report are this reviewer's new exact-checkpoint run.

One external reviewer secret probe initially used SYNTHETIC in both its environment variable name and its fake secret-marker assertion, causing a false failure on a safe error naming the variable. Only the external probe identifier was corrected; no product change was needed. Original diagnostic output and passing rerun are retained.

Non-blocking test hygiene: the four API tests pass but emit Python ResourceWarnings for fake-server sockets because shutdown is called without server_close. The test process closes those ephemeral sockets on exit. This does not affect the actual stack or the acceptance above; explicitly closing fake servers would improve the tests.

Linux host secret UID mapping was **not** tested. Actual mode600 readability by uid10001 is demonstrated only on Docker Desktop for Mac. Image reproducibility is not claimed. Image metadata/history and reviewed source contained none of the actual normal or review secret values; this was not a forensic scan of every image-layer byte. No hosted-CI result is claimed here.

No broad unchanged paper/research suites were rerun, no fresh SEC retrieval or real provider/model request occurred, and no broker/order, Telegram, push, deployment or normal-app restart was performed. All Git access by this reviewer was read-only archive/source inspection; unrelated human push attribution is outside this product review. Provider rights/data quality, production identity/topology/budget, risk/evaluation and human live-lane activation gates remain unchanged.

## Preservation and retained resources

All **208 backend** and **24 frontend** archived tracked files retained their hashes and modes. The normal configuration privately matches its protected backup and remains ignored/mode600; actual private token/password/DSN values were absent from reviewed tracked source and image metadata/history, and were never printed.

Normal API **PID78497** and Vite **PID78502** stayed running. Normal paper MD5 remained **51285359c2d98893a4d47ead4c2fb4da**; research job aggregate remained **1:3c0d1aec0b92becfac3b1148ebaf1b54**, with two original SEC sources whose known SHA256s were rechecked. Cached original source files retained those hashes too.

No review containers or networks remain; ports18480/15373 are free. Deliberately retained, without deletion:

- `tradestocker-dev-reviewd229fb7819_natsdata`
- `tradestocker-dev-reviewd229fb7819_pgdata`
- `tradestocker-dev-reviewd229fb7819_research-results`
- Image `tradestocker-dev-app:tradestocker-dev-reviewd229fb7819`, ID `sha256:4ac70d3c949c6a3ad96ae88efdbaddb72d52e89d04bbc24a26d9ad0c0c1a2d9b`
- Own private secret folder `/Users/asaied/Documents/Codex/2026-10-23/referenced-chatgpt-conversation-this-is-an-4/work/reviewer-research/s01/s01t02-9fa6c8a-wbsgvaa6/backend/.local/dev-stack/tradestocker-dev-reviewd229fb7819` and the review evidence/build/browser files

Reviewer evidence root: `/Users/asaied/Documents/Codex/2026-10-23/referenced-chatgpt-conversation-this-is-an-4/work/reviewer-research/s01/s01t02-9fa6c8a-wbsgvaa6`. `review-results.json` records actual results, hashes, kept resources and preservation; raw per-stage stdout/stderr logs and the external file-boundary/JetStream probes are retained alongside the external runner. This report is provided externally for the coordinator's agreed canonical copy; no task status was changed by the reviewer.

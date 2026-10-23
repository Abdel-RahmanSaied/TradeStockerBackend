# Independent review — S01-T01 module boundaries and build commands

Reviewer: **independent-review-agent**, separate from author **claude-code-agent**. Date: 2026-10-23. Decision: **ACCEPT the bounded S01-T01 module/build checkpoint**. No remaining actionable product finding was identified in this review. No task state was edited by the reviewer.

## Exact scope

- Backend **491017206cda5775249a86df25cb97449c411e35**, tree **f4d7bc0553bc4d66ec16042cf531758fdf26b2a6**, parent **91776986c3d432f934679238eadb4d4d12230435**. All 41 changed paths were reviewed, including entrypoints, parser relocation, package/lock, dependency checks, launchers, packaging, contract, README, Makefile and CI.
- Frontend **9c75c9183df2ed9457012260bb904a9ea404a960**, unchanged and clean.
- Tests used an exact Git archive of all **186 tracked files**, plus a separate clean private Git checkout for build provenance. Canonical source, configuration, Git and planning state were read-only throughout. Later coordinator/author documentation commits do not substitute for this source checkpoint.
- Local environment: macOS arm64, **Go 1.26.10**, **CPython 3.14.8**, **uv 0.13.0**, existing PostgreSQL container with newly created reviewer-owned databases only. Go caches were outside canonical source. Python/uv assets were copied into the private archive; the package environment, wheel and fresh installed-wheel environment were created there. The five uv wheel pins independently match the official [PyPI release manifest](https://pypi.org/pypi/uv/0.13.0/json).

## Independent commands and results

All results below were executed by this reviewer against the exact checkpoint, rather than copied from author evidence. Full logs and private harnesses are retained in reviewer storage.

| Check | Actual result |
|---|---|
| Exact toolchain setup/version checks; `go vet ./...`; gofmt on tracked product Go files | PASS; Go 1.26.10, CPython 3.14.8, uv 0.13.0 |
| `TS_PYTHON=<supported private interpreter> TS_REQUIRE_PYTHON=1 go test -race -count=1 -json ./...` | **46 top-level PASS, 13 database-only SKIP, 0 FAIL**. This is 59 total outcomes, not 59 passes. No Python/parser test skipped. |
| Installed research environment: `python -I -B -m unittest discover -s research/tests -t research/tests` | **35 PASS**; includes four package-boundary checks and installed package subprocess behavior |
| `python3 -B scripts/test_package_out.py`; `scripts/test_toolchain_paths.py`; `scripts/test_disposable_api.py` | **16 / 6 / 10 PASS** |
| `docs/tools/test_planctl.py`; `test_install_docs.py`; planctl validate/render --check | **20 / 10 PASS**; plan validation and generated documents unchanged |
| Real PostgreSQL, `go test -race -count=1 -json ./internal/store` | **3 PASS, 0 SKIP** |
| Real PostgreSQL, supported Python required, `go test -race -count=1 -json ./internal/research` | **24 PASS, 0 SKIP**. Combined store/research: **27 PASS, 0 SKIP**. |
| External overlay: unchanged GET/HEAD under held row lock, four stale expiry contenders, financial replay/conflict/rollback/auth | **4 PASS, 0 SKIP** on their own database. Exactly one expiry persistence/refund/audit is still required. |
| Build both Go binaries; package artifact checks/checksums/startup refusals; fresh wheel installation and isolated imports/console | PASS |
| `check-reproducible.sh`, unique owned TMPDIR, two separate initially empty Go build caches | PASS: both packages byte-identical to each other and to the first archive package |
| Private clean-checkout package; `go version -m` on both binaries | PASS: both contain **vcs.revision=491017206cda5775249a86df25cb97449c411e35**, **vcs.modified=false**, Go1.26.10, trimpath, CGO disabled |
| External cached-real-SEC parity: actual Go-embedded parser versus fresh installed-wheel `research-worker`, fixed `as_of` | PASS: original hashes verified; outputs byte-identical; entry/backtest/write authority remain false |
| External streaming probe: nine malformed/envelope/unknown-task/live/deep/invalid-UTF/oversized inputs, each followed by a good line | PASS: each bad input rejected and each following good input recovered; stdout remains JSONL |
| Real API + archived frontend, isolated ports **18090 / 15173**, fresh token and disposable database, SEC disabled | **7 PASS, 1 intentional SEC SKIP, 0 unexpected, 0 flaky**; 19.386 seconds |

The existing race tests exercised subprocess environment/secret isolation, parser stdout/stderr limits, malformed reports, supported-interpreter refusal and safe provider refusals. The isolated API actually booted on the pinned interpreter for browser tests. None of these checks used the normal database as a test target.

The read overlay initially refused the reviewer database because my runner used a different private prefix than its explicit safety allowlist. I corrected only the external runner prefix and reran the four tests successfully; this was a harness setup diagnostic, not a product failure. An initial private Git provenance attempt recorded modified=true because the reviewer's .tools symlink was untracked (the directory-only ignore pattern did not cover that symlink). After a private Git-info exclusion for that reviewer symlink, the clean-checkout package produced the exact clean commit stamps above. Both diagnostic attempts are retained; no product change was needed.

## Criteria decision

| Original criterion | Independent assessment |
|---|---|
| Three independent entrypoints | PASS: both Go executables build/start with expected refusal boundaries; the wheel installs and its console script executes outside source. Go core continues to use the same embedded parser, while the standalone worker is independently usable. Queue integration is later work. |
| Core domain has no framework/network dependencies | PASS: current imports inspected; architecture scan and real-source/fake-graph negative cases pass. It catches unapproved nested modules rather than skipping arbitrary go.mod trees. |
| Build/test commands and supported toolchains documented | PASS: README, Makefile, exact Go selection, Python pin/startup enforcement, uv hashes/lock and worker protocol agree with exercised behavior. |
| No service per advisory role | PASS: one product Go module, one research package, two closed Go entrypoints and one Python console entrypoint. The explicitly approved compatibility spike remains separate. |
| CI builds both binaries and imports the Python package; dependency direction | Workflow wiring PASS by inspection; its build/install/import and direction checks passed locally. **No hosted GitHub CI execution was triggered or claimed.** |

## Preserved boundaries and resources

All 186 archived source files retained their bytes and modes after testing. Canonical non-document product files still match the exact source checkpoint. The canonical ignored configuration privately matches its protected backup and remains mode600; actual private token/password/database values were absent from reviewed tracked source. Configuration values were not printed.

Normal API PID **78497** and Vite PID **78502** remained running. Normal paper document MD5 stayed **51285359c2d98893a4d47ead4c2fb4da**; the normal research job aggregate stayed **1:3c0d1aec0b92becfac3b1148ebaf1b54**, with **2 original sources**. The frontend stayed exact and clean. All reviewer databases were removed, captured review child processes stopped, and review ports18090/15173 were confirmed free.

Cached originals used, with no new SEC retrieval:

- submissions SHA256 **47e4383b0133a4146cc631725f9aa28bac6d5679d504f735a78f1a1923c4b622**
- companyfacts SHA256 **73a86c6aedc31f77cac2ea4df5f80f0b3bd7e6eb58bb4e01444fbedf3afb9c43**

The archive package checksums below are reproducibility evidence for the archive context. Clean Git-stamped packages intentionally differ because they include Git provenance.

```text
60e390d99df7265865ec08fabc5e440cb506560c46e2f2f3068e51637756aadd  darwin-arm64/market-ingestion
b1a599c209cdc8e042fb6b7fb3e7f5e16577969c3c42121bb96fa703027cc092  darwin-arm64/platform-core
ea020bad7de73c44170be4d8d64d9cba62814557af11ac4d26703939ef2de5b5  tradestocker_research-0.1.0-py3-none-any.whl
```

## Limits

This accepts the local module/build task, not deployment or a market provider. Hosted Linux CI has not run; its workflow is present and reviewed, and equivalent commands ran locally. The supported runtime assets were reused privately, not freshly downloaded from scratch; official uv wheel hashes were independently verified. No external vulnerability scan was rerun in this review. No fresh SEC, paid/model, brokerage, Telegram, push or production action occurred.

The Python import guard is a maintainer source check, not a sandbox. The standalone CLI runs with its caller's OS permissions; Go's parser subprocess separately strips inherited credentials. Packaging is bounded to ordinary owned fresh/empty output paths; it is not a defense against concurrent adversarial filesystem swaps. No destructive negative control or legacy paper-read benchmark was executed.

Production identity, topology/spend and evaluation decisions remain open; original S00-T05 is not completed by this task. Licensed/entitled real price data, history quality, risk/evaluation gates and human lane activation remain prerequisites to actionable advice. This review supplies no evidence of trading performance or a real recommendation.

## Reproduction evidence location

Reviewer storage: `/Users/asaied/Documents/Codex/2026-10-23/referenced-chatgpt-conversation-this-is-an-4/work/reviewer-research/s01/checkpoint-4910172-hz9f6u28`. The main suite, continuation, clean-checkout provenance and browser runners are external to canonical source. `final-summary.json` and `final-preservation.json` capture the final counts and preservation checks; per-suite logs retain raw results. Reviewer-only setup diagnostics are kept as described above.

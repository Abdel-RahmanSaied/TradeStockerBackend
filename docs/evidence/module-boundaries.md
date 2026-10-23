# Task evidence — S01-T01

Create reconciled module boundaries and build commands.

| Field | Value |
|---|---|
| Task / owner / reviewer | `S01-T01` / `claude-code-agent` (session `2026-10-23-development-8da61cee`) / **independent exact-commit review pending** |
| Base | Backend `91776986c3d432f934679238eadb4d4d12230435` (recovered canonical state, revision 56) |
| Code | The checkpoint commit that adds this record; no frontend change |
| Environment | macOS 27.0.1 arm64; Go 1.26.10 (pinned with `GOTOOLCHAIN`); CPython 3.14.8 and uv 0.13.0 in `.tools/`; PostgreSQL 16.15 (normal container, disposable `tradestocker_test_*` databases only); Playwright 1.64.1 |
| Normal app | Not stopped or restarted. API PID 78497 and Vite PID 78502 ran throughout; paper document MD5 `51285359c2d98893a4d47ead4c2fb4da`, 1 SEC job and 2 sources before and after; 0 test databases left |

All results below were produced after the 2026-10-23 repository loss and recovery, from source rebuilt edit by edit. No result from before the loss is used.

## What exists now

| Entrypoint | Source | Role today |
|---|---|---|
| `platform-core` (Go) | `cmd/platform-core` | Local PAPER API, paper state, SEC research jobs. Refuses to start unless `TS_PYTHON` is exactly CPython 3.14.8 (`research.CheckPython`, before any database connection). |
| `market-ingestion` (Go) | `cmd/market-ingestion` | Loads synthetic fixtures through the local API only. Takes no arguments; `TS_MARKET_PROVIDER` may only be empty or `fixture`, so every real feed is refused until a provider is qualified. |
| `research-worker` (Python) | `research/src/tradestocker_research/worker.py` | JSON lines in, typed results out ([protocol](../../contracts/research-worker.md)). Console script of the `tradestocker-research` package. |

- **One Go module** (`tradestocker`). The spike keeps its own module at the only approved nested path, `spikes/s00-t05/go`.
- **One Python package** in `research/src/tradestocker_research` (`financials`, `worker`), with `pyproject.toml`, `.python-version` and `uv.lock`. The parser file kept its exact bytes (SHA-256 `567065a7…6336`); Go still embeds it and runs it as an isolated subprocess.
- **No service per advisory role.** The architecture test fixes the Go entrypoints to exactly these two, and the Python test fixes the console scripts to exactly `research-worker`.

## Acceptance criteria

| Criterion | Outcome | Evidence |
|---|---|---|
| Three independent entrypoints: platform-core, market-ingestion, research-worker | **Met (author check).** Both Go binaries build; the wheel installs into a new venv and `research-worker` answers. market-ingestion reaches no database, research or HTTP-server adapter and no third-party code (checked transitively). | `make build`; `scripts/package.sh` smoke checks; `TestModuleDependencyDirection` |
| Core domain code has no framework or network dependencies | **Met (author check).** `internal/paper` may import only pure standard-library packages; adapters may import domain and embedded assets, not each other; nothing imports an entrypoint or tool; every package must be classified; an unapproved nested module is reported and still scanned. | `tests/architecture` (4 tests): the real graph, a fake module with a real `net/http` import, 4 unapproved nested-module cases, 15 broken-graph cases |
| Document build/test commands and supported toolchains | **Met (author check).** README "Build and test"; `Makefile`; exact pins in `go.mod` + `GOTOOLCHAIN`, `research/.python-version`, `tools/uv-requirements.txt` (hash-checked). Python 3.9 is end-of-life and refused. | README; `TestPythonPinMatchesThePackage`; `TestCheckPythonAcceptsOnlyThePinnedRelease` |
| Do not create a service per advisory role | **Met (author check).** Closed entrypoint sets in Go and Python; a `cmd/technical-analyst` (also as a nested module) is caught. | `TestRulesCatchViolations`, `TestUnexpectedNestedModuleIsReported`, `test_only_one_console_entrypoint` |

Validation in the catalog asks to build both binaries and import the Python package **in CI**. The workflow does that (`make build`, `scripts/package.sh "$RUNNER_TEMP/package"`), but it has **not run on GitHub**: nothing was pushed. Its steps were run locally below.

## Commands and results (author run, final tree)

| Command | Result |
|---|---|
| `scripts/toolchain.sh` | uv 0.13.0, CPython 3.14.8, research package synced with `--locked` |
| `make check` | PASS: gofmt, go vet, planctl validate (17 sprints, 107 tasks), render --check (18 documents unchanged) |
| `make test` | PASS: 59 top-level Go test outcomes in 8 packages with `-race` and `TS_REQUIRE_PYTHON=1`: 46 PASS + 13 SKIP. The 13 skip because they need `TEST_DATABASE_URL`; one more database test shows PASS here only because its 2 database subtests skip. All 14 ran in `scripts/test-integration.sh` below. No test skipped for Python. (The author log has no per-test lines; this split is counted from the test source.) Python research package 35; launcher helpers 10 |
| `make test-package` | PASS: package output guard 16; toolchain path exchange 6 |
| `make arch` | PASS: Go architecture tests; Python import boundary 4 |
| `make build` | Both binaries built |
| `./scripts/test-integration.sh` | PASS: 27 top-level tests (store 3, research 24), 0 skipped, 0 failed |
| `TS_E2E_REAL_SEC=0 TS_E2E_API_PORT=18090 TS_E2E_FRONTEND_PORT=15173 ./scripts/test-e2e.sh` | PASS: 7 passed, 1 skipped (real-SEC test, by design). The new platform-core booted with `research_python="CPython 3.14.8"`. Ports free before and released after |
| Record-only negative control: `PACKAGE_OUT_LIBRARY=<mock outside repo> python3 -B scripts/test_package_out.py` | Exit 1 as required: 13 of the 14 guard tests fail (19 failures and 1 error, counting subtests). The 14th, an already-empty folder, rightly passes even with a guard that accepts everything; the 2 static no-delete tests pass. The mock recorded 22 lines, all test-owned temporary paths (one relative path given with the test folder as working folder, and one path split in two by its newline) |
| `./scripts/package.sh <new scratch folder>` | PASS: go1.26.10, `-trimpath`, CGO off; checksums, start-up refusals and the installed wheel's `research-worker` checked |
| `TMPDIR=<scratch> ./scripts/check-reproducible.sh` | PASS: two packages, each with an empty Go build cache, byte-identical, and identical to the run above |
| `make package` | PASS into a new, Git-ignored `dist/`, identical to the runs above |
| `python3 -B docs/tools/test_planctl.py`; `python3 -B test_install_docs.py` | PASS: 20; 10 |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | No vulnerabilities found |

Package contents on this tree (uncommitted working tree, so binaries record `vcs.modified=true`):

```text
ce2d96b1230b7c8f189756be50419ba98bd4f3ac758c740e795ff7305aaff67e  darwin-arm64/market-ingestion
3acfc37497dd7d152c85be35dd907eaa49b4dabcc334c0b7de7845e26ffa5419  darwin-arm64/platform-core
ea020bad7de73c44170be4d8d64d9cba62814557af11ac4d26703939ef2de5b5  tradestocker_research-0.1.0-py3-none-any.whl
```

## Behavior preserved on the supported interpreter

The real SEC originals preserved during recovery (AAPL submissions `47e4383b…`, companyfacts `73a86c6a…`, the same hashes as the stored sources) were run through the parser with the stored job's `as_of`. The reports from CPython 3.9.6, CPython 3.14.8 and the installed `research-worker` are byte-identical (SHA-256 `2c54101665b5bda724404a07d8a71c6135160a881a4a44b4a3b3538dcde6577d`; 5 periods, `INSUFFICIENT_DATA`, not eligible for entry or backtest). The stored report itself was not re-read from the database in this run.

The 35 research package tests also run on 3.9.6, except 2 that start the installed package (not installed for 3.9) and 1 that needs `tomllib`. 3.9 is not supported.

## Safety review of the packaging scripts

Packaging and its tests ran only after a static review of the exact file hashes (Codex record `work/reviewer-research/s01/packaging-static-clearance-final.json`). Review findings fixed before any run:

- The output guard never deletes: a new or empty folder only; the repository, HOME and `/` and their parent folders refused regardless of content; `.git`, symlinks, files, missing parents, unlistable or unwritable folders and any newline in OUT, REPO or HOME refused; emptiness counted per entry, not from file names.
- No `eval` anywhere in `scripts/` or the `Makefile`: paths pass by plain assignment (`TS_PYTHON=$(scripts/toolchain.sh python)`), tested with a checkout path holding an apostrophe and a harmless marker.
- The end-to-end refusal test runs a copy of `package.sh` in a fake repository with record-only stubs, so even a broken guard reaches only test-owned paths.
- The architecture scan accepts a nested `go.mod` only at the approved spike path.

## Limits and follow-ups

- `make build` replaced the ignored `bin/platform-core` that recovery had rebuilt from accepted source. The running API was not affected (it runs the earlier, deleted file). An accepted-source build of `9177698` is ready outside the repository for the coordinator to put back before any restart; restarting with the new binary needs `TS_PYTHON` from `scripts/toolchain.sh`.
- platform-core runs the parser through the embedded source, not through the research-worker CLI; a queue between them is S02-T04.
- `scripts/paper-read-bench.sh` still has its accepted `rm -rf` of the fixed `.local/paper-read-bench` folder; unchanged here.
- CI has not run on GitHub. S01-T03 owns the full CI baseline (lint/type checks, scans, drift tests).
- Production identity, deployment and capacity (S00-T05, D-05/D-06/D-07) are not touched.

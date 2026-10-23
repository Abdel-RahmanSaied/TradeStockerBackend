# Task evidence — S01-T03

Implement the CI baseline and documentation drift checks.

| Field | Value |
|---|---|
| Task / owner / reviewer | `S01-T03` / `claude-code-agent` (session `2026-10-23-development-8da61cee`) / **independent review pending** |
| Base | Backend `0515eee271613a10311126b0065fd1fb1ebb06aa` (state 66) |
| Code | Security fix `b552c499c2d240194ddbd2cc1354f8d0ac5a0002` (uv 0.13.4), then the checkpoint commit that adds this record |
| Environment | macOS 27.0.1 arm64; Go 1.26.10; CPython 3.14.8; uv 0.13.4; ruff 0.16.10; mypy 2.4.0; gitleaks v8.30.1; govulncheck v1.9.0 |
| Hosted CI | **Not run.** Nothing was pushed. The workflow runs the same `make ci-*` targets that ran locally below; its GitHub runner behaviour (for example the runner's `python3 -m venv`) is not verified. |

## What exists now

| Part | What it does |
|---|---|
| `.github/workflows/ci.yml` | Jobs `go`, `python`, `docs`, `security`, `package`; each sets up the pinned toolchains and runs one `make ci-*` target, then writes the report to the job summary. Read-only `contents` permission, `persist-credentials: false`, no secrets, `pull_request` (never `pull_request_target`), actions pinned by commit, PostgreSQL service pinned by digest, full history only in `security`. |
| `make ci-go` | gofmt (names the files), `go vet`, `go test -race -json`. PostgreSQL tests run when `TEST_DATABASE_URL` is set (CI sets it to its service database); otherwise the report lists each one as skipped with its reason. |
| `make ci-python` | ruff (`ruff.toml`: rules E4, E7, E9, F, listed explicitly) and mypy (`mypy.ini`: `check_untyped_defs` everywhere except the reviewed SEC parser, which is checked in default mode so its bytes stay as reviewed), then every Python suite. Tools come from the locked, hash-pinned `dev` group in `research/pyproject.toml` with a cooldown date; they are not in the wheel. |
| `make ci-docs` | `planctl validate` (plan, task catalog and state schema), `render --check` (generated documents), planctl and installer tests. |
| `make ci-security` | Waiver check; gitleaks over the committed history (`--redact`; only Git objects are read, so ignored local files are never scanned); govulncheck (only vulnerable code the module actually reaches counts); OSV for every locked PyPI package and the uv pin. `scripts/scan_checks.py` fails on any open finding and on any invalid, expired, self-reviewed or unused waiver. |
| `make ci-package` | Both binaries, `scripts/package.sh` into a new folder, and the two-build reproducibility check. |
| `make ci-report` | `scripts/ci_suite.py report`: every suite with its real counts (from `go test -json` events and unittest summaries; a suite with no counts fails), the skipped Go tests with their reasons, and the suites CI never runs. |
| `security/scan-waivers.json` | One waiver, W-001: an exact gitleaks fingerprint, a false positive on the public SHA-256 hashes of the SEC source files. Owner `claude-code-agent`, reviewed by `codex-reviewer` on 2026-10-23 (who checked privately that no protected credential occurs there), expires 2026-11-22. No rule, file or history exclusion. |

## Finding fixed on the way: vulnerable uv

The first full run failed `ci-security`: OSV reported GHSA-hq85-5cpj-mr62 / PYSEC-2026-4321 (critical) for the pinned uv 0.13.0. Its bundled CA store trusted the emSign ECC Root CA - G3, whose intermediate keys are compromised. Commit `b552c499` pins uv 0.13.4, the first fixed version (released 2026-10-20, a documented exception to the 14-day cooldown), installs each uv version into its own `.tools/uv-<version>/` folder and checks the installed version. The coordinator independently checked the five official hashes, a fresh install, the locked sync, the path tests and the wheel. The wheel is now built by uv_build 0.13.4, so its bytes (and hash) changed: `fcbfaa74…`. The old `.tools/uv-venv` (0.13.0) was left in place, not deleted.

## Acceptance criteria

| Criterion | Outcome (author run) | Evidence |
|---|---|---|
| Go format/vet/race tests and Python lint/type/unit tests run with pinned tooling | **Met locally.** All pass in `make ci`. Go from `go.mod` via `GOTOOLCHAIN`; Python tools from the locked group; the lint run first found and fixed 3 real issues in tests and 3 in the new CI scripts. | `make ci` below |
| Dependency/secret scans are enabled; explicit reviewed waivers have owners and expiry | **Met locally.** gitleaks, govulncheck and OSV run in `make ci-security`; W-001 is the only waiver. The OSV scan found a real critical advisory, which was fixed, not waived. | `test_scan_checks.py` (9 tests: unreviewed, self-reviewed, expired, future-dated, duplicate, unused and prefix waivers all refused) |
| planctl validate and render --check run on docs changes | **Met locally.** Both run in `make ci-docs`, on every push and pull request. | `make ci` below |
| CI reports test counts and unrun integration suites honestly | **Met locally.** See the report below; 13 PostgreSQL tests are reported as skipped locally with their reason, and 5 suites as never run by CI. | `test_ci_suite.py` (6 tests) |
| Validation: intentional test, schema and docs drift failures in an isolated branch | **Not yet run.** `scripts/ci_negative_controls.sh` (10 breaks, each on its own branch of a private clone with isolated Git settings, each requiring its own diagnosis) is waiting for static review. | — |

## Local `make ci` (third run, final tree before this commit)

All targets exit 0. Scans: waivers 1 valid; gitleaks 1 finding, 1 waived, 0 open; govulncheck 0; OSV 0 of 8 packages. Packaging byte-identical over two builds.

| Suite | Ran | Skipped | Failed |
|---|---|---|---|
| go (top-level tests, `-race`) | 61 | 13 (need `TEST_DATABASE_URL`) | 0 |
| research | 35 | 0 | 0 |
| fixtures | 7 | 0 | 0 |
| launcher-helpers | 10 | 0 | 0 |
| package-guard | 16 | 0 | 0 |
| toolchain-paths | 6 | 0 | 0 |
| dev-stack | 13 | 0 | 0 |
| dev-stack-api | 4 | 0 | 0 |
| scan-checks | 9 | 0 | 0 |
| ci-suite | 6 | 0 | 0 |
| planctl | 20 | 0 | 0 |
| install-docs | 10 | 0 | 0 |

Earlier runs, kept honestly: run 1 stopped at ruff (3 findings in the new CI scripts, fixed); run 2 stopped at OSV (the uv advisory, fixed in `b552c499`).

## Limits

- Hosted CI has never run; the workflow is checked only by running its make targets locally.
- Lint uses a small, explicit rule set; mypy checks the SEC parser in default mode only.
- Frontend checks live in the frontend repository and are not part of this workflow.
- The 13 PostgreSQL tests did not run in the local `make ci`; they run with `scripts/test-integration.sh` and, in CI, against the service database.

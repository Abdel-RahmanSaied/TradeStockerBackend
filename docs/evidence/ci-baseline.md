# Task evidence — S01-T03

Implement the CI baseline and documentation drift checks.

| Field | Value |
|---|---|
| Task / owner / reviewer | `S01-T03` / `claude-code-agent` (session `2026-10-23-development-8da61cee`) / **independent review pending** |
| Base | Backend `0515eee271613a10311126b0065fd1fb1ebb06aa` (state 66) |
| Code | Security fix `b552c499c2d240194ddbd2cc1354f8d0ac5a0002` (uv 0.13.4); CI baseline `7ed1c760dd481d1b9401cd75d49bd7c392fdb634` (its own `ci-security` fails, see the negative controls); test-value fix `01238982315b17d7162638e640d69736f5782c73`; W-002 review `19da95f6e2e7852365c5451b90e9e047241d6d17`; this record updated in a later docs-only commit |
| Environment | macOS 27.0.1 arm64; Go 1.26.10; CPython 3.14.8; uv 0.13.4; ruff 0.16.10; mypy 2.4.0; gitleaks v8.30.1; govulncheck v1.9.0 |
| Hosted CI | **Not run.** Nothing was pushed. The workflow runs the same `make ci-*` targets that ran locally below; its GitHub runner behaviour (for example the runner's `python3 -m venv`) is not verified. |

## What exists now

| Part | What it does |
|---|---|
| `.github/workflows/ci.yml` | Jobs `go`, `python`, `docs`, `security`, `package`; each sets up the pinned toolchains and runs one `make ci-*` target. The `go`, `python` and `docs` jobs then write the suite report to the job summary (`make ci-report`); `security` and `package` have no report step and show only their own output. Read-only `contents` permission, `persist-credentials: false`, no secrets, `pull_request` (never `pull_request_target`), actions pinned by commit, PostgreSQL service pinned by digest, full history only in `security`. |
| `make ci-go` | gofmt (names the files), `go vet`, `go test -race -json`. PostgreSQL tests run when `TEST_DATABASE_URL` is set (CI sets it to its service database); otherwise the report lists each one as skipped with its reason. |
| `make ci-python` | ruff (`ruff.toml`: rules E4, E7, E9, F, listed explicitly) and mypy (`mypy.ini`: `check_untyped_defs` everywhere except the reviewed SEC parser, which is checked in default mode so its bytes stay as reviewed), then every Python suite. Tools come from the locked, hash-pinned `dev` group in `research/pyproject.toml` with a cooldown date; they are not in the wheel. |
| `make ci-docs` | `planctl validate` (plan, task catalog and state schema), `render --check` (generated documents), planctl and installer tests. |
| `make ci-security` | Waiver check; gitleaks over the committed history (`--redact`; only Git objects are read, so ignored local files are never scanned); govulncheck (only vulnerable code the module actually reaches counts); OSV for every locked PyPI package and the uv pin. `scripts/scan_checks.py` fails on any open finding and on any invalid, expired, self-reviewed or unused waiver. |
| `make ci-package` | Both binaries, `scripts/package.sh` into a new folder, and the two-build reproducibility check. |
| `make ci-report` | `scripts/ci_suite.py report`: every suite with its real counts (from `go test -json` events and unittest summaries; a suite with no counts fails), the skipped Go tests with their reasons, and the suites CI never runs. |
| `security/scan-waivers.json` | Two reviewed waivers, each for one exact gitleaks fingerprint, both owned by `claude-code-agent` and expiring 2026-11-22. No rule, file or history exclusion. **W-001**: a false positive on the public SHA-256 hashes of the SEC source files; reviewed by `codex-reviewer` on 2026-10-23, who checked privately that no protected credential occurs there. **W-002** (added in `01238982`, review recorded in `19da95f6`): the historical finding of an invented test value in `7ed1c760`; reviewed by `independent_review` on 2026-10-23 (see the negative controls). |

## Finding fixed on the way: vulnerable uv

The second full `make ci` run (the first had stopped at ruff on 3 lint findings in the new CI scripts) failed `ci-security`: OSV reported GHSA-hq85-5cpj-mr62 / PYSEC-2026-4321 (critical) for the pinned uv 0.13.0. Its bundled CA store trusted the emSign ECC Root CA - G3, whose intermediate keys are compromised. Commit `b552c499` pins uv 0.13.4, the first fixed version (released 2026-10-20, a documented exception to the 14-day cooldown), installs each uv version into its own `.tools/uv-<version>/` folder and checks the installed version. The coordinator checked the five wheel hashes against PyPI. Separately, the independent reviewer ran a fresh install, CPython 3.14.8, the locked sync, the toolchain path tests and a wheel install, import and console check in its own snapshot. The wheel is now built by uv_build 0.13.4, so its bytes (and hash) changed: `fcbfaa74…`. The old `.tools/uv-venv` (0.13.0) was left in place, not deleted.

## Acceptance criteria

Every result in this record is an **author run**. The independent reviewer's own runs and counts are in its separate report and are not repeated or claimed here.

| Criterion | Outcome (author run) | Evidence |
|---|---|---|
| Go format/vet/race tests and Python lint/type/unit tests run with pinned tooling | **Met locally.** All pass in `make ci`. Go from `go.mod` via `GOTOOLCHAIN`; Python tools from the locked group; the lint run first found and fixed 3 real issues in tests and 3 in the new CI scripts. | `make ci` below |
| Dependency/secret scans are enabled; explicit reviewed waivers have owners and expiry | **Met locally.** gitleaks, govulncheck and OSV run in `make ci-security`, with the two reviewed waivers W-001 and W-002. The OSV scan found a real critical advisory, which was fixed, not waived; the negative-control baseline found a committed key-shaped test value, which was replaced, and only its historical fingerprint is waived. | `test_scan_checks.py` (9 tests: unreviewed, self-reviewed, expired, future-dated, duplicate, unused and prefix waivers all refused) |
| planctl validate and render --check run on docs changes | **Met locally.** Both run in `make ci-docs`, on every push and pull request. | `make ci` below |
| CI reports test counts and unrun integration suites honestly | **Met locally.** See the report below; 13 PostgreSQL tests are reported as skipped locally with their reason, and 5 suites as never run by CI. | `test_ci_suite.py` (6 tests) |
| Validation: intentional test, schema and docs drift failures in an isolated branch | **Met (author run on `749877ca`).** Baseline green on all four targets, and all 10 breaks failed their target with their own diagnosis. The earlier run on `7ed1c760` FAILED: its baseline `ci-security` failed on a real finding in that commit (below). `7ed1c760` is not a green CI commit. | Negative controls below |

## Local `make ci` (author, third run, on the tree that became `7ed1c760`)

This run predates W-002: the key-shaped test value was not yet committed, so the history scan could not see it. On `749877ca`, `make ci-python` and `make ci-security` passed again with gitleaks at 2 findings, 2 waived, 0 open.

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

## Negative controls (author run on `749877ca`): PASS

`scripts/ci_negative_controls.sh 749877cac0da91ba3f2199a25f31d14408273b04` (unchanged script, after static delta clearance), in a new folder in the author's scratch space. Baseline: `ci-go`, `ci-python`, `ci-docs` and `ci-security` all exit 0; gitleaks 2 findings, 2 waived (W-001, W-002), 0 open. All 10 breaks failed with the same diagnoses as in the table below, except that the secret break's commit differs. The clone had 0 remotes and an empty hooks folder; the real repository stayed unchanged. Clone and logs are kept.

## Negative controls (author run on `7ed1c760`): FAILED

`scripts/ci_negative_controls.sh 7ed1c760dd481d1b9401cd75d49bd7c392fdb634`, after static execution clearance, with `TMPDIR` a new folder in the author's scratch space. The clone had no remote; Git there ran with private settings only (no system or global configuration, empty template and hooks folder, no signing, no credential helper); the real repository stayed unchanged. The clone, every log and `results.tsv` are kept.

| Control | Target | Exit | Matched diagnosis |
|---|---|---|---|
| baseline | ci-go, ci-python, ci-docs | 0 | (pass) |
| **baseline** | **ci-security** | **2** | **FINDING: aws-access-token in scripts/test_scan_checks.py line 98 (commit 7ed1c760)** |
| failing Go test | ci-go | 2 | `service_test.go:450: deliberate negative control` |
| gofmt drift | ci-go | 2 | `gofmt needed: internal/paper/model.go` |
| failing Python test | ci-python | 2 | `self.fail("deliberate negative control")` |
| unused import | ci-python | 2 | `F401 [*] 'os' imported but unused` |
| type error | ci-python | 2 | `label.py:88: error: Incompatible types in assignment` |
| fixture drift | ci-python | 2 | `FAIL: test_manifest_matches_every_fixture` |
| invalid state status | ci-docs | 2 | `ERROR: S01-T03: invalid task status MAYBE` |
| generated-doc drift | ci-docs | 2 | `ERROR: Generated docs drift: sprints/S00.md` |
| invented GitHub token | ci-security | 2 | `FINDING: github-pat in docs/evidence/README.md` |
| expired waiver | ci-security | 2 | `INVALID WAIVER: W-001: expired on 2026-01-01` |

**The baseline failure is a real defect, found by the control.** The scan-gate test in `7ed1c760` used an invented AWS-key-shaped string to check that unredacted reports are refused, and gitleaks correctly flagged it in history. The earlier local `make ci` passed only because that file was not yet committed: `ci-security` scans committed history, not the working tree. The independent reviewer found the same failure in its own run. Commit `01238982` replaces the value with a low-entropy placeholder and lists the historical finding as W-002; the independent reviewer approved that exact fingerprint only, recorded in `19da95f6`.

## Limits

- Hosted CI has never run; the workflow is checked only by running its make targets locally.
- The local secret scan reads committed history only, so a new file is scanned only once it is committed (the W-002 case).
- Lint uses a small, explicit rule set; mypy checks the SEC parser in default mode only.
- Frontend checks live in the frontend repository and are not part of this workflow.
- The 13 PostgreSQL tests did not run in the local `make ci`; they run with `scripts/test-integration.sh` and, in CI, against the service database.

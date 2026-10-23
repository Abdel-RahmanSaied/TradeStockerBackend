# S01-T03 independent CI review

Reviewer: `independent_review`. Date: 2026-10-23. Product author: `claude-code-agent`. This report records my own checks; author results are not counted as independent results.

## Decision

**ACCEPT the implemented S01-T03 CI baseline at backend `8c7d7fc9b244c9f9b273574fc579e0f74c6983c4`, tree `242910e59cbb2239e037cf1bd95c22c68ab15787`.** Its product source is identical to the fully checked 749877c baseline; the final commit changes only author evidence and received a new committed-history secret scan. All four bounded acceptance criteria and the intentional-failure validation were demonstrated locally. There is no remaining blocking finding. This is acceptance of CI configuration and locally executed targets, not a claim that GitHub-hosted CI ran.

Frontend remains `9c75c9183df2ed9457012260bb904a9ea404a960`. No frontend or container/browser suite was rerun for this CI-only change. Prior S01-T02 acceptance remains separate.

## Exact source and corrections

The initial reviewed CI checkpoint was `7ed1c760dd481d1b9401cd75d49bd7c392fdb634`. My first `make ci` correctly failed with exit 2 at the committed-history secret gate. The new redaction unit test itself contained a synthetic key-shaped string. This was a real CI baseline failure, rather than evidence of an exposed AWS credential. The original failed run and redacted scanner output remain preserved.

The corrected history is:

- `01238982315b17d7162638e640d69736f5782c73`: neutral test placeholder and proposed exact historical waiver.
- `19da95f6e2e7852365c5451b90e9e047241d6d17`: W-002 approved-review metadata.
- `749877cac0da91ba3f2199a25f31d14408273b04`: evidence corrections only.

Exactly three paths changed from 7ed1c76: `scripts/test_scan_checks.py`, `security/scan-waivers.json`, and `docs/evidence/ci-baseline.md`. All other reviewed source bytes and modes, including the workflow, Makefile and negative-control script, matched. Thus successful unchanged stages were reused, while the changed unit/lint and previously blocked security/package/control stages were run at 749877c. The fixed source manifest contains 220 tracked files; their bytes and executable modes remained unchanged after testing. Both reviewer clones were clean and had no remotes.

The preceding uv security correction `b552c499c2d240194ddbd2cc1354f8d0ac5a0002` was independently matched byte-for-byte to the previously tested six-file snapshot. A fresh hash-checked uv 0.13.4 installation, CPython 3.14.8, locked sync, six path tests, wheel build, fresh installation/import and console entrypoints had passed. The old uv advisory was corrected, not waived. Official five-distribution hash verification was the coordinator's separate work; runtime checks were mine.

## Isolation and commands

All work was performed in new reviewer-owned storage. History-preserving clones used private HOME/XDG/config/template/hooks, no prompts or credential helper, disabled signing in the child environment, `--no-checkout`, and origin removal before checkout. No canonical/global Git settings were changed. There was no push or GitHub workflow dispatch. The controls made branch commits only in their own no-remote clone. No deletion control or legacy benchmark ran.

Actual independently executed targets included:

- Initial exact 7ed1c76: `make ci CI_OUT=<fresh reviewer directory>`, with `TEST_DATABASE_URL` absent; successful Go/Python/docs stages retained even though the later secret gate failed.
- Corrected exact 749877c: `sh scripts/toolchain.sh`, pinned ruff, `python -I -B scripts/test_scan_checks.py`, `make ci-security`, `make ci-package`, and `sh scripts/ci_negative_controls.sh 749877cac0da91ba3f2199a25f31d14408273b04`.
- `scripts/ci_suite.py report` over only actual successful fixed-baseline records; deliberately broken branch records were excluded.
- Separate actual PostgreSQL full Go race run on unchanged Go source using a dedicated reviewer PostgreSQL 16.15 container on loopback 18532 with random throwaway configuration and tmpfs storage. The normal 55432 cluster was never a test target.

Plain reviewer-owned paths were used for `TMPDIR` and `CI_OUT`; this review does not establish an arbitrary-path interface guarantee.

## Results

| Check | Independent result |
|---|---|
| Go formatting, vet and full race, no integration database | 61 top-level outcomes: **48 PASS, 13 expected DB-only SKIP**, zero failures |
| Full Go race with actual dedicated PostgreSQL | **61 PASS, zero SKIP**, zero failures |
| Python lint / type checks | Pinned ruff and mypy PASS |
| Python and documentation/helper suites | **136 PASS**, zero SKIP/failure/error |
| Changed scan-check units at fixed commit | **9 PASS**, zero SKIP/failure/error |
| planctl validate and generated-doc `render --check` | PASS |
| Committed-history gitleaks | 2 findings, **2 exact reviewed waivers**, zero open, zero unused |
| govulncheck | Zero findings |
| OSV lock/tool scan | 8 PyPI packages checked, zero findings |
| Package and wheel checks | PASS |
| Two packages built with separate empty Go build caches | Byte-identical Go binaries and wheel |
| Fixed-commit control baselines | **4 PASS** |
| Deliberate breaks in separate private branches | **10 failed as required, each with its own matching diagnosis** |

The 136 Python/docs outcomes comprise research 35, synthetic fixtures 7, launcher helpers 10, package guard 16, toolchain paths 6, development-stack helpers 13, API helper 4, scan checks 9, CI-suite helpers 6, planctl 20, and install/docs 10. The rerun scan nine are the same suite and are not added again to that total. The fixed-control baselines rerun these suites; they are corroborating runs, not additional unique-test counts.

The separate database run initially used a reviewer database name without the mandatory disposable prefix. Existing guards refused it. That first harness-setup refusal (47 pass, 14 guard failures) was retained, then only the reviewer database name was corrected to the required `tradestocker_test_` prefix. The completed run passed all 61 with no skips. No product fix was made or needed for that setup diagnostic.

The fixed report enumerated all 13 DB-only skips with reasons, and explicitly listed browser E2E, development-stack smoke, real SEC, performance benchmark and sibling frontend suites as not run by CI. The configured hosted Go job supplies a disposable PostgreSQL service, while the independent dedicated-database run demonstrated those actual DB paths locally.

### Intentional-failure evidence

All four fixed baselines (`ci-go`, `ci-python`, `ci-docs`, `ci-security`) returned 0. Each following branch returned target exit 2 and matched its specific expected diagnostic:

| Break | Target | Observed reason |
|---|---|---|
| Go failing test | ci-go | Deliberate test assertion |
| Go formatting | ci-go | Exact changed Go file needs gofmt |
| Python failing test | ci-python | Deliberate unit assertion |
| Python lint | ci-python | Ruff F401 |
| Python type | ci-python | Incompatible assignment |
| Synthetic fixture drift | ci-python | Fixture manifest mismatch |
| State schema | ci-docs | S01-T03 invalid status MAYBE |
| Generated document drift | ci-docs | Generated S00 document drift |
| Synthetic secret | ci-security | Open gitleaks finding |
| Expired waiver | ci-security | Expired W-001 |

The control clone was returned to the exact fixed baseline and was clean, no-remote, with private hooks after execution. The author's earlier ten controls are not substituted for this independent run.

### Reviewed exceptions

W-001 matches one historical public-SHA false positive, with coordinator review `codex-reviewer`. W-002 matches only:

`7ed1c760dd481d1b9401cd75d49bd7c392fdb634:scripts/test_scan_checks.py:aws-access-token:98`

I independently inspected that historical Git blob and AST/local redaction test provenance, verified the neutral replacement, and approved that exact historical fingerprint. It is a synthetic fixture with no AWS/network use, not a general scanner exclusion. W-002 owner is `claude-code-agent`, reviewer `independent_review`, reviewed 2026-10-23, expires 2026-11-22; those values match my actual review. Both current exceptions have explicit expiry. Invalid, expired, missing and unused waiver behavior is tested; the expired-waiver control failed as required.

## Acceptance criteria mapping

1. Go format/vet/race and Python lint/type/unit are wired and actually passed with pinned tools. Separate real PostgreSQL execution covers the otherwise skipped database paths.
2. Dependency and history secret scans are enabled and passed with exactly reviewed scoped exceptions. No uv vulnerability exception was used.
3. Validation and generated-doc checks run in the docs job; real schema and generated-document mutations were independently rejected.
4. Counts distinguish ran/pass/skip/fail/error and list out-of-CI suites. Least-privilege workflow permissions are `contents: read`; checkout credentials are not persisted, action references are SHA-pinned, no pull-request secret is required, and no privileged pull_request_target workflow is used.

## Preservation and limits

Final read-only checks confirmed normal API/Vite PIDs 78497/78502 remain; protected configuration values still match privately, remain ignored and mode 600. Normal paper document MD5 is unchanged at `51285359c2d98893a4d47ead4c2fb4da`; the same one-job aggregate and two stored source originals remain unchanged. Frontend 9c75c91 is clean. No normal data, application process, secret or canonical source was mutated by this review.

Reviewer PostgreSQL container `tradestocker-review-ci-pg-1c0e522697` is stopped and retained; port 18532 is closed. Reviewer packages, caches, clones, control branches/logs and tmpfs-container definition are retained. No Docker removal/prune/down-v was performed.

Limits: local macOS arm64 checks establish target behavior, not a hosted Linux GitHub run. No fresh SEC/provider, model, broker, Telegram or paid-service call was made. No frontend/browser/stack rerun was needed for unchanged financial/runtime product code. Secret scanning covers the committed history available after the documented recovery, not a certification of untracked drafts, ignored files, unrecovered historical objects or all future secrets. Scanner results are time-specific. This task does not qualify market data, establish an investment edge, resolve open identity/production gates or enable trading.

## Final documentation commit re-review

Final commit `8c7d7fc9b244c9f9b273574fc579e0f74c6983c4`, tree `242910e59cbb2239e037cf1bd95c22c68ab15787`, is a direct child of tested 749877c. Its only changed file is `docs/evidence/ci-baseline.md` (SHA-256 `c0c247b97fa7a24fb8e281b3107773364d40079d52ece789fcf27e46e2c26c40`). I read the complete diff: it fixes the stale single-waiver statement, clearly attributes author runs, labels the old precommit/history-scan timing, retains the failed 7ed1c76 baseline, and records the author's corrected control run without claiming it as independent evidence.

The other 219 tracked file bytes and modes match tested 749877c exactly. No broad suite was rerun or attributed to unexecuted changed product code. I independently scanned the final committed history with pinned gitleaks and the unchanged waiver gate: **24 commits scanned; 2 findings, 2 exact reviewed waivers, zero open, zero unused; exit 0**. Final private clone is clean and has no remotes. Repeated final preservation checks at this boundary again passed for all 220 final-source files/modes, frontend, normal PIDs/configuration/data and stopped retained reviewer PostgreSQL resource. This closes the documentation follow-up with no open finding.

## Retained reviewer assets

All paths below are relative to this report's reviewer directory:

- `fixed-ci-results.json`, `fixed-source-manifest.json`, `fixed-delta-clearance.json`
- `fixed-{toolchain,ruff,scan-units,security,package,negative-controls,count-report}.log`
- `fixed-security-out/` redacted scanner output; `fixed-package-out/` packages and reproducibility evidence
- `fixed-baseline-report/` actual count JSON records
- `fixed-isolation/tmp/tradestocker-ci-negative.NDJHfw/` own control clone, branches, logs and results.tsv
- `go-real-pg-results.json`, `go-real-pg.log`, and retained initial prefix-refusal diagnostic
- `w002-independent-review.json`, `committed-fake-secret-finding.json`
- `final-preservation.json`

Final-commit assets: `final-docs.diff`, `final-docs-review.json`, `final-history-gitleaks.json` (redacted), `final-history-scan.log`.

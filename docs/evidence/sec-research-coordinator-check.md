# SEC research coordinator verification

Date: 2026-10-23. Coordinator: codex-reviewer. Product development/rework belongs to Claude in existing session `8da61cee-0de9-4193-af9a-cb914502dcee`; the separate independent reviewer is named in [the independent report](sec-research-independent-review.md).

## Direct verification by the coordinator

Exact backend tested: `7ab44180b15cafbb16f7f3a5141bd8dfcdada440`, from a Git archive outside the checkout. External review overlay `work/review/live_job_review_test.go` and `run_live_job_review.sh` are retained in the coordinating chat workspace, outside product code.

`TestReviewLiveSECJobAPIStorageParserAndFinancialIsolation`, executed with the race detector against its own uniquely named disposable PostgreSQL database: **PASS**. It used the actual authenticated HTTP handler, durable job creation, real SEC provider and isolated embedded Python parser. Apple CIK `0000320193` produced five original annual periods; both balance checks passed in all five. The latest original annual revenue was exactly `416161000000` USD. Both original downloads matched their recorded SHA-256. The paper workspace document was byte-for-byte unchanged before and after processing. The test database was dropped afterwards. No shared API/frontend port was used.

Actual sources received in that run:

| Source | Bytes | SHA-256 |
|---|---:|---|
| submissions | 163917 | `47e4383b0133a4146cc631725f9aa28bac6d5679d504f735a78f1a1923c4b622` |
| companyfacts | 3789099 | `73a86c6aedc31f77cac2ea4df5f80f0b3bd7e6eb58bb4e01444fbedf3afb9c43` |

The contact sent to `data.sec.gov` was specifically approved by the human. No SEC contact was sent to Python. These are official filing snapshots, not prices or historical point-in-time archives. The result remained `INSUFFICIENT_DATA`, ineligible for entry and backtesting.

The coordinator also directly ran the planning-tool regression suite after the documentation checkpoint: **20 tests passed**. The independent report distinguishes its own checks from coordinator/author checks; neither author test output nor a passing planning validator substitutes for independent review.

## Final cross-repository gate

At backend `5da12da73cd66f52b53dc44ff08cd1260329c271` / frontend `7c739f5beb51089280598804e1499d4691cc05c4`, the author ran both browser E2E modes (7 passed, 1 intentionally skipped in each), the real SEC smoke, build/unit/integration and vulnerability checks. The coordinator observed these outputs through the original VS Code conversation. These are author runs, not independently rerun cross-repository evidence.

The independent reviewer then reproduced an additional P2 in both fixed-port test runners: a preexisting server could satisfy unauthenticated readiness while the intended child had failed, causing test mutations to target the old instance. Reproduction used fake credentials/commands and a private port, without real financial state or SEC access. Claude corrected it in `7cf3123ed47ded23f3f74968c98f5bd672855f9c`, with final evidence in `4db63e550d6042ef1869dfe3487522041bade902`; the corrected frontend caption is in `2e1841916eecefbb954a3c304d965cff3cf93744`.

The separate reviewer independently verified the isolated runner fixes, all eight helper tests, occupied-port refusal and four post-preflight wrong-instance cases. It then ran both complete browser E2E modes itself: **7 passed / 1 intentional skip in each mode**, including a real SEC job, five original annual rows, original-download SHA verification and visible issuer mismatch failure. The coherent contract/ADR/provider/evidence corrections were reviewed too. Its final report **accepts the complete bounded S07-T07 increment** at backend `4db63e550d6042ef1869dfe3487522041bade902` / frontend `2e1841916eecefbb954a3c304d965cff3cf93744`, with no remaining actionable finding in that scope. The normal paper document checksum stayed unchanged, and disposable databases/servers were cleaned up. See the independent report for the exact commands and limits.

No live lane, broker action, production deployment, paid resource or real Telegram delivery is authorized by these results.

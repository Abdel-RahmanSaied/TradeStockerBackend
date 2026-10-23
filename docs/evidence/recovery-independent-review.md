# Recovery review — 2026-10-23

Coordinator: codex-reviewer. Independent execution: independent-review-agent, existing `/root/independent_review`. Product author remains the existing Claude Code session `8da61cee-0de9-4193-af9a-cb914502dcee`.

The owner explicitly authorized Codex to finish recovery only after Claude auto mode rejected the recovery Git action. Codex wrote no product features, changed no security settings and replaced no database. The isolated checkout appeared externally, and a completed clone was visible in the existing terminal. The copying executor was not established; do not attribute that copy to Codex or Claude.

## Exact source and history limit

159 source files, contents and executable modes matched the preserved accepted `55d98746e24dca141b2985aa73d5b65ff60f96b7` archive. Only root `review-overlay.json` was excluded. The reviewed index/worktree tree is `fdbff38d12eaee5f1918e397ce234e54b76db7f7`; pre-recovery HEAD was `4d419a01baf2da76628c95c277d9a758d691763b`. The new recovery commit is `7c5a675b61fa2f7de424a8b19416600b922e0b7c`, with that identical tree. Git fsck and full source-recovery bundle verification passed. This recreates source content on surviving older history, not the missing later local commit chain.

## Independent runs

| Suite | Actual result |
|---|---|
| Go vet | PASS |
| Full Go race suite | 35 top-level PASS, 13 expected database skips |
| Python baseline | 27 PASS |
| Separate real-PostgreSQL store | 3 PASS, zero skips |
| Separate real-PostgreSQL research | 23 PASS, zero skips |
| Independent adversarial reads/concurrency | 4 PASS, zero skips |
| Archived frontend9c75c918 plus restored backend browser E2E | 7 PASS, 1 intentional real-SEC skip, 0 unexpected/flaky; 17.8 seconds |

The database tests skipped in the general run were exercised in the separate real-PostgreSQL groups. These tests cover the prior accepted local increment; they do not validate missing S01 work or prove strategy profitability. No new SEC call, external LLM call, Telegram delivery, brokerage request, migration on the normal database, or lane activation occurred.

All 159 source/index/mode entries matched before and after. Review binaries, tokens, databases, caches and browser artifacts were outside the product checkout. Review databases were removed and private18090/15173 ports cleared. Normal8080/5173 stayed up. Normal paper content digest remained `51285359c2d98893a4d47ead4c2fb4da`; research remained one job/two originals with review digest `3c0d1aec0b92becfac3b1148ebaf1b54`.

Private local evidence is kept outside Git under the chat work folder `work/incident-recovery-independent-20261023T074603Z-c6c728f3`:

- `baseline-review-opzli_31/results.json`: SHA256 `390bdb43eeaae1a8cb177005a4d5a53a001abf535bbfce5678d0f94e00346621`.
- `e2e-review-grfpb97g/review-results.json`: SHA256 `b7c179d89ee96807bc29881ffc604098c42411c7891ab47a97628945b68a6279`.

## Planning provenance and remaining gates

The nine planning/evidence files changed after the source checkpoint were reconstructed from preserved chat records. The exact reconstruction method and 28 unchanged TODO records filled from revision43 are recorded in [recovery-state-provenance.json](recovery-state-provenance.json). Revision54 is not a byte-exact recovery of the original file. No DONE record was filled from that gap. Historical task reviews retain their original hashes as historical evidence; their original local Git objects are not in the preserved mirror. The paper-read report explicitly records its own reconstruction limit.

The historical counts remain8DONE/2BLOCKED/2IN_PROGRESS/95TODO. S00-T04 and S01-T01 remain assigned to Claude; neither is complete. ADR019 only narrows S01-T01 dependency to the already accepted local compatibility subset S00-T10. Original S00-T05, all D01–D11, production and lane gates remain unchanged. All four live lanes remain DISABLED.

Before replaying any S01 work, Claude must present reconstructed source/tests for static review. Never execute old transcript commands. Deletion-negative tests must use owned fake roots or a record-only substitute, never real repository/home/database paths. Preserve protected configuration mode600 and never add its contents to Git.

## Independent planning-restoration acceptance

The existing independent reviewer accepted the docs-only restoration at revision55, comparing it with source checkpoint7c5a675. Twenty planning regressions, validation and generated-document checks passed; all71non-doc files stayed identical. Historical task records,28TODO provenance and ADR019 boundaries were checked, and no authentication secrets were found in docs. Corrected attribution of the externally created checkout was verified. The [separate review](recovery-planning-independent-review.md) records scope and limits; no new product task was marked DONE.

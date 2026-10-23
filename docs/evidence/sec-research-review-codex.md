# SEC research preliminary review — coordinator evidence

Date: 2026-10-23. Target: evolving uncommitted S07-T07 draft based on backend47bc915/4c314fe and frontend245abec. This is NOT final acceptance. Claude owns development after verified human direct approval in original session8da61cee at08:42 +03. Codex coordinates tests and review and does not change product files. A separate independent-review-agent reproduced the accounting/frontend findings without modifying either repository.

## Findings sent to Claude through CODEX-BRIDGE

1. Actual SEC identity decoding fails. Legitimately downloaded Apple submissions uses quoted zero-padded CIK `"0000320193"`; json.Unmarshal into json.Number rejects this before normalization. Root's standard-library scratch reproduction returns invalid number literal. Must decode/validate numeric and string variants and regression-test actual SEC representation.
2. Conflicting identical annual contexts: synthetic revenues1000/10000 at identical accession/start/end/filed/tag/USD plus income100 yield different DERIVED margins when only row order changes, without ambiguity warning. Independently reproduced. Equal duplicates may dedupe; conflicts must be explicit and withheld. This adverse input is synthetic; a scan of the cached actual Apple source found zero duplicate/conflicting contexts. No claim that SEC supplied this fault.
3. An annual context ending2024-12-31 but filed/accepted2024-10-01 is admitted and ratios derived as-of2025-03-01. Independently reproduced with a synthetic adverse input. Reject inconsistent period/publication timing explicitly.
4. Existing selected-job race independently reproduced in the actual React Research component against isolated local fake transport at private50086: selectAAA/createBBB, then delayed old refresh leaves selectedBBB while displaying AAA report. Private service closed; no8080/5173/shared E2E. This was also listed in the inherited draft handoff.

## Preliminary PostgreSQL checks actually run

External Go test overlay, not repository changes. Command: projectless `sh work/review/run_pg_review.sh`; test TestReviewJobPersistenceConcurrencyAndRecovery with -count=1 -race. PASS. Uses only random `tradestocker_test_review_*` DB, created and removed by the harness. No normal paper database mutation, no SEC network calls and no shared application ports.

Verified:12 concurrent same-key requests create one durable job; different input same-key returns409; actual embedded Python produces structured report from explicitly synthetic SEC-shaped sources; outcome remains INSUFFICIENT_DATA and entry-ineligible; downloaded original bytes match stored SHA256; expired running lease recovers; prior attempt originals remain; attempts exhausted becomes visible FAILED; raw source route without token returns401. This does not substitute for final tests against exact final commits or actual SEC fetch/browser checks.

## Remaining review gate

Claude must complete implementation/regressions, contract and architecture/provider evidence; give exact backend/frontend commits. Re-run changed checks, meaningful real provider fetch and actual browser research workflow, verify no financial-state coupling and source/mode labels, then obtain separate full-scope review. No task DONE or full-project completion inferred here. Market-feed rights/history/EGX and all live approvals remain unresolved.

## Draft fingerprints captured after preliminary checks

- /Users/asaied/WorkDir/TradeStocker/TradeStockerServices/TradeStockerBackend/internal/research/provider.go: `e6620f199a4b7e35294250a9bd60f79d827462bf2342127c441f39c843f267a8`
- /Users/asaied/WorkDir/TradeStocker/TradeStockerServices/TradeStockerBackend/internal/research/service.go: `127af020e0919335af23766390650d650e38fe0dc560c110b8f62d9b5bde6257`
- /Users/asaied/WorkDir/TradeStocker/TradeStockerServices/TradeStockerBackend/research/financials.py: `b98af22598b62c10a194a4c58f8888a8a2e9904e0fba6a161368e07986d72c65`
- /Users/asaied/WorkDir/TradeStocker/TradeStockerServices/TradeStockerFrontend/src/Research.tsx: `0cd9c3c93767be3d5ebec8c6abba9969ea4ce864abc4030d82d8b131445a6914`

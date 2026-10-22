# Changelog

## 1.0.0 — 2026-10-23

Initial development planning pack based on the approved conversation scope. Added 16 core sprints (S00–S15), optional S16, 102 task specifications, requirements traceability, architecture decisions, contracts baseline, source-of-truth ownership, machine-readable execution state, local plan validation/status tools and safe installation helper.

All implementation statuses initialized TODO; all four advisory lanes DISABLED. No application code was inspected, changed, committed, pushed, deployed or activated on the user’s Mac. Package validation results belong in `PACKAGE_VALIDATION.md`; they do not count as product tests.

Future entries must identify changed task IDs, decision/ADR references, reason, approved scope and evidence. Do not rewrite historical acceptance criteria to make a failed result pass.

## 2026-10-23 — authorized local paper increment

Added S00-T07 and S00-T08 for bounded backend/frontend engineering while unresolved provider, risk and production decisions stay open. All 102 original tasks and release gates retained. Frontend location is the owner-specified sibling TradeStockerFrontend. Independent review supersedes the historical no-Git observation after the owner first commit.

Local implementation adds Go/PostgreSQL paper state, synthetic ingestion, research stubs, real-API dashboard and repeatable tests. Planning/install tests now use disposable fixtures instead of mutable live state; original installer manifest is preserved. Go project toolchain and text dependency updated after the initial vulnerability scan; see increment evidence. ADR-017 documents prototype limitations without superseding production tasks.

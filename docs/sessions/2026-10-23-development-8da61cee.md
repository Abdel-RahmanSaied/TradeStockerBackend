# Session handoff — 2026-10-23-development-8da61cee (Claude development)

Written by `claude-code-agent` in the original Claude Code conversation `8da61cee-0de9-4193-af9a-cb914502dcee`, for the coordinator (Codex) and any later session. Task state lives only in `docs/state/project_state.json`, which the coordinator writes; this file records what Claude did and what comes next. Registering it with `planctl handoff` is the coordinator's call.

## Authority and roles

- Owner direct line, 2026-10-23 08:42 +03: Claude develops both repos locally (edits, tests, app runs, local commits; SEC requests to data.sec.gov with User-Agent `abdo.am169@gmail.com`); no push, spend, production, broker orders, live lanes or real Telegram. Routine dependency downloads from official registries are within that approval; keep them isolated.
- Codex coordinates, writes shared state/catalog/handoffs, and runs independent reviews. Claude never self-reviews and never marks tasks DONE.
- Normal app: 8080 / 5173 / PostgreSQL 55432 and the `.env.local` token belong to the owner. Use disposable `tradestocker_test_*` databases and private ports.

## Work by Claude in this session (all local commits, none pushed)

| Task | Commits | Status (per coordinator) |
|---|---|---|
| S00-T01 repository inventory | evidence `repository-inventory.md`, ADR-016 | DONE (reviewed by Codex) |
| S07-T07 SEC research | backend `7fa1281` → `7ab4418` → `5da12da` → `7cf3123` → `4db63e5`; frontend `7c739f5` → `2e18419` | DONE after independent acceptance |
| Launcher fix | `8f867dc` | accepted |
| S00-T05 compatibility spike | `8d7d112`, `dd2f4b4` (outbox claim narrowed), `1e778e4` (Python dates) | technical subset accepted; task BLOCKED on D-05/D-06/D-07 |
| S00-T09 paper read fix | `dcbfd26`, `1aec76c`, `55d9874` (HEAD fix) | IN_PROGRESS, awaiting re-review of `55d9874` |

## S00-T09 state at handoff

- GET and HEAD read a committed snapshot without lock or write. A due expiry is committed once under `FOR UPDATE` and only when the locked state still has work. Only POST writes.
- Proof: exact trigger-counted UPDATEs on real PostgreSQL (0 for reads, 1 for a queued-reader expiry, 30 → 0 for HEAD); fixed-400 benchmark 36 → 178–189 reads/s, 414–424 → 0 updates.
- Evidence: `docs/evidence/paper-read-performance.md`. Checks at `55d9874`: race unit 35 pass; integration 26 pass; isolated E2E 7 pass / 1 skip.

## Next work (bounded, as directed by the coordinator)

1. **S00-T04 (prepare):** public-source research on account types, settlement, sessions, holidays, halts, eligibility, fees and taxes for US and EGX, with unknown real broker/account capabilities kept as blockers; a hypothetical capability matrix clearly labeled as not verified. Draft in `docs/evidence/broker-rules-research.md`.
2. **US_SWING critical path:** map which catalog tasks and decisions stand between now and a first evidence-backed US_SWING recommendation, using actually qualified price history only.
3. **S00-T03 provider notes:** record primary-source facts about Alpaca and Massive (formerly Polygon.io) only after reading the pages directly; note the Alpaca paper-only IEX entitlement discrepancy; no account, purchase or terms acceptance.

## Hazards

- Do not run `install_docs.py --apply` from the repo root; do not reset `project_state.json`.
- `test_planctl.py` / `test_install_docs.py` now use their own fixtures; planning state is real.
- Spike and benchmark use 55532 / 54222 / 58222 / 18180; E2E private ports 18090 / 15173 (or 18095 / 15175); check the coordinator owns none of them before running.
- Never send the SEC contact email to any other service.

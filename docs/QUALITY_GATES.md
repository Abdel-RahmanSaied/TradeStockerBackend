# Quality Gates — task completion is not product activation

## Definition of Ready for a task

Its prerequisites are DONE, scope and deliverables are understood, contracts/rules are identified, safe test data exists, and external dependencies are resolved or an explicit fixture-only scope is documented. No task is ready to consume unlicensed feeds merely because code can call a URL.

## Definition of Done for a task

Acceptance criteria pass with evidence. Relevant unit/integration/property/security tests are run and reported. Contracts and docs are updated. Exact commit/build is identified. Independent review is recorded, especially for ledger/risk/auth/migrations. A handoff exists. No hidden unrun test is described as green.

Completion of a review task may honestly record NO_GO. Completion of an implementation task cannot be replaced by a NO_GO review or mock when its criteria require real integration.

## Per-lane activation gates

| Gate | Required evidence |
|---|---|
| G0 — scope/account | account capabilities, owner authorization, strategy horizon and mode |
| G1 — legal/entitlement | permitted source use, display/derived/LLM rights, appropriate review for commercial scope |
| G2 — data | timestamps, corrections, history coverage, mandatory capabilities and freshness |
| G3 — financial state | reconciled ledger, settled/eligible cash, actual holdings, atomic allocation |
| G4 — research | current financial/history assessment, evidence validity, dissent/unknowns |
| G5 — risk | approved policy and same kernel in live/replay, fail-closed tests |
| G6 — quantitative evaluation | preregistered holdout/walk-forward, costs, human latency, uncertainty and baseline comparison |
| G7 — forward evidence | shadow/paper results with sample/coverage adequate under D-09; no calendar-only promotion |
| G8 — operational readiness | security, CI, metrics, backup restore, incident drill and scoped rollback |
| G9 — explicit activation | named reviewer + owner signoff + exact versions + limited scope and disable criteria |

Four independent reports: EGX_SWING, US_SWING, EGX_INTRADAY, US_INTRADAY. An approved US swing lane does not activate EGX or intraday.

The project-state JSON records evidence-backed approval status but does not deploy/change runtime. Runtime activation must be separately audited. This plan’s CLI never enables lanes.

## Mandatory tests

Financial invariants: no money creation, no double-fill posting, no negative available cash by concurrent reservation, no sale above held quantity, no goal-driven risk increase.

Temporal invariants: no future facts, no silent restatement overwrite, historical universe and corporate actions versioned, stale recommendation expires, material update invalidates cache.

Human-only invariants: no broker order route/tool/credential; ACK is not fill; a pause is not liquidation; expired recommendation does not close a position.

Reliability: restart before/after commit/ACK, poison message, queue backlog, provider outage, expired lease/fencing, DB rollback/restore, ambiguous Telegram send.

Research: correct units/period/consolidation, evidence matching, hallucinated citation rejection, prompt-injection resistance, model pretraining leakage limitation, ablation versus baseline.

## Metrics and thresholds

Define thresholds before reviewing strategy results. D-07 owns engineering performance/freshness/recovery targets. D-09 owns statistical sufficiency/uncertainty/benchmark/cost gates. A metric without workload/data and an evaluation window is not an SLO or strategy proof.

Report net return, drawdown, turnover, exposure, fees/slippage, opportunity/actual-fill difference, alert latency, data gaps, abstention, cost and goal shortfall/attainment. Do not optimize only win rate or count only executed winning trades.

## Re-entry and rollback

Any material model/prompt/strategy/cost-policy/data-source change repeats affected gates. Software rollback does not reverse actual trades or ledger history. Preserve old manifests and explain economic consequences separately. Keep position monitoring active when new entries are paused.

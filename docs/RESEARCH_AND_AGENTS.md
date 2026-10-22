# Advisory Committee and Trading Intelligence

Logical roles, not 23 microservices or 23 compulsory LLM calls. Shared evidence does not become independent statistical confirmation merely because multiple prompts cite it.

## Research and independent control roles

| ID | Role | Work type / decision authority |
|---|---|---|
| R01 | Chief / synthesis | Bounded workflow orchestration; proposes rationale; no risk override |
| R02 | Macro economist | Context for US/Egypt policy, inflation, rates and liquidity |
| R03 | Geopolitical analyst | Conditional event analysis; distinguish facts, scenarios and uncertainty |
| R04 | Fundamental analyst | Synthesizes financial/peer evidence; not duplicate extraction |
| R05 | Technical analyst | Interprets tested trend/setup features |
| R06 | News/sentiment analyst | Licensed news, deduplication, evidence quality and catalysts |
| R07 | Portfolio strategist | Proposes fit using actual confirmed exposures; no ledger writes |
| R08 | Risk reviewer | Explains deterministic Go policy results; cannot change them |
| R09 | Financial statements specialist | Mandatory statements, cash flow, notes, auditor issues and provenance |
| R10 | Historical behavior specialist | Mandatory raw/adjusted history, liquidity, drawdown and event context |
| R11 | Liquidity / microstructure | Quant-first execution feasibility; requires actual data capabilities |
| R12 | Event / catalyst specialist | Scheduled and newly observed events within the holding horizon |
| R13 | Cross-asset specialist | Conditional FX/rates/oil/other exposures using licensed inputs |
| R14 | Sector / peer specialist | Sector-aware comparisons; prevents invalid generic ratio templates |
| R15 | Market regime | Tested quant classification; cannot autonomously alter strategy limits |
| R16 | Statistical edge / quant | Backtest/evaluation evidence and uncertainty, not invented win probability |
| R17 | Devil’s advocate | Strong supported countercase and missing assumptions |
| R18 | Evidence & data auditor | Required source/time/quality checks; not a substitute for legal counsel |

## Trading Intelligence Division — five specialists

| ID | Specialist | Data dependency |
|---|---|---|
| T01 | Order flow and book behavior | Executed trades, quotes; L2 reconstruction only when licensed, complete and tested |
| T02 | RVOL / VWAP / volume profile | Accurate session/trade history; same-time-of-day baselines; profile needs sufficient resolution |
| T03 | Intraday price action and setup | Fresh bars/quotes, spread and explicit invalidation; no image-only pattern claim |
| T04 | Market breadth and sector rotation | Point-in-time universe and comparable reliable symbols; coverage limitations stated |
| T05 | Human execution feasibility | Spread, fees, size, user availability, delay, session cutoff and eligibility |

T01/T02 features are calculations first. Interpretations from these roles do not replace R11 liquidity checks or R18 evidence validation. Avoid counting overlapping inputs twice in a composite score.

## Workflow

1. Cheap deterministic candidate screening with source/capability gates.
2. Require valid financial and historical reports; re-use snapshots only while inputs remain valid.
3. Independent first-pass applicable research roles.
4. Bounded evidence-based challenge and dissent capture.
5. Structured synthesis with missing inputs and invalidation conditions.
6. Independent evidence gate and Go policy evaluation on fresh account/market state.
7. Recommendation revision and official delivery only from Go core.

If any mandatory assessment is unavailable, the outcome is INSUFFICIENT_DATA/WATCH/NO_TRADE. Missing optional evidence can be disclosed only if the strategy’s approved requirements allow it.

## Evaluation and leakage

Store structured rationale, evidence and dissent rather than private model chain-of-thought. A schema-valid output is not automatically financially correct. Check claims and calculations independently.

Historical LLM evaluation has a special risk: a model may already know later outcomes from training. Restricting retrieval timestamps does not prove that internal knowledge is absent. Use historical runs as research with this limitation, evaluate deterministic portions separately, and require forward shadow/paper evidence for live claims.

No model writes prompts/risk rules into production based on short-term performance. New prompts/models/configs are versioned experiments. Record unsuccessful trials and role-ablation results; do not select only profitable runs.

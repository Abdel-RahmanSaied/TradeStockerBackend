# First evidence-backed advisory: readiness and remaining work

Date: 2026-10-23. Coordinator: codex-reviewer. The owner requests continued work through all tasks toward a first real, independently verified recommendation. Claude develops in the original session; separate reviews and reproducible tests remain mandatory. This request does not make a recommendation valid, choose risk limits, buy data or activate any lane.

## Current evidence

The local paper lifecycle, real dashboard, bounded SEC annual research and compatibility subset have genuine reviewed implementations. S00-T09's unchanged-read fix was also independently accepted at backend `55d98746e24dca141b2985aa73d5b65ff60f96b7` / frontend `9c75c9183df2ed9457012260bb904a9ea404a960`. These prove specific engineering behavior, not a trading edge. There is no qualified price dataset, strategy return series, holdout result or prospective strategy record in these evidence packs. No actionable real recommendation is ready. SEC reports intentionally remain entry/backtest ineligible.

## Critical path, retaining original tasks and gates

| Work | Required proof before it can support real advice |
|---|---|
| S00-T03/T04 | Real market-data entitlement and samples; account/rule/settlement capabilities with effective dates. Unknowns remain lane blockers. |
| S01/S02 | Actual entrypoints/builds/CI, identity and account boundaries, exact relational financial state, event contracts/outbox/inbox, recoverable workers and immutable datasets. Local scaffolding may use ADR-019's precise compatibility split; production gates remain. |
| S03/S04/S05 | Qualified ingestion and price history, stable instrument/corporate-action identities, fresh input/session checks, approved deterministic risk policy and human-only lifecycle. No broker orders. |
| S06/S07 | Original point-in-time financial evidence and historical analysis; explicit data gaps; a frozen experiment record before evaluating holdouts. Today's SEC companyfacts snapshot alone is not historical availability certification. |
| S08/S09 | Versioned strategy/committee and adversarial research, reproducible net-cost backtests, out-of-sample/walk-forward checks, multiple-trial accounting and sensitivity to costs/gaps/regimes. Weak or inconclusive evidence must yield NO_TRADE. |
| S10–S15 | Correct delivery/monitoring/manual execution reports, security/recovery checks, actual prospective paper/shadow observation, independent lane review and documented human activation. Each market/horizon qualifies separately. |

Do not choose a winning strategy and then tune the test or minimum evidence to justify it. Benchmarks, fees/slippage assumptions, train/holdout dates, candidate-trial count, minimum sample size and release thresholds belong in a preregistered D-09 protocol before the holdout is inspected. Report failures, drawdown, uncertainty and missing regimes alongside successful results. The [original backtest-overfitting research](https://www.davidhbailey.com/dhbtalks/dhb-hamilton-fin.pdf) explains why selecting among many apparently successful historical variants requires particular care. No research claim here guarantees future returns.

Forward observation takes actual elapsed sessions. It cannot be manufactured by replaying historical bars under today's clock, nor can paper fills be labeled executed real trades. A read-only analysis candidate is not an activated actionable recommendation; a valid NO_TRADE result is not proof that the requested first actionable recommendation has been achieved.

## Provider exploration, not qualification

No market-data provider variable names were present in the inspected backend `.env`/`.env.local`; no secret values were printed. The owner confirmed there is no existing subscription or licensed history and requested a trustworthy free source. Local engineering continues alongside [free-source screening](provider-free-source-screening.md). Published offerings remain candidates until rights, actual account entitlement and representative samples are verified.

Candidate US historical sources checked on 2026-10-23:

- Alpaca's [market-data plans](https://docs.alpaca.markets/us/docs/about-market-data-api) and [FAQ](https://docs.alpaca.markets/us/docs/market-data-faq) describe Basic historical access and SIP queries ending at least15minutes in the past, with live Basic data limited to IEX. Its [paper-only account page](https://docs.alpaca.markets/us/docs/paper-trading) instead states an IEX-only entitlement. This account/scope discrepancy must be resolved with actual authorized entitlement/sample validation; no blanket free-SIP qualification is recorded. Keys/account are needed, and redistribution/retention/processing rights must be checked for the intended use.
- Massive's [official stocks page](https://massive.com/stocks) advertises a free Basic end-of-day tier with two years of history and five requests/minute. This is a candidate's published offering, not an entitlement, representative dataset, long-history validation or approved vendor. Account rights, adjustments, delistings and usable sample coverage remain unverified.

No account, agreement, purchase, external model call or provider credential was created. The specifically approved SEC contact must not be reused for another service. Credentials belong in protected local configuration, never chat/docs/Git. An allowlisted data-only adapter must not include the provider's brokerage/order API.

## Decisions still requiring actual evidence or owner choices

D-01/D-02 data coverage/rights; D-03 account capabilities; D-04 capital/risk settings; D-05 identity; D-06 hosting/budget; D-07 workload/recovery/freshness targets; D-08 distribution/legal scope; D-09 evaluation thresholds; D-10 Telegram pairing; optional D-11. All retain their canonical OPEN status until the resolution protocol is met. Routine authorized local code, tests, dependency installation and local commits do not require a new permission at every step. Actual external commitments and live activation are different gates.

Next implementation: original S01-T01 module/build entrypoints and S00-T04 rule research, with independent validation and preserved normal application data. Complete the original roadmap incrementally, recording exact DONE evidence and exact blockers rather than lowering acceptance criteria to match a desired recommendation.

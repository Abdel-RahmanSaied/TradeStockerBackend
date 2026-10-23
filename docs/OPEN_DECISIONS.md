# Open Decisions — ownership and unblock criteria

Status is canonical in `state/project_state.json.decision_status`. This file defines the questions; do not duplicate current status here. Evidence must be local to docs or an approved archived reference.

| ID | Decision | Owner | First needed | Resolution evidence |
|---|---|---|---|---|
| D-01 | US data vendor, venues, tier, history and rights | Owner + Data Lead | S03 real US adapter/data use | signed/approved entitlement record + sample validation + cost |
| D-02 | EGX data vendor, real-time/history/quotes and document rights | Owner + Data Lead | S03 real EGX adapter/data use | entitlement/coverage evidence; explicit missing L2 and history limits |
| D-03 | Brokers/accounts, currency, settlement and same-session capabilities | Owner + Domain Lead | personalized account rules / live lane | verified account capability matrix and rules sources |
| D-04 | Capital, base currency, per-trade/lane/account loss limits and goal semantics | Owner + Risk Reviewer | actionable recommendations | explicitly approved versioned risk config; no example defaults |
| D-05 | Identity provider/session design and owner/reviewer access | Tech Lead + Security | S01 auth | threat-reviewed identity ADR and test plan |
| D-06 | AWS account/region/topology, NATS operations, availability and spending budget | Owner + Platform | production provisioning | approved IaC plan, cost estimate and recovery model |
| D-07 | Workload profile, SLOs, RTO/RPO, freshness/cost budgets | Tech Lead + Owner | capacity/reliability gates | measured spike + preregistered acceptance thresholds |
| D-08 | Private pilot vs commercial advisory; relevant legal review and data distribution | Owner + qualified counsel where needed | external commercial delivery | documented scoped review; a model is not legal approval |
| D-09 | Benchmarks, costs, holdouts, minimum evidence and lane release thresholds | Quant + Independent Reviewer | before evaluation selection | preregistered evaluation protocol with uncertainty criteria |
| D-10 | Telegram bot ownership, paired chat IDs and secret rotation | Owner + Security | real bot setup | protected config/permission test; no token in docs |
| D-11 | Optional OpenClaw need and scope approval | Owner | S16 only | read-only scope and ADR-011 compatibility review |

## Resolution protocol

Record authority, date, scope, evidence and expiry/review date. Update `decision_status` under an incremented revision with an audit history entry and corresponding ADR where needed. Values allowed: OPEN, RESOLVED, DEFERRED. DEFERRED never satisfies an activation gate.

A task may contain engineering work that proceeds with fixtures while a decision is open. The claim tool prints this fact; it does not authorize real provider access or production use. Completion evidence must distinguish fixture-level validation from real integration.

If a missing market provider blocks only one market, split the task explicitly with stable new IDs and scope; do not erase the missing prerequisite or mislabel the market as ready.

## Account-rule research input

[S00-T04 research](evidence/broker-rules-research.md) records dated public sources, corrected free-riding exceptions and separate hypothetical capability matrices. It does not verify the owner's actual accounts, Alpaca key scopes, fees or legal distribution rights. The questions in its section 5 remain inputs to D-03/D-08; neither decision is resolved by that document. Unread official EGX sources and announcement-only tax rates cannot authorize market rules or cost models. Local S01 containers/fixtures can proceed independently.

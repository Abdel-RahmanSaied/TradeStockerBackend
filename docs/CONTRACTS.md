# Contract Baseline — to be implemented in S02

This document specifies direction and ownership. It is not an OpenAPI implementation, database migration or proof of existing endpoints.

The executable contracts now live in `contracts/` (S02-T01): OpenAPI for the served API, the SEC parser payloads and the v1 event schemas, with their reconciliation against this baseline in [contracts/README.md](../contracts/README.md).

## 1. Shared envelope

Proposed event shape (synthetic identifiers; no actual recommendation):

```json
{
  "event_id": "evt_fixture_001",
  "event_type": "research.assessment.completed",
  "schema_version": 1,
  "occurred_at": "2026-10-23T09:00:00Z",
  "published_at": "2026-10-23T09:00:01Z",
  "producer": "research-worker",
  "producer_build": "fixture-build",
  "correlation_id": "corr_fixture_001",
  "causation_id": "job_fixture_001",
  "mode": "PAPER",
  "account_id": null,
  "data": {
    "assessment_id": "asmt_fixture_001",
    "instrument_id": "instrument_fixture_001",
    "strategy_id": "US_SWING",
    "evidence_manifest_id": "manifest_fixture_001",
    "assessment_revision": 1,
    "status": "INSUFFICIENT_DATA"
  }
}
```

Money/price/quantity precision is explicit; financial decimal values are strings in JSON. Stable instrument identity is not a free-form ticker. Events have schema compatibility fixtures in Go and Python. Unknown mandatory fields/versions are quarantined, not silently coerced.

## 2. Proposed event catalog

| Event | Producer → consumer | Business uniqueness and implications |
|---|---|---|
| `market.bar.finalized.v1` | ingestion → screening/history | provider + instrument + interval + start + version |
| `market.quote.updated.v1` | ingestion → intraday/quality | source sequence or approved source identity; duplicates harmless |
| `market.status.changed.v1` | ingestion → core/research | effective-dated trading status; invalidate affected advice |
| `market.data.quality.changed.v1` | ingestion → core/ops | quality revision; entry gate re-evaluated |
| `evidence.document.available.v1` | ingestion → research | document hash + publication/version |
| `research.assessment.requested.v1` | core → research | candidate + snapshot/config hash + mode |
| `research.assessment.completed.v1` | research → core | assessment ID/revision; no direct official delivery |
| `recommendation.revision.created.v1` | core → notifications/paper | recommendation ID + revision + destination/mode |
| `recommendation.invalidated.v1` | core → notifications/monitoring | revised reason + causal change; old entry never revived |
| `portfolio.execution.recorded.v1` | core → projections/monitoring | account + actual execution/import key |
| `portfolio.reconciliation.changed.v1` | core → decision/goals | account revision; blocks stale personalized advice |
| `goal.assessment.completed.v1` | research → core | goal revision + input manifest; cannot alter risk |
| `risk.entries.paused.v1` | core → routing/ops | scope + policy/reason revision; monitoring continues |

Subject/environment separation prevents REPLAY/PAPER consumers from accessing production notification effects. Consumer acknowledgment follows committed state. Redrive uses original logical IDs and records operator authorization/reason.

## 3. Go/Python service contracts

`AssessmentRequest`: job/candidate/instrument/strategy IDs, mode, as-of cutoff, licensed evidence manifest, required roles, financial/history report references, config versions, budget/deadline and idempotency key.

`AssessmentResult`: typed status, concise rationale, evidence references, dissent, missing inputs, invalidation conditions, data coverage, model/prompt/tools versions, cost/timing and signed/trusted service identity. No arbitrary SQL, executable code or risk override.

`PolicyEvaluationInput`: exact account/ledger revision, confirmed balances/holdings, candidate, strategy/policy/rule versions, market data and research snapshots, approved limits and current time.

`PolicyEvaluationResult`: allow/reject/watch/insufficient-data, reason codes, maximum proposed long quantity, capital/risk requirements, validity range and required rechecks. Deterministic and side-effect-free; reservation is a separate atomic application transaction.

## 4. Proposed API surface

Prefix `/api/v1`. Auth scope, account ownership, rate limiting, pagination and typed error responses are common. Write endpoints use stable idempotency keys where meaningful and optimistic revisions where state can be concurrently changed.

| Endpoint | Responsibility |
|---|---|
| `GET /health/live`, `GET /health/ready` | Minimal health; no secrets |
| `GET /instruments`, `GET /market-status` | Entitled instruments and effective market state |
| `GET /accounts/{id}/portfolio` | Confirmed actual holdings, cash, revisions and freshness |
| `POST /accounts/{id}/execution-reports` | Record a past human execution; **not** send an order |
| `POST /accounts/{id}/imports/preview` | Validate an authorized statement import |
| `POST /accounts/{id}/imports/{import_id}/commit` | Apply previously reviewed import idempotently |
| `GET /accounts/{id}/reconciliations` | Discrepancies and resolution history |
| `GET /recommendations/{id}` | Current and historical revisions; permission-filtered evidence |
| `POST /recommendations/{id}/responses` | ACKNOWLEDGE / IGNORE / WATCH; not execution |
| `POST /research-jobs` | Authorized bounded asynchronous research; no automatic advice bypass |
| `GET /research-jobs/{id}` | Status/results with data cutoff and limitations |
| `POST /goals`, `PATCH /goals/{id}` | Human-approved goal creation/revision; cannot raise risk policy |
| `GET /goals/{id}/progress` | Actual/paper progress separately |
| `POST /availability` | Human availability windows; affects fresh intraday entries |
| `POST /controls/entry-pause` | Authenticated scoped pause, not broker liquidation |
| `POST /telegram/webhook` | Validated/deduplicated updates with identity checks |

There is no `/orders`, `/execute-trade`, autonomous broker tool or equivalent capability. A `BUY` recommendation is not an order object.

Suggested error families: `STALE_MARKET_DATA`, `UNRECONCILED_PORTFOLIO`, `MISSING_FINANCIAL_ASSESSMENT`, `INSUFFICIENT_HISTORY`, `UNVERIFIED_MARKET_RULE`, `RISK_LIMIT_EXCEEDED`, `USER_UNAVAILABLE`, `LICENSE_NOT_APPROVED`, `STALE_REVISION`, `ENTRY_PAUSED`.

## 5. State machines must remain separate

**Recommendation revision:** draft → evaluated → published; may then be expired/invalidated/superseded. Historical content is immutable.

**Delivery:** pending → sending → sent/unknown/failed. Sent means API acknowledgment, not user reading or a trade.

**Human response:** none/acknowledged/ignored/watched. Independent from actual execution.

**Actual execution report:** recorded → reconciled or disputed/corrected by linked entries. No synthetic closing on advice expiry.

**Lane:** disabled → data_validated → replay_validated → shadow → paper_validated → limited_advisory → active_advisory; any stage may pause entries. Every promotion has exact evidence and owner authorization.

## 6. Database ownership and minimal invariants

| Owner | Main entities | Constraints/invariants |
|---|---|---|
| core | accounts, ledger journals/lines, quantity entries, execution reports, reconciliation snapshots | account-scoped identities, balanced currency journals, positive quantities, external fill uniqueness |
| core | risk policies, strategy configs, reservations, recommendations/revisions | append-only versions; concurrent budgets checked under transaction; unique logical candidate/version |
| core | goals/revisions/assessments/progress, notifications/attempts, user availability | goal cannot mutate risk; sends have attempt status; actual and paper are isolated |
| market | instruments, listings, calendar sessions, effective-dated rules, bars, quote/trade metadata, corporate actions | stable IDs, validity windows, source revisions, session-aware keys |
| research | filings/facts, evidence manifests, assessments, role outputs, experiments, checkpoints | source/period/unit/publication provenance; model/prompt/data versions; no core writes |
| audit | access/action records, approval references, outbox/inbox metadata as scoped | immutable history, authorization context, event uniqueness |

S02 supplies actual DDL, FK/index choices, migration tests and retention partitions after workload measurement. Do not claim this table is a complete physical schema.

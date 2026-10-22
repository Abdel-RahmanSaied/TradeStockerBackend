# Architecture Baseline

**Design, not an implementation inventory.** Reconcile proposed paths and dependencies during `S00-T01`.

## 1. Topology and ownership

```text
Approved price / filings / macro / news sources
           |
           v
Go market-ingestion ──> normalized events / immutable evidence
           |                         |
           v                         v
     NATS JetStream             S3 + Parquet/raw files
           |
           v
Go cheap screening / job scheduling
           |
           v
Python research-worker ──> evidence-backed assessments
           |                 (no ledger/risk writes)
           v
Go platform-core: fresh inputs → policykernel → reservation → recommendation
           |
           v
PostgreSQL transaction: recommendation + outbox
           |
           v
Telegram delivery → HUMAN acts outside platform → execution report → ledger

React dashboard → Go API only
Optional OpenClaw (last) → restricted read-only Go API only
```

Three code entrypoints; deployment may run isolated worker groups from the same image. Separate online research from offline backtests with queues, concurrency and resource limits. Avoid false independence: multiple replicas sharing a bottleneck still require measured capacity planning.

| Component | Owns | Must not own |
|---|---|---|
| `platform-core` (Go) | API/auth, portfolio journal, actual fills, goals, policy, capital allocation, recommendation revisions, Telegram, audit | Broker execution; unrestricted content browsing |
| `market-ingestion` (Go) | Feed adapters, subscription ownership, normalization, quality, calendars, raw ingest provenance | Account balances; advice activation |
| `research-worker` (Python) | Financial/historical assessments, quant features, committee, scenarios, backtesting | Direct core-table writes; delivery of official actionable advice; risk overrides |
| `policykernel` (Go library + batch command) | Deterministic eligibility, sizing and limits | LLM calls; database access inside pure rule evaluation |

## 2. Stack and version policy

Go / chi / pgx / sqlc / goose; Python / uv / Pydantic / LangGraph / NumPy / pandas; PostgreSQL; NATS JetStream; object storage + Parquet; DuckDB; React / TypeScript / Vite / TanStack Query.

This list is a design selection, not a claim that all versions interoperate. S00 pins tested supported versions and hashes; CI checks generated SQL/types and reproducible lockfiles. Exact money uses decimal arithmetic with documented precision/rounding; PostgreSQL `numeric` supports exact numeric storage [SRC-03](SOURCES.md).

## 3. Storage boundaries

- PostgreSQL schemas `core`, `market`, `research`, `audit` with separate write roles. Same database does not grant cross-service ownership.
- Core journal is append-only, balanced by currency; quantity ledger/positions and balance projections can be rebuilt. No binary floating-point reference balances.
- Raw source documents are preserved in original formats; Parquet stores analytical history. Manifests fix object hashes, correction/adjustment version and source entitlements.
- Hot bars/features may live in PostgreSQL; full raw tick/order-book history is not blindly dumped into core tables. Retention/capacity follows measured provider data volumes.
- NATS is transport plus bounded replay, not the only long-term market archive. Large evidence travels as scoped object references.

## 4. Delivery and idempotency

Use outbox/inbox and stable logical IDs. Commit business effects before acknowledging consumption. Producer de-duplication windows do not replace permanent business uniqueness. Outbox can publish more than once; consumers must tolerate that [SRC-01, SRC-04](SOURCES.md).

A unique recommendation is not exactly-once Telegram delivery. A remote send may succeed before the client times out; persist UNKNOWN and reconcile/escalate according to a documented policy. Do not invent a Telegram idempotency-key guarantee. Incoming updates are deduplicated separately [SRC-05](SOURCES.md).

LangGraph checkpoints preserve workflow state, but external side effects require separate idempotency/reconciliation. Persist model output before advancing where possible; do not assume a retry returns identical text [SRC-02](SOURCES.md).

## 5. Timing and point-in-time correctness

Persist `event_time`, `published_at`, `ingested_at`, `available_at`, `decision_at`, correction version and clock quality where applicable. Unknown source publication time stays unknown with a conservative availability rule; never substitute accounting period-end as market availability.

Store UTC instants; calculate sessions from verified calendars and IANA zones. A US or Egyptian session is not a hard-coded offset from Riyadh. Corporate actions use effective/entitlement/payment dates as distinct concepts. Realistic backtests distinguish source availability from the time the platform actually ingested an event.

## 6. Intraday versus swing

Intraday runs cheap streaming features, recent financial/history snapshots and event-aware invalidation. It does not wait for 23 logical roles on every tick. A material new filing invalidates cached research even if its TTL has not elapsed.

Swing permits deeper scheduled research but still rechecks fresh prices, account state and market status at finalization. Intraday entry validity must tolerate measured human latency. No scalping. A halt can prevent same-day exit; represent residual holdings and alert the human rather than invent a closing fill.

## 7. Goal engine

Goals: base currency, dedicated capital, metric (realized/unrealized/total net return), horizon, cash flows and risk caps. Forecast/scenario model is Python; authority is Go. A target cannot modify risk policy, extend a losing trade, pool unavailable foreign cash or force a new entry. No-trade and infeasible-goal outcomes are first-class.

## 8. Proposed repository map

```text
TradeStockerBackend/
  AGENTS.md
  go.mod
  cmd/
    platform-core/
    market-ingestion/
    policy-eval/
  internal/
    auth/ instruments/ marketcal/ providers/ dataquality/ events/
    scheduling/ ledger/ executions/ settlement/ reconciliation/
    policykernel/ allocations/ recommendations/ monitoring/ goals/
    researchjobs/ telegram/ notifications/ telemetry/
  db/migrations/
  db/queries/
  contracts/openapi/
  contracts/events/v1/
  contracts/fixtures/
  research/
    pyproject.toml
    uv.lock
    src/filings/ financials/ history/ evidence/ assessments/
    src/committee/ trading/ backtest/ goals/ performance/
    evals/
  strategies/{egx_swing,us_swing,egx_intraday,us_intraday}/
  web/                       # reconcile with existing frontend/sibling repo first
  deploy/{terraform,nats,observability,releases}/
  tests/{architecture,security,integration,invariants,parity,load,chaos}/
  docs/
```

Do not relocate an existing frontend or research service just to match this illustration. S00 documents the actual workspace mapping and updates the task deliverables accordingly.

## 9. Workload and SLOs

Measure throughput, backlog age, DB contention, feed freshness, committee duration, message send latency and cost under a declared workload. Prior chat latency numbers are not approved SLOs. D-07 fixes targets, RTO/RPO and sample measurement before gate evaluation.

Entry quality and financial integrity are hard invariants. Latency is a measured budget. Shed/defer optional research before bypassing a correctness gate.

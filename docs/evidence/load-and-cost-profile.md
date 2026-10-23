# Load and cost profile — S00-T05 (local spike)

Owner: `claude-code-agent`. Measured 2026-10-23, 06:53–06:54 UTC, by `spikes/s00-t05/run.sh` from a **clean state** (no spike tools, caches or images present before the run). One machine: macOS 27.0.1, Apple silicon (arm64), Docker 29.7.2. These are local engineering numbers, **not SLOs or targets**; D-07 owns targets.

All data is synthetic except the source-size sample (Apple's real SEC files, already downloaded, no new SEC request). The normal app, its database (55432) and its ports (8080/5173) were not used.

## Measurements

| Measure | Result |
|---|---|
| JetStream publish, one synchronous publisher, 5,000 messages | 5,582/s; p50 0.169 ms; p95 0.234 ms |
| JetStream stored size | 720,000 bytes for 5,000 messages (144 bytes each, headers included) |
| Consumer: one PostgreSQL transaction (inbox insert + effect) per message, then ack | 1,159 messages/s |
| One research job's original sources (Apple) | 3,953,016 bytes raw; 671,744 bytes on disk after PostgreSQL compression |
| One real SEC research job, end to end (Apple) | about 2.7 s (smoke run in [sec-research.md](sec-research.md)); the client waits 1 s between SEC requests by design |
| Paper API reads, small state (2.5 KB response), 8 concurrent | 1,868/s; p50 1.6 ms; p95 12.3 ms; p99 59.5 ms |
| Paper API candidate evaluations (writes), 8 concurrent | 79.8/s; p50 96.5 ms; p95 177 ms |
| Paper API reads after 412 evaluations (703 KB response) | 39/s; p50 201 ms; p95 214 ms |
| Paper state after that load | one 1,168,980-byte JSON document; its table uses 95,354,880 bytes on disk |

Two earlier full runs the same morning gave the same picture within normal variation (for example, reads after writes: 38.7/s and 40.3/s).

## What the numbers mean

- **The paper prototype does not scale with history.** Each evaluation adds about 2.8 KB to one JSON document (1,168,980 bytes / 412). Every request, including reads, locks and rewrites that whole document, so reads become about 48 times slower after roughly 400 evaluations. The table grows about 80 times larger than the document, because PostgreSQL keeps old row versions until vacuum. ADR-017 already calls this a low-throughput prototype; these numbers confirm it. Possible fixes (not done here, each needs its own reviewed task): stop rewriting the document on reads that change nothing, then move to the planned relational schema (S02).
- **JetStream de-duplication with an inbox consumer** handles thousands of messages per second here; the consumer is limited by one database transaction per message (about 0.86 ms each). A real outbox relay (reading and marking the outbox table) was not measured.
- **SEC research** is limited on purpose by the 1-request-per-second spacing (two requests per job). That is a courtesy limit, not a capacity target.

## Retention estimates (formulas, not plans)

- Research sources: about **0.67 MB on disk per Apple-sized job**. Issuers differ in size; the distribution is UNKNOWN. Illustration only: 100 issuers refreshed weekly would be 5,200 jobs a year, about 3.5 GB a year.
- JetStream: about **144 bytes per small event**, always bounded by the stream's age and size limits (the spike used 1 hour and 256 MiB). Real event rates are UNKNOWN until a market-data vendor is chosen.
- Paper aggregate: not suitable for retention planning; it should be replaced before it holds long history.

## Workload inputs still unknown

| Input | Status | Decided by |
|---|---|---|
| Symbols per market (EGX, US) | UNKNOWN | owner universe; vendor coverage (D-01, D-02) |
| Feed event rate (trades, quotes, bars per second) | UNKNOWN | vendor and tier (D-01, D-02) |
| Concurrent use | single-owner pilot per S00-T02 scope; exact pattern UNKNOWN | owner |
| Analysis budget (model calls, cost per day) | UNKNOWN; the spike made no model calls | D-07 |
| Retention periods for sources, events and audit | UNKNOWN | owner, with legal review where needed (D-08) |
| Latency, freshness, RTO and RPO targets | UNKNOWN | D-07 |

## Costs (dated 2026-10-23, USD)

| Item | Cost | Basis |
|---|---|---|
| This spike and local development | 0 paid. Local disk: 936 MB of spike tools plus about 452 MB of images (PostgreSQL 411 MB, NATS 40.7 MB) | measured |
| SEC EDGAR APIs | no fee, account or key found | [provider record](provider-sec-edgar.md) |
| Software licences (Go, PostgreSQL, NATS, Python libraries) | not reviewed here | — |
| Hosting and operations (compute, database, storage, NATS) | **UNKNOWN**; no estimate without a deployment choice | D-06 |
| Market data | **UNKNOWN**; needs vendor quotes | D-01, D-02 |
| Model usage | **UNKNOWN**; none used | D-07 |

No paid resource, account or production system was created.

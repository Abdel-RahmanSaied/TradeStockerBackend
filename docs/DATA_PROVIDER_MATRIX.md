# Data Provider Matrix and Entitlement Gates

**All procurement/credential/coverage approvals are initially OPEN.** Public documentation identifies potential sources, not actual user entitlements. A feed advertised as real time may still require account/exchange permissions.

| Data | US candidate/source | EGX candidate/source | Required evidence before activation |
|---|---|---|---|
| Stable instruments and listings | Approved licensed market-data source | EGX/approved EGX provider | symbol history, delistings, venue mapping, reuse/coverage |
| Daily and intraday prices | Commercial provider selected in D-01 | EGID or contracted approved vendor in D-02 | raw/adjusted data, intervals, historical depth, corporate actions |
| Quotes / executed trades | Approved feed with declared venue coverage | Contracted feed with actual available fields | delay, conditions, timestamps, corrections and entitlement |
| L2 / order book | Optional separately qualified feed | Optional, availability not assumed | snapshot+incremental sequencing, resets/gaps, reconstruction/replay rights |
| Filings / statements | SEC EDGAR and issuer disclosures | Official exchange/issuer reports and permitted vendor sources | publication, amendments, contexts, original documents |
| Macro and policy | Federal Reserve, official statistics, licensed/vintage datasets | CBE/official Egyptian statistics and licensed sources | release/availability, revisions, licensing and source identifiers |
| News and catalysts | Official RSS + licensed news | Official disclosures + licensed local coverage | redistribution/non-display/LLM rights, latency, corrections |
| Market/broker rules | Official exchange/regulatory and actual broker documentation | Official exchange/regulator and actual broker documentation | effective dates, review owner, account applicability |
| Benchmarks / FX | Approved benchmark/FX source | Approved EGX/FX series | total/price return definition, currency/time and usage rights |

SEC provides company submissions and structured financial/XBRL APIs [SRC-06](SOURCES.md). Fed publishes official RSS categories [SRC-07](SOURCES.md). EGID describes its EGX market-data services [SRC-08](SOURCES.md). This pack did not verify a purchased feed, sample payload, broker contract or redistribution right. Direct EGX site access failed during preparation; do not treat its contents as reviewed.

## Strategy capability matrix

| Capability | Swing | Intraday |
|---|---|---|
| Point-in-time daily history | Required | Required |
| Mandatory financial/history assessments | Required | Required, precomputed where valid |
| Fresh actionable price | Required | Required with strict defined latency |
| Quotes/bid-ask execution assumptions | Required if strategy/policy uses them; otherwise approved conservative model | Required for actionable entry baseline |
| 1-minute history | Optional/strategy-specific | Required baseline; insufficient for some flow/profile strategies |
| Trades / tick resolution | Optional | Required for strategies claiming trade-flow signals |
| L2 | Not generally required | Conditional; no L2 means L2 strategy disabled |
| Halts / eligibility / sessions | Required | Required |

## Due diligence

For each provider complete `templates/PROVIDER_DUE_DILIGENCE.md`: contract owner, account tier, exchanges/venues, professional/nonprofessional classification when applicable, real-time versus delayed, non-display use, derived outputs, retention/replay, LLM use, redistribution, limits, reconnect, disaster recovery and pricing date.

A contract may permit internal analysis while forbidding redistribution of underlying data. Telegram message content and dashboard displays must be checked against the actual terms, not a generic assumption.

## Missing data and fallback

Use synthetic fixtures to implement core safely while access is unresolved. Do not scrape restricted sites or label an unlicensed consumer page as a production feed. If a critical live source fails, block affected new entries; keep monitoring with an explicit degraded-data warning. Switching vendors requires compatibility/adjustment/source-parity checks, not silently merging prices.

Maintain independent readiness per market and capability. A lane can receive NO_GO while unrelated implementation continues. No L2 purchase is necessary unless the approved strategy demonstrates a need.

# Sources and verification boundaries

Checked on **2026-10-23** for this planning pack. These are official technical references or official provider pages, not evidence that the user owns access or that repository code implements the documented capability. Recheck current docs and pinned-version behavior during implementation.

| ID | Primary source | Supports / limitation |
|---|---|---|
| SRC-01 | [NATS JetStream documentation](https://docs.nats.io/learn/jetstream/) | Persistence, consumers and acknowledgment model; business idempotency still belongs to the application |
| SRC-02 | [LangGraph persistence](https://docs.langchain.com/oss/python/langgraph/persistence) and [functional API](https://docs.langchain.com/oss/python/langgraph/functional-api) | Persistent checkpoints and replay; consequential side effects need idempotency/reconciliation |
| SRC-03 | [PostgreSQL numeric types](https://www.postgresql.org/docs/current/datatype-numeric.html) | Exact numeric representation versus inexact floating-point |
| SRC-04 | [AWS transactional outbox pattern](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html) | DB/event dual-write problem and duplicate-message handling |
| SRC-05 | [Telegram Bot API](https://core.telegram.org/bots/api) | sendMessage, incoming updates and webhook secret support; no end-to-end exactly-once promise is assumed |
| SRC-06 | [SEC EDGAR APIs](https://www.sec.gov/search-filings/edgar-application-programming-interfaces) | Public company submissions and structured XBRL data; comply with current fair-access requirements |
| SRC-07 | [Federal Reserve RSS](https://www.federalreserve.gov/feeds/feeds.htm) | Official policy, speeches and statistics feeds, not a complete licensed economic/news product |
| SRC-08 | [EGID official site](https://www.egidegypt.com/) | Describes EGX market-data services; payloads, licensing, depth and history still need contractual verification |
| SRC-09 | [GitHub Actions OIDC for AWS](https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-in-aws) | Scoped short-lived cloud authentication for CI/CD |
| SRC-10 | [sqlc with pgx](https://docs.sqlc.dev/en/stable/guides/using-go-and-pgx.html) | SQL generation integration; confirm exact version compatibility in S00 |

## Not verified

No local Mac repository, source tree, Git status, broker account, market-data subscription, sample licensed payload, current fee table, tax treatment or exchange eligibility list was inspected. A direct attempt to open the EGX homepage failed; do not treat that page as read. Official exchange/regulator/broker sources must be verified as part of D-03.

The architecture choices, sprint decomposition, task dependencies and acceptance criteria are proposed engineering design synthesized from the user's approved scope. They are not external claims of profitability, performance or compliance. No returns, win rates, live readiness, vendor prices or exact runtime versions have been invented.

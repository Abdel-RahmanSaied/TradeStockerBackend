# TradeStocker — local paper increment

A working private **PAPER-only** engineering prototype. Go owns financial state; PostgreSQL persists it; the sibling `TradeStockerFrontend` serves the React/TypeScript dashboard. Both services bind to loopback. There is no brokerage connection, live feed, real recommendation service or external Telegram delivery.

## Run

From this directory:

```sh
./scripts/dev.sh
```

This starts the local PostgreSQL container, builds the API and installs/starts the sibling frontend. Open `http://127.0.0.1:5173`. In **Access**, enter `TS_LOCAL_DEV_TOKEN` from `.env.local`. That file is created with random local credentials and mode 0600; never commit or share it. The dashboard keeps the token only in memory, so a reload requires reconnecting. Database password/token are never bundled into frontend assets.

Optional demo setup, in another terminal:

```sh
./scripts/seed-paper.sh
```

This creates isolated synthetic USD/EGP balances and refreshes invented test observations. These are not capital supplied by the owner or real asset prices. Repeating account setup does not add more cash.

Try **Paper portfolio → Evaluate candidate**: AAPL, US_SWING, 1 share, entry 200.00, stop 195.00 against the synthetic USD account. The backend reserves funds only if its paper checks pass. **Acknowledge** does not create a position. **Record paper fill** requires separate human confirmation. **Reduce paper position** records a human paper sale and cannot short. MISSING, STALE and Intraday examples fail closed. All four live lanes stay DISABLED. Refresh fixture data if the five-minute freshness window expires.

Ctrl-C stops app processes and keeps the database volume. Logs are in `.local/api.log` and `.local/frontend.log`. To stop PostgreSQL without deleting data: `docker compose --env-file .env.local stop postgres`. `scripts/dev.sh` refuses busy app ports. To run services separately use `scripts/setup-local.sh`, `scripts/run-local.sh`, and `npm ci && npm run dev` in the sibling frontend.

## SEC research (read-only)

The **Research** tab retrieves official SEC annual filings for a US issuer and shows each figure with its source. It needs an SEC contact in `.env.local`, quoted because it has spaces:

```sh
TS_SEC_USER_AGENT='TradeStocker Research you@example.com'
```

Use only a contact the owner approved; restart the API after setting it. Without it, research is refused and everything else works. The backend contacts only `data.sec.gov`, at most once per second. Jobs, original sources and events are kept in PostgreSQL. Reports never make anything eligible for entry or backtesting. The contract is `contracts/research-api.md`; the design is ADR-018.

`./scripts/research-smoke.sh [SYMBOL] [CIK]` runs one real SEC job through the real API on a disposable database and port 18080, then verifies the downloaded originals. It makes two SEC requests.

## Verify

```sh
go test -race ./...
./scripts/test-integration.sh
python3 -B -m unittest discover -s research -p 'test_*.py'
python3 -B docs/tools/test_planctl.py
python3 -B test_install_docs.py
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

In the frontend: `npm test`, `npm run build`, `npm audit --audit-level=high`.

For real-API browser E2E, stop the frontend on 5173 first and run `./scripts/test-e2e.sh` from here. It creates a uniquely named disposable database and API on 18080, runs desktop/mobile browser tests and removes only that temporary database. By default the disposable API has no SEC contact and makes no external request; `TS_E2E_REAL_SEC=1 ./scripts/test-e2e.sh` also runs the real SEC research test (about four SEC requests). While the normal app holds 8080/5173, run it on private ports: `TS_E2E_API_PORT=18090 TS_E2E_FRONTEND_PORT=15173 ./scripts/test-e2e.sh`.

`./scripts/paper-read-bench.sh <base-git-ref>` compares paper read throughput and SQL write counts at a fixed synthetic history between a base commit and the working tree, on the spike's isolated PostgreSQL (55532) and port 18180. The normal paper database is untouched. `npx playwright install chromium` in the frontend is needed on a fresh machine. PostgreSQL must be running and `.env.local` present. Without `TEST_DATABASE_URL`, the integration test is deliberately skipped by plain `go test`; the integration script runs it for real.

## Boundaries and remaining work

The implemented contract is `contracts/local-paper-api.md`; engineering deviation and limitations are in ADR-017. Original production tasks remain in the catalog; this increment does not complete them. The PostgreSQL aggregate is durable and transactionally locked, but is deliberately a low-throughput prototype with no history retention or production relational/event architecture. Expiry runs on API access, not a background scheduling service. Full fills only. Paper fees are zero-fee test assumptions; there are no reported performance returns.

SEC research covers structured annual facts only (no quarters, notes, auditor opinions, prices or EGX) and always ends INSUFFICIENT_DATA for entry. The paper workflow's `POST /research/assessments` is still a stub returning INSUFFICIENT_DATA. Paper financial/history fixtures are invented test evidence. The risk policy is an unapproved **paper test fixture**, not investment advice or approved production limits. Real feeds/licensing, actual accounts/settlement, D-04 risk approval, identity, Telegram pairing, full research/evaluation, operational recovery and independently reviewed live release are unresolved. Intraday remains blocked. No external actions or deployment were performed.

CI workflows are scaffolding checked locally; no remote CI run, push or deploy is claimed. The original installer manifest is historical; do not apply the package installer over this live repo. Installer tests now generate an isolated disposable package fixture and retain tamper/conflict/symlink checks. Planning tests generate a disposable bootstrap state and never reset real work.

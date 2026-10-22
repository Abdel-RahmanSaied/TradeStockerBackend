# Dashboard evidence - S00-T08

Date: 2026-10-23. Implementer/task owner: codex-agent. Independent reviewer: pending. Frontend is in the owner-requested sibling TradeStockerFrontend directory, with a new separate local Git repository. No push or hosting was performed.

React/TypeScript/Vite dashboard has Overview, Market data, Decisions, Paper portfolio, Audit trail and Settings. All views use the real Go dashboard API. Fixture prices and assessments are visibly synthetic; paper balances are labeled, stale/missing data is shown and no returns/win rates are invented. Four live lanes remain disabled. Entry evaluation, acknowledgement, human paper fill, cancellation, reduction, entry pause, local audit and Telegram previews are functional. Accounts/currencies remain separate. No brokerage UI or orders exist.

The API token remains in memory only, not local/session storage or a VITE build variable. Lost/ambiguous mutation responses preserve their idempotency key for retry within the tab. Dialogs have focus containment, keyboard dismissal and visible backend errors. Buttons prevent concurrent UI writes, and unverified/non-PAPER API state disables mutations. The backend remains authoritative.

Actual verification on the implementation tree:

- `npm test`: 6 tests PASS (request/auth/idempotency, rejection, gateway errors, ambiguous response retry, missing financial values and freshness states).
- `npm run build`: TypeScript and Vite PASS.
- `npm audit --audit-level=high`: 0 vulnerabilities.
- Backend `scripts/test-e2e.sh`: 2 Playwright tests PASS, against a real separate Go process/PostgreSQL temporary database. Full desktop scenario: fixture ingestion, account creation, visible duplicate-account error, evaluation/reservation, acknowledgement without a position, human fill/cash change, missing-evidence rejection, paper reduction/cash change, pause and audit/dry-run outbox. Mobile navigation and token clearing on reload also pass. No API mocking/interception.
- Screenshot of the real tested dashboard was rendered and visually inspected; it contains synthetic paper data only. It is in the Codex task outputs as tradestocker-dashboard.png.
- Generated private API token checked absent from the bundle. No runtime external fonts/assets requested.

Earlier E2E attempts failed because test selectors used visible text rather than accessible names or exact text on a parent with extra text. Those selectors were corrected; final tests pass. Initial unit runner also picked up Playwright tests; unit script now explicitly scopes src. No passing results were claimed for those failed attempts.

Frontend commit and backend contract commit are recorded in the final handoff. The frontend CI scaffold runs build/unit/audit; cross-repository remote E2E and production identity are not configured. Static hosting/deployment, live features and full S13 production scope are not claimed complete. Use backend README and the task runbook for exact startup/tests. Independent review still required.

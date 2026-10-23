# Provider due diligence — SEC EDGAR APIs / US filings

A filings source, **not a market-data provider**. It does not qualify any price feed and does not resolve D-01 (US prices) or D-02 (EGX). Statements marked "SEC states" were read on the official pages listed at the end on 2026-10-23.

| Field | Record |
|---|---|
| Owner / verified date / next review | `claude-code-agent` for S07-T07 / 2026-10-23 / before any commercial, LLM or production use |
| Product/tier and account entitlement | `data.sec.gov` submissions and XBRL companyfacts JSON. SEC states: "These APIs do not require any authentication or API keys to access." No account is used. |
| Contract reference (no secrets) | None; public government information. Owner approved the User-Agent contact `abdo.am169@gmail.com` (development handoff). |
| Covered venues, consolidated versus venue-only | Not applicable: no trading venues. Covers SEC registrants by 10-digit CIK. No EGX issuers. |
| Instruments and historical universe/delistings | Tickers come from each registrant's submissions document. UNKNOWN: coverage for deregistered issuers and ticker history. The user must supply the CIK; no ticker-to-CIK lookup is implemented. |
| Bars/trades/quotes/L2 and actual timestamps/delay | None. No prices of any kind. Filing acceptance times are present in the submissions index; we use them as an availability proxy, not as proof of first public availability. |
| Historical depth, revisions and corporate actions | SEC states the recent index holds "at least one year's of filing or to 1,000 (whichever is more)", with older filings in extra files. Observed for Apple: 1,002 recent filings back to 2015, plus one older page (1994–2015). companyfacts keeps one row per filing for each period, so later restatements are visible. Corporate actions: not applicable. |
| Streaming reconnect, sequence gaps, snapshots and rate limits | No streaming. SEC states the JSON is "updated throughout the day, in real time, as submissions are disseminated". SEC states the "current maximum access rate is 10 requests per second" and that it may "block IP addresses that submit excessive requests". SEC also states: "Once the rate of requests has dropped below the threshold for 10 minutes, the user may resume accessing content on SEC.gov." Our client sends at most 1 request per second, and after a 403/429 pauses every job for 10 minutes, which follows that guidance but does not guarantee an unblock. |
| Halts, sessions and eligibility coverage | Not applicable. |
| Non-display/internal use rights | SEC states: "Information presented on sec.gov is considered public information and may be copied or further distributed by users of the web site without the SEC's permission." The page names sec.gov and does not mention data.sec.gov separately. This is not a legal review. |
| Display/Telegram/derived-data redistribution rights | Same public-information statement. Commercial advisory delivery still needs the owner's D-08 review. |
| Backtest/replay/retention/disaster-recovery rights | Copies may be kept (public information). companyfacts is a **current snapshot**, not a point-in-time archive, so reports are never certified for historical backtesting. |
| LLM transmission/processing rights | No SEC restriction found on the policy page; not a legal review. Today no LLM receives SEC data. |
| Security and geographical restrictions | SEC states it monitors traffic, does not allow "unclassified" bots, and asks clients to declare a user agent such as "Sample Company Name AdminContact@<sample company domain>.com". The contact is configured in the ignored `.env.local`. |
| Costs with currency/date and source | No fee, account or key found on the SEC pages above (2026-10-23). Local hosting cost only. |
| Sample payload and conformance-test references | Real samples for Apple (CIK 0000320193), 2026-10-23 through the real API: submissions 163,917 bytes, SHA-256 `47e4383b0133a4146cc631725f9aa28bac6d5679d504f735a78f1a1923c4b622`; companyfacts 3,789,099 bytes, SHA-256 `73a86c6aedc31f77cac2ea4df5f80f0b3bd7e6eb58bb4e01444fbedf3afb9c43`. Parsed by `sec-annual-v2` into 5 annual periods. See [SEC research evidence](sec-research.md). |
| UNKNOWN fields and blockers | Which HTTP status SEC uses for rate limits (we treat 403 and 429 as limits); deregistered-issuer coverage; how long after acceptance a filing is publicly disseminated (acceptance time is our availability proxy); whether data.sec.gov falls under the sec.gov policy text (likely, not confirmed in writing). |
| Authorized verdict and lane/capability scope | **Qualified for local, read-only US annual filing research in S07-T07.** Not a price source; satisfies no lane activation gate. All four lanes stay DISABLED. |

Official pages read:

- <https://www.sec.gov/search-filings/edgar-application-programming-interfaces>
- <https://www.sec.gov/about/developer-resources>
- <https://www.sec.gov/about/webmaster-frequently-asked-questions>
- <https://www.sec.gov/about/privacy-information> (reached from https://www.sec.gov/privacy)

# Task evidence — S00-T04 (in progress)

Account, settlement and market-rule research. This record lists what official sources say today, how each rule will be sourced and re-checked, and what is still unknown. **It does not describe any real broker account.** The owner's brokers, account types and permissions are unknown; every lane that depends on them stays blocked.

| Field | Value |
|---|---|
| Task / owner / reviewer | `S00-T04` / `claude-code-agent` / independent review pending |
| Checked | 2026-10-23, from the sources linked in each row |
| Decisions it feeds | D-03 (accounts, settlement, same-session capability), D-08 (private pilot vs commercial advisory); both stay OPEN |

Status words used below:

- **VERIFIED**: read today on the official source named in the row.
- **SECONDARY**: only a non-official source, a search-engine summary, or an official page that could not be opened directly.
- **UNKNOWN**: no usable source yet.

## 1. Rules and their sources

Rules are effective-dated. Code must load them as versioned data with a source, a "checked on" date and a re-check date. It must never hard-code today's values.

### United States

| Rule | What the source says | Status | Source | Re-check by |
|---|---|---|---|---|
| Regular session | NYSE core session 9:30 a.m. – 4:00 p.m. ET | VERIFIED | [NYSE hours and calendars](https://www.nyse.com/markets/hours-calendars) | each calendar change; at latest 2027-01-01 |
| Holidays 2026 | Jan 1, Jan 19, Feb 16, Apr 3, May 25, Jun 19, Jul 3, Sep 7, Nov 26, Dec 25 | VERIFIED | same | same |
| Holidays 2027 | Jan 1, Jan 18, Feb 15, Mar 26, May 31, Jun 18, Jul 5, Sep 6, Nov 25, Dec 24 | VERIFIED | same | same |
| Early closes | 1:00 p.m. ET on 2026-11-27, 2026-12-24 and 2027-11-26 | VERIFIED | same | same |
| Settlement | T+1 for most broker-dealer transactions since 2024-05-28 | VERIFIED | [SEC press release 2024-62](https://www.sec.gov/newsroom/press-releases/2024-62) | yearly |
| Cash account free-riding | Selling a security before it has been paid for in full (free-riding) withdraws the delayed-payment privilege for 90 calendar days (12 CFR 220.8(c)(1)). The freeze does not apply when, within the payment period, full payment is received and the sale proceeds are not withdrawn first, or when the security went to another broker's cash account holding enough funds (220.8(c)(2)). Buying with unsettled proceeds is a separate case; the rule does not freeze every such purchase. How the owner's broker applies this is UNKNOWN. | VERIFIED (rule text) | [eCFR 12 CFR 220.8](https://www.ecfr.gov/current/title-12/chapter-II/subchapter-A/part-220/section-220.8), read through the [official eCFR API](https://www.ecfr.gov/api/versioner/v1/full/2026-10-01/title-12.xml?part=220&section=220.8) (version 2026-10-01). The coordinator also checked the [SEC investor bulletin on cash accounts](https://www.investor.gov/introduction-investing/general-resources/news-alerts/alerts-bulletins/investor-bulletins/updated-9) (updated 2026-08-07); it returned HTTP 403 to the author, so it was not read here. | yearly |
| Pattern day trading | FINRA replaces the day-trading margin rules: no day-trade count test and no $25,000 minimum. New intraday margin standards apply to margin accounts (not good-faith or portfolio margin accounts). Effective 2026-06-04; firms may phase in until 2027-10-20. | VERIFIED | [FINRA Regulatory Notice 26-10](https://www.finra.org/rules-guidance/notices/26-10) (2026-04-20) | 2027-10-20, then with the broker's actual rule |
| Market-wide circuit breakers | S&P 500 down 7% or 13%: 15-minute halt before 3:25 p.m. ET; down 20%: halt for the rest of the day | VERIFIED | [Nasdaq Trader: market-wide circuit breaker](https://www.nasdaqtrader.com/trader.aspx?id=CircuitBreaker) | yearly; a 2026 SEC filing proposes changing Level 3 resumption |
| Single-stock pauses (LULD) and halt codes | Price-band pauses and halt codes exist; details not read on the official page today | SECONDARY | [LULD Plan](https://www.luldplan.com/), [Nasdaq halt codes](https://nasdaqtrader.com/trader.aspx?id=tradehaltcodes) | before S03-T04 |
| SEC Section 31 fee | $20.60 per million dollars of covered sales, for charge dates on or after 2026-04-04, until 60 days after the FY2027 appropriation | VERIFIED | [SEC fee rate advisory 2026-2](https://www.sec.gov/rules-regulations/fee-rate-advisories/2026-2) | each new advisory |
| FINRA trading activity fee, broker commissions | Not read today | UNKNOWN | FINRA Schedule A; the owner's broker fee schedule | before any cost model (D-09) |
| Taxes | Depend on the owner's tax residency and the account; not researched for the owner | UNKNOWN | IRS Publication 515 (withholding for non-resident aliens) and a qualified adviser | before any after-tax figure |

### Egypt (EGX)

The official EGX website did not accept connections from this machine on 2026-10-23 (two attempts, plus one through the web fetch tool). The MCDR website failed TLS certificate verification. Neither was used.

| Rule | What the sources say | Status | Source | Re-check by |
|---|---|---|---|---|
| Session | A search-index view of the official EGX trading-hours page shows: discovery from 09:30 (random end 09:50–10:00), continuous 10:00–14:15, closing auction 14:15–14:25, trade-at-close 14:25–14:30, Sunday–Thursday, different hours in Ramadan. Other third-party pages say continuous trading ends at 14:30. | SECONDARY (conflicting) | [EGX trading hours](https://www.egx.com.eg/en/Trading_Hours.aspx), not opened directly | blocker until read directly or confirmed by the owner's broker |
| Holidays | No official calendar read | UNKNOWN | EGX notices | blocker for EGX sessions |
| Price limits | A search summary of an EGX page mentions ±20% for the main market; not read directly | SECONDARY | EGX trading-system page | before S02-T03 |
| Settlement | Third-party custodian profiles say T+2 (delivery versus payment) | SECONDARY | MCDR (unreachable); RBC Investor Services market profile | blocker for EGX settlement modelling |
| Same-day (intraday) trading | Governed by FRA Board Decree 67 of 2012 and later amendments; only FRA-licensed brokers, with specific client forms. Names come from a search view of EGX's membership-rules page. | SECONDARY | [EGX membership rules](https://www.egx.com.eg/en/Membership_rules.aspx) | blocker for EGX_INTRADAY |
| Capital gains tax (listed shares) | Abolished and replaced by a proportional stamp duty | VERIFIED (announcement only) | [FRA news, 2026-08-03](https://fra.gov.eg/en/fra_news/%D8%AF-%D8%A5%D8%B3%D9%84%D8%A7%D9%85-%D8%B9%D8%B2%D8%A7%D9%85-%D8%B1%D8%A6%D9%8A%D8%B3-%D8%A7%D9%84%D8%B1%D9%82%D8%A7%D8%A8%D8%A9-%D8%A7%D9%84%D9%85%D8%A7%D9%84%D9%8A%D8%A9-%D9%8A%D9%84%D9%82%D9%8A) | when the Official Gazette text is found |
| Stamp duty | 0.05% from each of buyer and seller; 0.025% for each party on intraday trades; market makers exempt | VERIFIED (announcement only) | same FRA news item | same |
| Law number and effective date | Professional summaries name Laws 151–153 of 2026 (EY, PwC). Reported effective dates differ (2026-06-23 vs later). | SECONDARY (conflicting) | Official Gazette (not found) | blocker for any EGX after-cost figure |

The two tax rows marked "announcement only" record what the regulator announced. They are not the effective law: the rates, scope and start date count only after the Official Gazette text is read.

## 2. Unknowns that block activation

None of these can be inferred. The Alpaca Basic account the owner created does not answer them: it is considered here only as a candidate data source, and its account type, key scopes and data entitlements are unverified.

| Unknown | Blocks | Who answers |
|---|---|---|
| Which broker(s) the owner uses for US and for EGX, and in which country each account is held | D-03; every live or personalized lane | Owner |
| Account type per market: cash, margin, good-faith or other | US_SWING, US_INTRADAY (in a cash account, selling before the purchase is paid for can trigger the 90-day freeze unless an exception applies) | Owner, then broker documents |
| Whether the broker has already applied FINRA's new intraday margin standard (phase-in until 2027-10-20) | US_INTRADAY | Broker documents |
| Whether the EGX account is enabled for same-day trading, and the broker's licence for it | EGX_INTRADAY | Owner, broker |
| Account currencies, funding and FX path between EGP and USD | D-04 capital rules, any cross-market allocation | Owner |
| Fees and commissions actually charged | Any net-of-cost backtest or result (D-09) | Broker fee schedule |
| Tax residency and treatment | Any after-tax figure | Owner with a qualified adviser |
| EGX session, holidays, settlement and price limits from an official source | EGX_SWING, EGX_INTRADAY | Direct read of EGX/MCDR/FRA, or broker confirmation |
| Private pilot or commercial advice; whether a legal review is needed | D-08; any delivery to anyone other than the owner | Owner, with counsel if not private |

## 3. Hypothetical account capability matrices

These two tables are examples of what must be filled in. They are **not** the owner's accounts and **not** verified broker behaviour. "Rule" is the public rule above; "This account" stays UNKNOWN until the owner supplies statements or broker documents.

An account that allows margin, short selling or derivatives does not let the platform use them. The baseline prohibits leverage, short selling, derivatives and scalping (Source of Truth INV-07), whatever the account permits.

US account (example: one cash account and one margin account at the same broker):

| Capability | Rule (public) | Cash account: this account | Margin account: this account |
|---|---|---|---|
| Settlement | T+1 | UNKNOWN | UNKNOWN |
| Sell a purchase before it is paid for | Cash account: free-riding, 90-day freeze unless a 220.8(c)(2) exception applies. Margin account: FINRA intraday margin standard. | UNKNOWN (broker's handling) | UNKNOWN (broker phase-in) |
| Short selling | Usually needs a margin account (rule not read today) | UNKNOWN | UNKNOWN |
| Extended hours | Broker-specific | UNKNOWN | UNKNOWN |
| Fractional shares | Broker-specific | UNKNOWN | UNKNOWN |
| Fees | Section 31 on sales plus broker fees | UNKNOWN | UNKNOWN |
| Statements available | Broker-specific | UNKNOWN | UNKNOWN |

EGX account (example: one standard account and one account enabled for same-day trading):

| Capability | Rule (public) | Standard: this account | Same-day enabled: this account |
|---|---|---|---|
| Settlement | T+2 (SECONDARY) | UNKNOWN | UNKNOWN |
| Buy and sell the same day | Only through the FRA intraday mechanism (SECONDARY) | UNKNOWN (expected: no) | UNKNOWN |
| Short selling | FRA news titles mention a short-selling mechanism being launched (pages not read) | UNKNOWN | UNKNOWN |
| Stamp duty | 0.05% per side; 0.025% per side intraday | UNKNOWN | UNKNOWN |
| Statements available | Broker-specific | UNKNOWN | UNKNOWN |

## 4. How account data may enter the system

- **Allowed:** statements or trade confirmations exported by the owner (CSV or PDF), imported manually and checked against the ledger. An optional read-only data API may be added later, only after its own review.
- **Never:** trading credentials, order-entry keys, or any broker API that can place, change or cancel orders (ADR-005 human-only execution). The owner's Alpaca Basic account is considered only as a candidate data source; its account type, key scopes and entitlements are unverified. Its order API must not be integrated, and no key has been requested or received.
- Imported records must keep their source file hash, import time and the account they claim to describe.

## 5. Questions for the owner (D-03, D-08)

1. Which broker do you use for US shares, and which for EGX? In which country is each account?
2. For each: cash or margin account? Is same-day trading enabled?
3. Which currencies do you hold, and how do you move money between EGP and USD?
4. Is this a private tool for you only, or will anyone else ever receive its recommendations? If anyone else, a legal review comes first (D-08).
5. Can you export statements (CSV or PDF) from each broker? That is the only account input planned.

## Limits

- Several EGX rules could not be read on an official page today; they stay SECONDARY or UNKNOWN until someone reads them directly.
- No tax advice is given. The figures are announcements and rule texts, not a tax position for the owner.
- A verification owner for production must be named under D-03. Until then, the author re-checks on the dates above and the reviewer confirms.

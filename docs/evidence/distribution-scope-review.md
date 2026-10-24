# Distribution scope — the owner's answer and what it changes

| Field | Value |
|---|---|
| Task / decision | S00-T04 acceptance criterion 3; D-08 (private pilot vs commercial advisory, legal review, data distribution) |
| Prepared by | `claude-code-agent` (session `2026-10-23-development-8da61cee`), 2026-10-24 |
| Status | **Assessment for review. D-08 stays OPEN.** Nothing here allows sending, sharing, accounts, sign-in or any live lane. |

## 1. The owner's answer

Question 5 of the [account-rule research](broker-rules-research.md) asked who receives recommendations: (a) only the owner, (b) a closed group of named people, or (c) the public or paying clients. The owner answered:

> «لمجموعة مغلقة من أشخاص محددين» — "for a closed group of specified people".

Provenance: the coordinator relayed this as the owner's direct answer in CODEX-BRIDGE message 057 on 2026-10-24. The author did not see the owner's message itself.

## 2. What the answer settles, and what it does not

**Settled:** the intended audience is neither the owner alone nor the public. It is a closed group of specified people.

**Not settled.** None of these is inferred from the answer:

- whether the group pays or gives anything in return, directly or indirectly;
- how many people are in it, and the countries where they live;
- who they are (family, friends, colleagues or clients);
- whether everyone gets the same recommendations, or recommendations for their own accounts;
- whether members see prices and charts, or only the recommendation text;
- whether any member is a finance professional;
- whether the data used may be passed on to them.

**Assessment of criterion 3** ("Determine whether the pilot remains private and whether commercial advisory review is required"):

- The pilot does not stay private to the owner: the intended audience is a closed group.
- A scoped review is required before anything is delivered to the group. D-08 already requires "a documented scoped review" for delivery beyond the owner, and section 3 shows that advice and data rules in both markets can depend on facts that are still unknown.
- The review itself has not been done. A model's analysis is not legal approval. D-08 stays OPEN, and delivery stays limited to the owner until the review is recorded under D-08's own authority.

Whether this assessment satisfies criterion 3 is for the reviewer and the coordinator to judge. The author does not mark it met.

## 3. What a legal and regulatory review must answer

These texts show that the questions are real. They do not answer them; that needs qualified counsel for each country involved.

| Market | What the source says | Why it matters here | Source |
|---|---|---|---|
| United States | An "investment adviser" is "any person who, for compensation, engages in the business of advising others, either directly or through publications or writings, as to the value of securities or as to the advisability of investing in, purchasing, or selling securities", with listed exclusions. One exclusion covers the publisher of a bona fide newspaper, news magazine or business or financial publication "of general and regular circulation". | Whether the group pays, and whether this is a "business", are unknown, so whether the definition applies is unknown. State rules were not researched. | [15 U.S.C. 80b-2(a)(11)](https://www.govinfo.gov/content/pkg/USCODE-2023-title15/html/USCODE-2023-title15-chap2D-subchapII-sec80b-2.htm), US Code 2023 edition (read 2026-10-24) |
| Egypt | Article 125 of the updated executive regulations of Capital Market Law 95 of 1992 lists, among activities under that law, "valuation and analysis of securities" and "publishing information about securities". For a company doing them, it sets the issued and paid capital at EGP 5 million, paid in full in cash. Article 120 lists the activities of companies working in the securities field. | Whether sending recommendations to a closed group counts as one of these activities is a legal question this research does not answer. | [FRA's updated copy of the executive regulations](https://fra.gov.eg/wp-content/uploads/2024/07/%D9%84%D8%A7%D8%A6%D8%AD%D8%A9-%D8%A7%D9%84%D8%AA%D9%86%D9%81%D9%8A%D8%B0%D9%8A%D8%A9-%D9%85%D9%86-%D9%82%D8%A7%D9%86%D9%88%D9%86-%D8%B3%D9%88%D9%82-%D8%A7%D9%84%D9%85%D8%A7%D9%84-%D8%A8%D8%B5%D9%8A%D8%BA%D8%A9-WORD-4.pdf) (176 pages, read 2026-10-24; pages 62–63) |
| Members' own countries | Unknown | Each country where a member lives may have its own rules. | — |

## 4. Data rights

Sending a recommendation can also pass on the data behind it. Each source has its own terms.

| Data | What is known | Effect |
|---|---|---|
| SEC filings (research reports) | The SEC states that information on sec.gov "is considered public information and may be copied or further distributed" without its permission. It asks for citation, and limits use of its seal, logos and trademarks. The page does not address third-party material inside filings. | SEC-sourced facts look shareable, with citation. Third-party content inside filings is not covered by that statement. Source: [SEC privacy and website information](https://www.sec.gov/about/privacy-information), read 2026-10-24. |
| US market prices | No vendor is chosen (D-01). The Alpaca Customer Agreement (version V26.2026.07, file last modified 2026-07-30), section 30, says: "I agree not to reproduce, distribute, sell or commercially exploit the market data in any manner without written consent from Alpaca." It also incorporates the "non-professional" definitions of the Nasdaq and NYSE subscriber agreements. | Whether the owner's Alpaca Basic account is governed by this agreement is **not verified**. If it is, showing its prices to the group would need Alpaca's written consent. Source: [Alpaca customer agreement](https://files.alpaca.markets/disclosures/library/AcctAppMarginAndCustAgmt.pdf), SHA-256 `d086d69f…`. |
| EGX market data | No vendor is chosen (D-02). EGID licensing needs contractual verification ([SRC-08](../SOURCES.md)). | Unknown. |
| Synthetic PAPER prices | Owned fixtures | No third-party rights, but synthetic outputs are never advice. |

Effect on D-01 and D-02: any data vendor chosen must allow display or redistribution to the named recipients. A vendor may also distinguish professional and non-professional users, so each member's status may matter.

## 5. Security, identity and privacy effects

- **Identity (D-05).** The options document assumed one owner plus a reviewer. A group adds one identity per member, with a recipient role: read-only, no administration, no paper or ledger writes. Removing a member must revoke their access at once.
- **Telegram (D-10).** Each member's chat is paired and revoked one by one. One member's messages never contain another person's account data.
- **Account data (D-03, D-04).** If recommendations are tailored to each member, each member needs their own account facts, capital and risk limits, kept account-scoped. If they are not tailored, messages must not suggest they suit each member. Which applies is unknown.
- **Personal data.** Members' names and Telegram IDs are personal data. The obligations in their countries were not researched.
- **Audit.** Who received which recommendation, and when, must be recorded. The delivery state machine in `docs/CONTRACTS.md` already separates sent from read and from acted on.
- **Lanes.** All lanes stay DISABLED. Nothing may be sent to the group until the D-08 review is recorded, D-10 is resolved and a lane is separately activated by its own authority.

## 6. Inputs the review will need from the owner

These are listed for the coordinator to relay when appropriate. They have not been asked here.

1. Will the group pay or give anything in return, now or later?
2. About how many people, and in which countries do they live?
3. Who are they: family and friends, colleagues, or clients?
4. Same recommendations for everyone, or tailored to each person's account?
5. Will they see prices and charts, or only the recommendation text?
6. Does any member work in finance?

## 7. Effect on open decisions

| Decision | Effect of the answer | Status |
|---|---|---|
| D-08 | Scope is now a closed group; a scoped legal review is required before delivery | OPEN |
| D-01, D-02 | A vendor must allow display or redistribution to named recipients | OPEN |
| D-05 | Per-member identities with a recipient role | OPEN |
| D-10 | Per-member Telegram pairing and revocation | OPEN |
| D-03, D-04 | Per-member account facts and limits only if recommendations are tailored | OPEN |

## Source copies

Saved outside the repository, in the author's session scratch area, and not committed. All were fetched on 2026-10-24 with normal TLS verification, except that 15 U.S.C. 80b-2 and the SEC website page were read through the web fetch tool, so no copy was saved.

| Source | SHA-256 |
|---|---|
| FRA updated executive regulations of Capital Market Law 95/1992 (PDF, 176 pages) | `84fa8ea2073e73166c6974a5dcc4522776f339a013c1ba707725439566233072` |
| Alpaca customer agreement V26.2026.07 (PDF) | `d086d69f063bba1e9693fffbb3185c4469b24b47bda79f587053bd771580fcf6` |

## Limits

- No legal, regulatory or tax advice is given. The texts quoted show which questions exist, not their answers.
- Only the US federal definition and the Egyptian executive regulations were read. US state law, other Egyptian texts and the members' countries were not researched.
- The owner's answer arrived through the coordinator; the author did not see it directly.

"""Writes the synthetic development fixtures, byte for byte the same every time.

Everything here is invented for tests and local development. The issuer, its
numbers and its filings do not exist; no real market, filing or account data is
used, so no data licence applies. Run from the repository root:
    python3 -I fixtures/synthetic/generate.py fixtures/synthetic
The expected research results (research/expected-results.jsonl) come from
running research-worker on research/requests.jsonl; test_fixtures.py checks both.
"""
import hashlib
import json
from pathlib import Path
import sys

NOTICE = "SYNTHETIC: invented for local development and tests; not real market, filing or account data"
CIK = "0000000001"
SYMBOL = "SYN.TEST"
ISSUER = "Synthetic Test Issuer (invented)"
AS_OF = "2026-10-23T00:00:00Z"
FY24 = ("0000000001-24-000010", "2024-11-01", "2024-09-28", "2023-10-01", "2024-11-01T14:01:36.000Z")
FY25 = ("0000000001-25-000020", "2025-10-31", "2025-09-27", "2024-09-29", "2025-10-31T14:01:26.000Z")


def dumps(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def annual(accn, filed, end, start, revenue, income, cash, assets, liabilities, equity):
    def flow(v):
        return {"start": start, "end": end, "val": v, "accn": accn, "fy": int(end[:4]), "fp": "FY", "form": "10-K", "filed": filed}

    def instant(v):
        return {"end": end, "val": v, "accn": accn, "fy": int(end[:4]), "fp": "FY", "form": "10-K", "filed": filed}
    return {"RevenueFromContractWithCustomerExcludingAssessedTax": [flow(revenue)], "NetIncomeLoss": [flow(income)],
            "NetCashProvidedByUsedInOperatingActivities": [flow(cash)], "Assets": [instant(assets)],
            "Liabilities": [instant(liabilities)], "StockholdersEquity": [instant(equity)],
            "LiabilitiesAndStockholdersEquity": [instant(liabilities + equity)]}


def sec_request():
    tags: dict[str, list[dict]] = {}
    for part in [annual(*FY24[:4], 1000, 100, 150, 5000, 3000, 2000),
                 annual(*FY25[:4], 1200, 120, 130, 5500, 3200, 2300),
                 annual(FY25[0], FY25[1], FY24[2], FY24[3], 1000, 100, 150, 5000, 3000, 2000)]:
        for tag, rows in part.items():
            tags.setdefault(tag, []).extend(rows)
    recent = {"accessionNumber": [FY24[0], FY25[0]], "form": ["10-K", "10-K"], "filingDate": [FY24[1], FY25[1]],
              "reportDate": [FY24[2], FY25[2]], "acceptanceDateTime": [FY24[4], FY25[4]]}
    submissions = {"cik": CIK, "name": ISSUER, "tickers": [SYMBOL], "sic": "3571", "notice": NOTICE, "filings": {"recent": recent}}
    facts = {"cik": int(CIK), "entityName": ISSUER, "notice": NOTICE,
             "facts": {"us-gaap": {tag: {"units": {"USD": rows}} for tag, rows in sorted(tags.items())}}}
    return {"symbol": SYMBOL, "cik": CIK, "as_of": AS_OF, "submissions": submissions, "companyfacts": facts,
            "submissions_sha256": hashlib.sha256(dumps(submissions).encode()).hexdigest(),
            "facts_sha256": hashlib.sha256(dumps(facts).encode()).hexdigest()}


def files():
    requests = [
        {"task": "sec-annual-v2", "input": sec_request()},
        {"task": "research-stub-v1", "input": {"symbol": SYMBOL, "mode": "PAPER"}},
        # Fails closed: no such task exists, and nothing may place orders.
        {"task": "place-order", "input": {"symbol": SYMBOL}},
    ]
    accounts = {"notice": NOTICE, "accounts": [
        {"currency": "USD", "amount_minor": 1000000, "idempotency_key": "synthetic-fixture-account-v1-USD"},
        {"currency": "EGP", "amount_minor": 10000000, "idempotency_key": "synthetic-fixture-account-v1-EGP"},
    ]}
    return {
        "research/requests.jsonl": "".join(dumps(r) + "\n" for r in requests),
        "paper-accounts.json": json.dumps(accounts, indent=2, sort_keys=True) + "\n",
    }


def main(out):
    out = Path(out)
    for rel, text in files().items():
        path = out / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
    print("wrote %d synthetic fixture files to %s" % (len(files()), out))


if __name__ == "__main__":
    main(sys.argv[1])

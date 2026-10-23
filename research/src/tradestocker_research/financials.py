"""Deterministic SEC annual financial research.

Reads one JSON request on stdin and writes one report on stdout. It has no
network, database, credential or financial-state access. Reports never make an
instrument eligible for entry or backtesting.
"""
import collections
import datetime as dt
from decimal import Decimal, localcontext
import json
import re
import sys

VERSION = "sec-annual-v2"
INPUT_LIMIT = 40 * 1024 * 1024
MAX_PERIODS = 5
ANNUAL_FORMS = ("10-K", "10-K/A")
# Priority-ordered US-GAAP tags. Within one filing the first clean tag wins.
METRICS = {
    "revenue": ["RevenueFromContractWithCustomerExcludingAssessedTax", "Revenues", "SalesRevenueNet",
                "RevenueFromContractWithCustomerIncludingAssessedTax"],
    "net_income": ["NetIncomeLoss"],
    "operating_cash_flow": ["NetCashProvidedByUsedInOperatingActivities"],
    "assets": ["Assets"],
    "liabilities": ["Liabilities"],
    "equity": ["StockholdersEquityIncludingPortionAttributableToNoncontrollingInterest", "StockholdersEquity"],
    "liabilities_and_equity": ["LiabilitiesAndStockholdersEquity"],
}
FLOWS = {"revenue", "net_income", "operating_cash_flow"}
RATIOS = [("net_margin", "net_income", "revenue"), ("cash_conversion", "operating_cash_flow", "net_income"),
          ("liabilities_to_assets", "liabilities", "assets")]
ACCESSION = re.compile(r"\d{10}-\d{2}-\d{6}")
SHA256 = re.compile(r"[0-9a-f]{64}")
BASE_WARNINGS = ["ANNUAL_STRUCTURED_FACTS_ONLY", "NOTES_AND_AUDITOR_NOT_ANALYZED", "LICENSED_PRICE_HISTORY_MISSING",
                 "CURRENT_QUARTERS_NOT_ANALYZED", "CURRENT_SNAPSHOT_NOT_POINT_IN_TIME_ARCHIVE"]
COUNTERCASE = [
    "Structured figures do not establish financial health without notes and auditor review.",
    "Annual figures can be outdated; recent quarterly or material filings may change the assessment.",
    "No price history, liquidity, valuation or expected return is inferred from accounting facts.",
]


def parse_date(value):
    if not isinstance(value, str):
        raise ValueError("date must be a string")
    return dt.date.fromisoformat(value)


FRACTION = re.compile(r"\.(\d+)(?=[+-]\d\d:\d\d$)")


def parse_instant(value):
    if not isinstance(value, str):
        raise ValueError("instant must be a string")
    # Python 3.9 fromisoformat accepts only 3 or 6 fractional digits; normalize to 6.
    text = FRACTION.sub(lambda m: "." + (m.group(1) + "000000")[:6], value.replace("Z", "+00:00"))
    t = dt.datetime.fromisoformat(text)
    if t.tzinfo is None:
        raise ValueError("instant needs a timezone")
    return t.astimezone(dt.timezone.utc)


def parse_cik(value):
    # SEC uses a zero-padded string in submissions and a number in companyfacts.
    if isinstance(value, bool):
        raise ValueError("CIK must not be boolean")
    if isinstance(value, int) and 0 < value < 10**10:
        return value
    if isinstance(value, str) and re.fullmatch(r"[0-9]{1,10}", value) and int(value) > 0:
        return int(value)
    raise ValueError("CIK must be a positive number of at most 10 digits")


def exact_integer(value):
    # Monetary facts must be exact integers; floats and booleans are refused.
    return isinstance(value, int) and not isinstance(value, bool) and abs(value) <= 10**18


def ratio(numerator, denominator):
    with localcontext() as c:
        c.prec = 40
        return str((Decimal(numerator) / Decimal(denominator)).quantize(Decimal("0.000001")))


def midnight(day):
    return dt.datetime.combine(day, dt.time(), dt.timezone.utc)


def validate_request(data):
    if not isinstance(data, dict):
        raise ValueError("request must be an object")
    symbol, cik = data.get("symbol"), data.get("cik")
    if not isinstance(symbol, str) or not re.fullmatch(r"[A-Z][A-Z0-9.-]{0,15}", symbol):
        raise ValueError("symbol is invalid")
    if not isinstance(cik, str) or not re.fullmatch(r"[0-9]{10}", cik) or int(cik) == 0:
        raise ValueError("cik must have 10 digits")
    for key in ("submissions_sha256", "facts_sha256"):
        if not isinstance(data.get(key), str) or not SHA256.fullmatch(data[key]):
            raise ValueError(key + " must be a lowercase SHA-256 hex digest")
    if not isinstance(data.get("submissions"), dict) or not isinstance(data.get("companyfacts"), dict):
        raise ValueError("submissions and companyfacts must be objects")
    as_of = parse_instant(data.get("as_of"))
    sub, facts = data["submissions"], data["companyfacts"]
    tickers = sub.get("tickers")
    if parse_cik(sub.get("cik")) != int(cik) or parse_cik(facts.get("cik")) != int(cik):
        raise ValueError("SEC issuer identity does not match the request")
    if not isinstance(tickers, list) or symbol not in tickers:
        raise ValueError("SEC submissions do not list the requested ticker for this CIK")
    return symbol, cik, as_of


def filing_index(sub):
    """Map accession -> filing metadata from the recent submissions index."""
    recent = sub.get("filings", {}).get("recent", {}) if isinstance(sub.get("filings"), dict) else {}
    index = {}
    if not isinstance(recent, dict):
        return index
    columns = [recent.get(k) for k in ("accessionNumber", "form", "filingDate", "reportDate", "acceptanceDateTime")]
    if not all(isinstance(c, list) for c in columns):
        return index
    for accn, form, filed, report, accepted in zip(*columns):
        try:
            if not isinstance(accn, str) or not ACCESSION.fullmatch(accn):
                continue
            index[accn] = {"form": form, "filed": parse_date(filed),
                           "report": parse_date(report) if report else None, "accepted": parse_instant(accepted)}
        except (ValueError, TypeError):
            continue
    return index


def collect(gaap, index, as_of, sha, excluded):
    """Return clean candidate annual facts; count every exclusion by reason."""
    rows = []
    for metric, tags in METRICS.items():
        for rank, tag in enumerate(tags):
            concept = gaap.get(tag)
            usd = concept.get("units", {}).get("USD", []) if isinstance(concept, dict) and isinstance(concept.get("units"), dict) else []
            if not isinstance(usd, list):
                excluded["MALFORMED_FACT"] += 1
                continue
            for i, row in enumerate(usd):
                if not isinstance(row, dict):
                    excluded["MALFORMED_FACT"] += 1
                    continue
                if row.get("form") not in ANNUAL_FORMS:
                    continue
                try:
                    accn, end, filed = row["accn"], parse_date(row["end"]), parse_date(row["filed"])
                    start = parse_date(row["start"]) if metric in FLOWS else None
                except (KeyError, ValueError, TypeError):
                    excluded["MALFORMED_FACT"] += 1
                    continue
                if not isinstance(accn, str) or not ACCESSION.fullmatch(accn):
                    excluded["MALFORMED_FACT"] += 1
                    continue
                if not exact_integer(row.get("val")):
                    excluded["NON_INTEGER_OR_OUT_OF_RANGE_VALUE"] += 1
                    continue
                if start is not None and not 330 <= (end - start).days <= 400:
                    continue  # Quarterly or other non-annual duration inside an annual report.
                filing = index.get(accn)
                if filing and (filing["form"] != row["form"] or filing["filed"] != filed):
                    excluded["FILING_METADATA_MISMATCH"] += 1
                    continue
                # Availability is never earlier than SEC acceptance or the filing date.
                # Without an indexed acceptance time, assume the following UTC day.
                if filing:
                    available, basis = max(filing["accepted"], midnight(filed)), "SEC_ACCEPTANCE"
                else:
                    available, basis = midnight(filed + dt.timedelta(days=1)), "FILING_DATE_PLUS_ONE_DAY_ESTIMATE"
                if end > filed or (start is not None and start > end):
                    excluded["PERIOD_ENDS_AFTER_FILING"] += 1
                    continue
                if available > as_of:
                    excluded["NOT_AVAILABLE_AT_AS_OF"] += 1
                    continue
                rows.append({"metric": metric, "rank": rank, "tag": tag, "index": i, "accession": accn,
                             "form": row["form"], "start": start, "end": end, "filed": filed, "value": row["val"],
                             "available": available, "basis": basis, "sha": sha})
    return rows


def fact_record(r, cik):
    return {"metric": r["metric"], "value": str(r["value"]), "unit": "USD",
            "start": r["start"].isoformat() if r["start"] else None, "end": r["end"].isoformat(),
            "filed": r["filed"].isoformat(), "available_at": r["available"].isoformat(),
            "availability_basis": r["basis"], "accession": r["accession"], "form": r["form"], "tag": r["tag"],
            "source_sha256": r["sha"], "pointer": "/facts/us-gaap/%s/units/USD/%d" % (r["tag"], r["index"]),
            "filing_url": filing_url(cik, r["accession"])}


def filing_url(cik, accn):
    return "https://www.sec.gov/Archives/edgar/data/%d/%s/%s-index.html" % (int(cik), accn.replace("-", ""), accn)


def select(rows, metric, accn, end):
    """Pick one fact for a metric in one filing. Conflicting values are withheld."""
    for tag in METRICS[metric]:
        matches = [r for r in rows if r["metric"] == metric and r["tag"] == tag and r["accession"] == accn and r["end"] == end]
        if not matches:
            continue
        contexts = {(r["start"], r["value"]) for r in matches}
        if len(contexts) > 1:
            values = sorted({str(r["value"]) for r in matches}, key=int)
            return None, {"metric": metric, "tag": tag, "reason": "CONFLICTING_VALUES" if len(values) > 1 else "AMBIGUOUS_DURATION",
                          "values": values}
        return min(matches, key=lambda r: r["index"]), None
    return None, None


def assess(data):
    symbol, cik, as_of = validate_request(data)
    sub, facts = data["submissions"], data["companyfacts"]
    gaap = facts.get("facts", {}).get("us-gaap", {}) if isinstance(facts.get("facts"), dict) else {}
    if not isinstance(gaap, dict):
        raise ValueError("companyfacts us-gaap facts must be an object")
    index = filing_index(sub)
    excluded = collections.Counter()
    rows = collect(gaap, index, as_of, data["facts_sha256"], excluded)
    warnings = set(BASE_WARNINGS)

    # Each fiscal period is anchored on its original annual report: the filing whose
    # own report date equals the period end. Later comparatives become counter-evidence.
    report_date = {}
    for accn, filing in index.items():
        if filing["report"] is not None:
            report_date[accn] = (filing["report"], "SEC_REPORT_DATE")
    for r in rows:
        if r["accession"] not in report_date or report_date[r["accession"]][1] != "SEC_REPORT_DATE":
            current = report_date.get(r["accession"], (r["end"], "LATEST_FACT_END_ESTIMATE"))[0]
            report_date[r["accession"]] = (max(current, r["end"]), "LATEST_FACT_END_ESTIMATE")
    originals = {}
    for r in rows:
        period, _ = report_date[r["accession"]]
        if r["end"] != period:
            continue
        key = (r["form"] != "10-K", r["filed"], r["accession"])
        if period not in originals or key < originals[period][0]:
            originals[period] = (key, r["accession"], r["form"], r["filed"])

    financial_issuer = str(sub.get("sic", "")).startswith("6")
    if financial_issuer:
        warnings.add("FINANCIAL_ISSUER_GENERIC_RATIOS_WITHHELD")
    periods = []
    for end in sorted(originals, reverse=True)[:MAX_PERIODS]:
        _, accn, form, filed = originals[end]
        selected, withheld = {}, []
        for metric in METRICS:
            fact, problem = select(rows, metric, accn, end)
            if fact:
                selected[metric] = fact
            elif problem:
                withheld.append(problem)
        if withheld:
            warnings.add("CONFLICTING_FACT_VALUES_WITHHELD")
        flows = [selected[m] for m in ("revenue", "net_income", "operating_cash_flow") if m in selected]
        starts = {f["start"] for f in flows}
        if len(starts) > 1:
            # Flow facts must describe the same fiscal year to be compared.
            for f in flows:
                withheld.append({"metric": f["metric"], "tag": f["tag"], "reason": "MISMATCHED_FISCAL_YEAR", "values": [str(f["value"])]})
                del selected[f["metric"]]
            warnings.add("CONFLICTING_FACT_VALUES_WITHHELD")
        values = {m: f["value"] for m, f in selected.items()}
        differences = []
        for metric, f in sorted(selected.items()):
            for later in rows:
                if (later["tag"] == f["tag"] and later["start"] == f["start"] and later["end"] == end
                        and later["accession"] != accn and later["filed"] >= f["filed"] and later["value"] != f["value"]):
                    differences.append({"metric": metric, "tag": f["tag"], "original_value": str(f["value"]),
                                        "later_value": str(later["value"]), "later_accession": later["accession"],
                                        "later_form": later["form"], "later_filed": later["filed"].isoformat()})
        if differences:
            warnings.add("LATER_FILINGS_REPORT_DIFFERENT_VALUES")
        checks = []
        if "assets" in values and "liabilities_and_equity" in values:
            delta = values["assets"] - values["liabilities_and_equity"]
            checks.append({"name": "BALANCE_TOTALS", "status": "PASS" if delta == 0 else "FAIL", "difference_usd": str(delta)})
            if delta:
                warnings.add("BALANCE_TOTALS_MISMATCH")
        else:
            checks.append({"name": "BALANCE_TOTALS", "status": "MISSING_CONTEXT"})
        if all(k in values for k in ("liabilities", "equity", "liabilities_and_equity")):
            delta = values["liabilities_and_equity"] - values["liabilities"] - values["equity"]
            # A gap can be temporary equity or noncontrolling interest; it is shown, not hidden.
            checks.append({"name": "LIABILITIES_PLUS_EQUITY", "status": "PASS" if delta == 0 else "UNEXPLAINED_DIFFERENCE", "difference_usd": str(delta)})
        else:
            checks.append({"name": "LIABILITIES_PLUS_EQUITY", "status": "MISSING_CONTEXT"})
        derived = {}
        if not financial_issuer:
            for name, num, den in RATIOS:
                if num not in values or den not in values:
                    derived[name] = {"value": None, "inputs": [num, den], "status": "MISSING_INPUT"}
                elif values[den] <= 0:
                    derived[name] = {"value": None, "inputs": [num, den], "status": "UNDEFINED_NONPOSITIVE_DENOMINATOR"}
                else:
                    derived[name] = {"value": ratio(values[num], values[den]), "inputs": [num, den], "status": "DERIVED"}
        missing = sorted(set(METRICS) - set(selected))
        if missing:
            warnings.add("INCOMPLETE_ANNUAL_CONTEXT")
        filing = index.get(accn)
        if not filing:
            warnings.add("FILING_TIME_ESTIMATED")
        available = max(f["available"] for f in selected.values()) if selected else None
        kept_starts = {selected[m]["start"] for m in FLOWS if m in selected}
        periods.append({
            "fiscal_period_end": end.isoformat(),
            "start": kept_starts.pop().isoformat() if kept_starts else None,
            "filing": {"accession": accn, "form": form, "filed": filed.isoformat(), "report_date": end.isoformat(),
                       "report_date_basis": report_date[accn][1],
                       "accepted_at": filing["accepted"].isoformat() if filing else None,
                       "available_at": available.isoformat() if available else None,
                       "availability_basis": "SEC_ACCEPTANCE" if filing else "FILING_DATE_PLUS_ONE_DAY_ESTIMATE",
                       "filing_url": filing_url(cik, accn)},
            "facts": [fact_record(selected[m], cik) for m in METRICS if m in selected],
            "derived": derived, "checks": checks, "missing_metrics": missing,
            "withheld_metrics": withheld, "later_filing_differences": differences,
        })
    if not periods:
        warnings.add("NO_ANNUAL_REPORT_CONTEXT")
    if len(periods) < MAX_PERIODS:
        warnings.add("LESS_THAN_FIVE_ANNUAL_PERIODS")
    if excluded["PERIOD_ENDS_AFTER_FILING"]:
        warnings.add("TEMPORALLY_INCONSISTENT_FACTS_EXCLUDED")
    if excluded["MALFORMED_FACT"] or excluded["NON_INTEGER_OR_OUT_OF_RANGE_VALUE"] or excluded["FILING_METADATA_MISMATCH"]:
        warnings.add("MALFORMED_FACTS_EXCLUDED")
    return {
        "schema_version": "2", "version": VERSION, "symbol": symbol, "cik": cik,
        "issuer": sub.get("name", "") if isinstance(sub.get("name"), str) else "",
        "source": "SEC_PUBLIC", "data_kind": "OFFICIAL_FILINGS",
        "status": "PARTIAL" if periods else "INSUFFICIENT_DATA", "outcome": "INSUFFICIENT_DATA",
        "eligible_for_entry": False, "eligible_for_backtest": False, "financial_state_write_access": False,
        "as_of": as_of.isoformat(), "accounting_framework": "US_GAAP",
        "scope": "SEC entity-wide standard taxonomy facts from original annual reports; segment and custom taxonomy facts excluded",
        "currency": "USD", "periods": periods, "warnings": sorted(warnings),
        "excluded_facts": dict(sorted(excluded.items())), "countercase": COUNTERCASE,
        "coverage": {"annual_periods": len(periods), "price_history": False, "notes_and_auditor": False, "quarterly_analysis": False},
        "sources": [{"kind": "submissions", "sha256": data["submissions_sha256"]}, {"kind": "companyfacts", "sha256": data["facts_sha256"]}],
    }


def main():
    raw = sys.stdin.buffer.read(INPUT_LIMIT + 1)
    try:
        if len(raw) > INPUT_LIMIT:
            raise ValueError("research input exceeds limit")
        result = assess(json.loads(raw))
    except (ValueError, TypeError, KeyError, AttributeError) as error:
        print("research input rejected: %s: %s" % (type(error).__name__, error), file=sys.stderr)
        return 2
    print(json.dumps(result, allow_nan=False, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    sys.exit(main())

"""Regression tests for the SEC annual parser. All inputs are synthetic, SEC-shaped test data."""
import copy
import json
from pathlib import Path
import subprocess
import sys
import unittest

import financials

SHA_A = "a" * 64
SHA_B = "b" * 64
FY24 = ("0000000001-24-000010", "2024-11-01", "2024-09-28", "2024-11-01T14:01:36.000Z")
FY25 = ("0000000001-25-000020", "2025-10-31", "2025-09-27", "2025-10-31T14:01:26.000Z")


def index(*filings, form="10-K"):
    recent = {"accessionNumber": [], "form": [], "filingDate": [], "reportDate": [], "acceptanceDateTime": []}
    for accn, filed, report, accepted in filings:
        for key, value in zip(recent, (accn, form, filed, report, accepted)):
            recent[key].append(value)
    return recent


def annual(accn, filed, end, start, revenue=1000, income=100, cash=150, assets=5000, liabilities=3000, equity=2000, form="10-K"):
    def flow(v):
        return {"start": start, "end": end, "val": v, "accn": accn, "fy": int(end[:4]), "fp": "FY", "form": form, "filed": filed}

    def instant(v):
        return {"end": end, "val": v, "accn": accn, "fy": int(end[:4]), "fp": "FY", "form": form, "filed": filed}
    return {"RevenueFromContractWithCustomerExcludingAssessedTax": [flow(revenue)], "NetIncomeLoss": [flow(income)],
            "NetCashProvidedByUsedInOperatingActivities": [flow(cash)], "Assets": [instant(assets)],
            "Liabilities": [instant(liabilities)], "StockholdersEquity": [instant(equity)],
            "LiabilitiesAndStockholdersEquity": [instant(liabilities + equity)]}


def merge(*parts):
    out = {}
    for part in parts:
        for tag, rows in part.items():
            out.setdefault(tag, []).extend(copy.deepcopy(rows))
    return out


def request(gaap, recent, as_of="2026-10-23T00:00:00Z", sic="3571", sub_cik="0000000001", facts_cik=1):
    # Row lists are wrapped like real companyfacts: tag -> units -> USD -> rows.
    concepts = {tag: rows if isinstance(rows, dict) else {"units": {"USD": rows}} for tag, rows in gaap.items()}
    return {"symbol": "TEST", "cik": "0000000001", "as_of": as_of, "submissions_sha256": SHA_A, "facts_sha256": SHA_B,
            "submissions": {"cik": sub_cik, "name": "Synthetic Test Issuer", "tickers": ["TEST"], "sic": sic, "filings": {"recent": recent}},
            "companyfacts": {"cik": facts_cik, "entityName": "Synthetic Test Issuer", "facts": {"us-gaap": concepts}}}


def two_years():
    fy24 = annual(FY24[0], FY24[1], "2024-09-28", "2023-10-01")
    fy25 = annual(FY25[0], FY25[1], "2025-09-27", "2024-09-29", revenue=1200, income=120, cash=130, assets=5500, liabilities=3200, equity=2300)
    # The FY2025 report repeats FY2024 as comparative columns.
    comparative = annual(FY25[0], FY25[1], "2024-09-28", "2023-10-01")
    return merge(fy24, fy25, comparative), index(FY24, FY25)


def period(report, end):
    return next(p for p in report["periods"] if p["fiscal_period_end"] == end)


def fact(p, metric):
    return next((f for f in p["facts"] if f["metric"] == metric), None)


class OriginalFilingAnchoring(unittest.TestCase):
    def test_each_period_uses_its_own_annual_report(self):
        report = financials.assess(request(*two_years()))
        self.assertEqual([p["fiscal_period_end"] for p in report["periods"]], ["2025-09-27", "2024-09-28"])
        fy24 = period(report, "2024-09-28")
        self.assertEqual(fy24["filing"]["accession"], FY24[0])
        self.assertTrue(all(f["accession"] == FY24[0] for f in fy24["facts"]))
        self.assertEqual(fy24["missing_metrics"], [])
        self.assertEqual({c["name"]: c["status"] for c in fy24["checks"]}, {"BALANCE_TOTALS": "PASS", "LIABILITIES_PLUS_EQUITY": "PASS"})
        self.assertEqual(fy24["derived"]["net_margin"], {"value": "0.100000", "inputs": ["net_income", "revenue"], "status": "DERIVED"})
        self.assertEqual(fy24["start"], "2023-10-01")
        self.assertEqual(fy24["filing"]["report_date_basis"], "SEC_REPORT_DATE")

    def test_later_restatement_is_shown_as_counter_evidence(self):
        gaap, recent = two_years()
        for row in gaap["RevenueFromContractWithCustomerExcludingAssessedTax"]:
            if row["accn"] == FY25[0] and row["end"] == "2024-09-28":
                row["val"] = 1010
        fy24 = period(financials.assess(request(gaap, recent)), "2024-09-28")
        self.assertEqual(fact(fy24, "revenue")["value"], "1000")
        self.assertEqual(fy24["later_filing_differences"], [{"metric": "revenue", "tag": "RevenueFromContractWithCustomerExcludingAssessedTax",
                                                             "original_value": "1000", "later_value": "1010", "later_accession": FY25[0],
                                                             "later_form": "10-K", "later_filed": "2025-10-31"}])
        self.assertIn("LATER_FILINGS_REPORT_DIFFERENT_VALUES", financials.assess(request(gaap, recent))["warnings"])

    def test_original_report_preferred_over_amendment(self):
        gaap, recent = two_years()
        amendment = ("0000000001-25-000030", "2025-12-01", "2025-09-27", "2025-12-01T15:00:00.000Z")
        gaap = merge(gaap, annual(amendment[0], amendment[1], "2025-09-27", "2024-09-29", revenue=1250, form="10-K/A"))
        for key, value in zip(recent, (amendment[0], "10-K/A", amendment[1], amendment[2], amendment[3])):
            recent[key].append(value)
        fy25 = period(financials.assess(request(gaap, recent)), "2025-09-27")
        self.assertEqual(fy25["filing"]["accession"], FY25[0])
        self.assertEqual(fact(fy25, "revenue")["value"], "1200")
        self.assertIn("0000000001-25-000030", [d["later_accession"] for d in fy25["later_filing_differences"]])

    def test_period_count_is_capped_and_short_series_flagged(self):
        report = financials.assess(request(*two_years()))
        self.assertIn("LESS_THAN_FIVE_ANNUAL_PERIODS", report["warnings"])
        filings, parts = [], []
        for year in range(2015, 2023):
            accn = "0000000001-%02d-000001" % (year % 100 + 1)
            filed, end = "%d-11-01" % year, "%d-09-30" % year
            filings.append((accn, filed, end, filed + "T14:00:00.000Z"))
            parts.append(annual(accn, filed, end, "%d-10-01" % (year - 1)))
        many = financials.assess(request(merge(*parts), index(*filings)))
        self.assertEqual(len(many["periods"]), 5)
        self.assertNotIn("LESS_THAN_FIVE_ANNUAL_PERIODS", many["warnings"])


class ConflictsAndTiming(unittest.TestCase):
    def conflicting(self, reverse):
        # Reviewer reproduction: one context, two different revenue values.
        gaap = annual(FY24[0], FY24[1], "2024-09-28", "2023-10-01", revenue=1000)
        gaap = {"Revenues": [dict(gaap["RevenueFromContractWithCustomerExcludingAssessedTax"][0])], **{k: v for k, v in gaap.items() if k != "RevenueFromContractWithCustomerExcludingAssessedTax"}}
        second = dict(gaap["Revenues"][0], val=10000)
        gaap["Revenues"] = [second, gaap["Revenues"][0]] if reverse else [gaap["Revenues"][0], second]
        return financials.assess(request(gaap, index(FY24)))

    def test_conflicting_values_are_withheld_in_any_row_order(self):
        reports = [self.conflicting(False), self.conflicting(True)]
        for report in reports:
            p = report["periods"][0]
            self.assertIsNone(fact(p, "revenue"))
            self.assertEqual(p["withheld_metrics"], [{"metric": "revenue", "tag": "Revenues", "reason": "CONFLICTING_VALUES", "values": ["1000", "10000"]}])
            self.assertEqual(p["derived"]["net_margin"]["status"], "MISSING_INPUT")
            self.assertIsNone(p["derived"]["net_margin"]["value"])
            self.assertIn("CONFLICTING_FACT_VALUES_WITHHELD", report["warnings"])
        self.assertEqual(json.dumps(reports[0], sort_keys=True), json.dumps(reports[1], sort_keys=True))

    def test_equal_duplicates_are_merged(self):
        gaap = annual(FY24[0], FY24[1], "2024-09-28", "2023-10-01")
        gaap["NetIncomeLoss"].append(dict(gaap["NetIncomeLoss"][0]))
        report = financials.assess(request(gaap, index(FY24)))
        self.assertEqual(fact(report["periods"][0], "net_income")["value"], "100")
        self.assertNotIn("CONFLICTING_FACT_VALUES_WITHHELD", report["warnings"])

    def test_period_ending_after_its_filing_is_rejected(self):
        # Reviewer reproduction: an annual period that ends after it was filed.
        accn = ("0000000001-24-000099", "2024-10-01", "2024-12-31", "2024-10-01T14:00:00.000Z")
        gaap = annual(accn[0], accn[1], "2024-12-31", "2024-01-01")
        report = financials.assess(request(gaap, index(accn), as_of="2025-03-01T00:00:00Z"))
        self.assertEqual(report["periods"], [])
        self.assertEqual(report["status"], "INSUFFICIENT_DATA")
        self.assertEqual(report["excluded_facts"]["PERIOD_ENDS_AFTER_FILING"], 7)
        self.assertIn("TEMPORALLY_INCONSISTENT_FACTS_EXCLUDED", report["warnings"])

    def test_filings_after_as_of_are_invisible(self):
        report = financials.assess(request(*two_years(), as_of="2025-10-31T14:00:00Z"))
        self.assertEqual([p["fiscal_period_end"] for p in report["periods"]], ["2024-09-28"])
        self.assertGreater(report["excluded_facts"]["NOT_AVAILABLE_AT_AS_OF"], 0)
        later = financials.assess(request(*two_years(), as_of="2025-10-31T14:01:26Z"))
        self.assertEqual(len(later["periods"]), 2)

    def test_availability_uses_acceptance_or_flags_an_estimate(self):
        gaap, recent = two_years()
        accepted = financials.assess(request(gaap, recent))
        fy25 = period(accepted, "2025-09-27")
        self.assertEqual(fy25["filing"]["availability_basis"], "SEC_ACCEPTANCE")
        self.assertEqual(fy25["filing"]["available_at"], "2025-10-31T14:01:26+00:00")
        estimated = financials.assess(request(gaap, index(FY24)))
        fy25 = period(estimated, "2025-09-27")
        self.assertEqual(fy25["filing"]["availability_basis"], "FILING_DATE_PLUS_ONE_DAY_ESTIMATE")
        self.assertEqual(fy25["filing"]["available_at"], "2025-11-01T00:00:00+00:00")
        self.assertEqual(fy25["filing"]["report_date_basis"], "LATEST_FACT_END_ESTIMATE")
        self.assertIn("FILING_TIME_ESTIMATED", estimated["warnings"])

    def test_acceptance_before_filing_date_never_makes_data_earlier(self):
        early = ("0000000001-24-000010", "2024-11-01", "2024-09-28", "2024-10-31T22:30:00.000Z")
        fy = financials.assess(request(annual(early[0], early[1], "2024-09-28", "2023-10-01"), index(early)))["periods"][0]
        self.assertEqual(fy["filing"]["available_at"], "2024-11-01T00:00:00+00:00")

    def test_index_metadata_must_agree_with_facts(self):
        gaap, recent = two_years()
        recent["filingDate"][0] = "2024-11-02"
        report = financials.assess(request(gaap, recent))
        self.assertEqual([p["fiscal_period_end"] for p in report["periods"]], ["2025-09-27"])
        self.assertIn("MALFORMED_FACTS_EXCLUDED", report["warnings"])


class UnitsValuesAndContext(unittest.TestCase):
    def test_non_annual_durations_other_units_and_forms_are_ignored(self):
        gaap = annual(FY24[0], FY24[1], "2024-09-28", "2023-10-01")
        quarter = dict(gaap["NetIncomeLoss"][0], start="2024-06-30", val=1)
        quarterly_form = dict(gaap["NetIncomeLoss"][0], form="10-Q", val=2)
        gaap["NetIncomeLoss"] += [quarter, quarterly_form]
        gaap["Assets"] = {"units": {"EUR": gaap["Assets"]}}
        p = financials.assess(request(gaap, index(FY24)))["periods"][0]
        self.assertEqual(fact(p, "net_income")["value"], "100")
        self.assertIsNone(fact(p, "assets"))
        self.assertEqual(p["missing_metrics"], ["assets"])
        self.assertEqual(p["derived"]["liabilities_to_assets"]["status"], "MISSING_INPUT")

    def test_non_integer_boolean_and_huge_values_are_refused(self):
        gaap = annual(FY24[0], FY24[1], "2024-09-28", "2023-10-01")
        gaap["NetIncomeLoss"][0]["val"] = 100.5
        gaap["Assets"][0]["val"] = True
        gaap["Liabilities"][0]["val"] = 10**19
        report = financials.assess(request(gaap, index(FY24)))
        p = report["periods"][0]
        self.assertEqual(report["excluded_facts"]["NON_INTEGER_OR_OUT_OF_RANGE_VALUE"], 3)
        self.assertEqual(p["missing_metrics"], ["assets", "liabilities", "net_income"])
        self.assertEqual(p["derived"]["net_margin"]["status"], "MISSING_INPUT")
        self.assertIn("INCOMPLETE_ANNUAL_CONTEXT", report["warnings"])

    def test_nonpositive_denominator_is_undefined_not_zero(self):
        gaap = annual(FY24[0], FY24[1], "2024-09-28", "2023-10-01", income=-50)
        p = financials.assess(request(gaap, index(FY24)))["periods"][0]
        self.assertEqual(p["derived"]["cash_conversion"], {"value": None, "inputs": ["operating_cash_flow", "net_income"], "status": "UNDEFINED_NONPOSITIVE_DENOMINATOR"})
        self.assertEqual(p["derived"]["net_margin"]["value"], "-0.050000")

    def test_ratio_is_exact_decimal(self):
        self.assertEqual(financials.ratio(1, 3), "0.333333")
        self.assertEqual(financials.ratio(2, 3), "0.666667")
        self.assertEqual(financials.ratio(391035000000, 394328000000), "0.991649")

    def test_balance_checks_show_differences(self):
        gaap = annual(FY24[0], FY24[1], "2024-09-28", "2023-10-01")
        gaap["LiabilitiesAndStockholdersEquity"][0]["val"] = 5100
        report = financials.assess(request(gaap, index(FY24)))
        checks = {c["name"]: c for c in report["periods"][0]["checks"]}
        self.assertEqual(checks["BALANCE_TOTALS"], {"name": "BALANCE_TOTALS", "status": "FAIL", "difference_usd": "-100"})
        self.assertEqual(checks["LIABILITIES_PLUS_EQUITY"]["status"], "UNEXPLAINED_DIFFERENCE")
        self.assertIn("BALANCE_TOTALS_MISMATCH", report["warnings"])
        del gaap["LiabilitiesAndStockholdersEquity"]
        checks = {c["name"]: c["status"] for c in financials.assess(request(gaap, index(FY24)))["periods"][0]["checks"]}
        self.assertEqual(checks, {"BALANCE_TOTALS": "MISSING_CONTEXT", "LIABILITIES_PLUS_EQUITY": "MISSING_CONTEXT"})

    def test_financial_issuer_ratios_are_withheld(self):
        report = financials.assess(request(*two_years(), sic="6021"))
        self.assertIn("FINANCIAL_ISSUER_GENERIC_RATIOS_WITHHELD", report["warnings"])
        self.assertTrue(all(p["derived"] == {} for p in report["periods"]))

    def test_mismatched_flow_years_are_withheld(self):
        gaap = annual(FY24[0], FY24[1], "2024-09-28", "2023-10-01")
        gaap["NetIncomeLoss"][0]["start"] = "2023-09-25"
        p = financials.assess(request(gaap, index(FY24)))["periods"][0]
        self.assertEqual({w["reason"] for w in p["withheld_metrics"]}, {"MISMATCHED_FISCAL_YEAR"})
        self.assertIsNone(fact(p, "revenue"))
        self.assertIsNotNone(fact(p, "assets"))

    def test_fact_records_point_to_their_source(self):
        f = fact(financials.assess(request(*two_years()))["periods"][0], "revenue")
        self.assertEqual(f["pointer"], "/facts/us-gaap/RevenueFromContractWithCustomerExcludingAssessedTax/units/USD/1")
        self.assertEqual(f["source_sha256"], SHA_B)
        self.assertEqual(f["filing_url"], "https://www.sec.gov/Archives/edgar/data/1/000000000125000020/0000000001-25-000020-index.html")
        self.assertEqual((f["unit"], f["start"], f["end"], f["filed"]), ("USD", "2024-09-29", "2025-09-27", "2025-10-31"))


class Instants(unittest.TestCase):
    def test_any_fraction_length_parses_on_python_39(self):
        # Go's RFC3339Nano trims trailing zeros, giving 1 to 9 fractional digits.
        for text, micro in [("2026-10-23T06:11:24.7Z", 700000), ("2026-10-23T06:11:24.74672Z", 746720),
                            ("2026-10-23T06:11:24.000Z", 0), ("2026-10-23T06:11:24.123456789Z", 123456),
                            ("2026-10-23T06:11:24Z", 0), ("2026-10-23T09:11:24.5+03:00", 500000)]:
            with self.subTest(text=text):
                t = financials.parse_instant(text)
                self.assertEqual((t.hour, t.microsecond, t.utcoffset().total_seconds()), (6, micro, 0))

    def test_naive_or_malformed_instants_are_refused(self):
        for text in ["2026-10-23T06:11:24", "2026-10-23", "not a time", 5]:
            with self.subTest(text=text), self.assertRaises(ValueError):
                financials.parse_instant(text)


class Boundaries(unittest.TestCase):
    def test_identity_accepts_both_sec_cik_encodings_and_rejects_mismatch(self):
        gaap, recent = two_years()
        self.assertEqual(financials.assess(request(gaap, recent, sub_cik="0000000001", facts_cik=1))["cik"], "0000000001")
        self.assertEqual(financials.assess(request(gaap, recent, sub_cik=1, facts_cik="1"))["cik"], "0000000001")
        for sub_cik, facts_cik in [("0000000002", 1), ("0000000001", 2), (True, 1), ("x", 1), (None, 1)]:
            with self.subTest(sub_cik=sub_cik, facts_cik=facts_cik), self.assertRaises(ValueError):
                financials.assess(request(gaap, recent, sub_cik=sub_cik, facts_cik=facts_cik))

    def test_malformed_requests_are_refused(self):
        good = request(*two_years())
        for change in [{"symbol": "test"}, {"cik": "1"}, {"as_of": "2026-10-23T00:00:00"}, {"facts_sha256": "A" * 64},
                       {"submissions": []}, {"companyfacts": None}]:
            with self.subTest(change=change), self.assertRaises((ValueError, TypeError)):
                financials.assess({**good, **change})
        no_ticker = copy.deepcopy(good)
        no_ticker["submissions"]["tickers"] = ["OTHER"]
        with self.assertRaises(ValueError):
            financials.assess(no_ticker)

    def test_reports_never_authorize_entry_backtest_or_state_writes(self):
        for data in [request(*two_years()), request({}, index()), request(*two_years(), sic="6021")]:
            report = financials.assess(data)
            self.assertEqual(report["outcome"], "INSUFFICIENT_DATA")
            self.assertIs(report["eligible_for_entry"], False)
            self.assertIs(report["eligible_for_backtest"], False)
            self.assertIs(report["financial_state_write_access"], False)
            self.assertEqual(report["version"], "sec-annual-v2")
            json.dumps(report, allow_nan=False)
        self.assertEqual(financials.assess(request({}, index()))["warnings"].count("NO_ANNUAL_REPORT_CONTEXT"), 1)

    def test_process_boundary_reports_or_rejects_without_partial_output(self):
        script = str(Path(financials.__file__))
        ok = subprocess.run([sys.executable, "-I", script], input=json.dumps(request(*two_years())).encode(), capture_output=True, timeout=30)
        self.assertEqual(ok.returncode, 0, ok.stderr)
        self.assertEqual(json.loads(ok.stdout)["version"], "sec-annual-v2")
        for bad in [b"{", b"[]", json.dumps({**request(*two_years()), "cik": "0000000002"}).encode()]:
            rejected = subprocess.run([sys.executable, "-I", script], input=bad, capture_output=True, timeout=30)
            self.assertEqual((rejected.returncode, rejected.stdout), (2, b""), bad[:40])
            self.assertIn(b"research input rejected", rejected.stderr)


if __name__ == "__main__":
    unittest.main()

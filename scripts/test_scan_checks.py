"""Tests for scripts/scan_checks.py. Offline: temporary files and a fake OSV
query only. Run with the pinned Python (it needs tomllib):
    .tools/research-venv/bin/python -I -B scripts/test_scan_checks.py
"""
import contextlib
import datetime
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
import scan_checks  # noqa: E402

TODAY = datetime.date(2026, 10, 23)
GOOD = {"id": "W-1", "scanner": "gitleaks", "match": "c:f:rule:1", "reason": "r", "owner": "a", "reviewed_by": "b",
        "reviewed_on": "2026-10-23", "expires": "2026-11-22"}


def waivers_file(*waivers):
    f = tempfile.NamedTemporaryFile("w", suffix=".json", delete=False)
    json.dump({"waivers": list(waivers)}, f)
    f.close()
    return f.name


def quiet(fn, *args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = fn(*args)
    return code, out.getvalue()


class WaiverTests(unittest.TestCase):
    def test_a_complete_reviewed_waiver_is_valid(self):
        valid, problems = scan_checks.load_waivers(waivers_file(GOOD), TODAY)
        self.assertEqual((valid["gitleaks"], problems), ({"c:f:rule:1": "W-1"}, []))

    def test_incomplete_self_reviewed_expired_or_odd_waivers_are_invalid(self):
        cases = {
            "unreviewed": {**GOOD, "reviewed_by": None},
            "no owner": {**GOOD, "owner": ""},
            "no expiry": {**GOOD, "expires": None},
            "self review": {**GOOD, "reviewed_by": "a"},
            "expired today": {**GOOD, "expires": "2026-10-23"},
            "expired": {**GOOD, "expires": "2026-01-01"},
            "review in the future": {**GOOD, "reviewed_on": "2026-12-01", "expires": "2027-01-01"},
            "bad date": {**GOOD, "expires": "next month"},
            "unknown scanner": {**GOOD, "scanner": "everything"},
        }
        for name, waiver in cases.items():
            with self.subTest(case=name):
                valid, problems = scan_checks.load_waivers(waivers_file(waiver), TODAY)
                self.assertEqual(valid["gitleaks"], {})
                self.assertEqual(len(problems), 1, problems)
        _, problems = scan_checks.load_waivers(waivers_file(GOOD, GOOD), TODAY)
        self.assertIn("W-1: duplicate id", problems)

    def test_the_repository_waivers_are_valid_today(self):
        valid, problems = scan_checks.load_waivers()
        self.assertEqual(problems, [])
        listed = json.loads(scan_checks.WAIVERS.read_text())["waivers"]
        self.assertEqual(sum(len(v) for v in valid.values()), len(listed), "every listed waiver must be valid")


class GateTests(unittest.TestCase):
    def setUp(self):
        self.waivers, _ = scan_checks.load_waivers(waivers_file(GOOD), TODAY)

    def test_waived_finding_passes_open_finding_fails(self):
        self.assertEqual(quiet(scan_checks.gate, "gitleaks", [("c:f:rule:1", "x")], self.waivers)[0], 0)
        code, out = quiet(scan_checks.gate, "gitleaks", [("c:f:rule:1", "x"), ("c:g:rule:2", "y")], self.waivers)
        self.assertEqual(code, 1)
        self.assertIn("FINDING: y", out)

    def test_waiver_matching_nothing_fails(self):
        code, out = quiet(scan_checks.gate, "gitleaks", [], self.waivers)
        self.assertEqual(code, 1)
        self.assertIn("UNUSED WAIVER: W-1", out)

    def test_a_waiver_is_exact_not_a_prefix(self):
        code, _ = quiet(scan_checks.gate, "gitleaks", [("c:f:rule:1", "x"), ("c:f:rule:10", "y")], self.waivers)
        self.assertEqual(code, 1)


class ScannerParsingTests(unittest.TestCase):
    def write(self, text):
        f = tempfile.NamedTemporaryFile("w", delete=False)
        f.write(text)
        f.close()
        return f.name

    def test_gitleaks_report_must_be_redacted(self):
        row = {"RuleID": "r", "File": "f", "StartLine": 1, "Commit": "c" * 40, "Fingerprint": "fp", "Secret": "REDACTED"}
        self.assertEqual(scan_checks.gitleaks_findings(self.write(json.dumps([row])))[0][0], "fp")
        with self.assertRaises(SystemExit):
            # A plain, low-entropy placeholder: long enough to count as an unredacted value,
            # but shaped like no real credential, so the secret scan does not flag this file.
            scan_checks.gitleaks_findings(self.write(json.dumps([{**row, "Secret": "x" * 20}])))

    def test_govulncheck_counts_only_reached_code(self):
        stream = "\n".join(json.dumps(o) for o in [
            {"config": {}},
            {"finding": {"osv": "GO-1", "trace": [{"module": "m", "package": "m/p", "function": "F"}], "fixed_version": "v2"}},
            {"finding": {"osv": "GO-2", "trace": [{"module": "m"}]}},
            {"finding": {"osv": "GO-3", "trace": [{"module": "m", "package": "m/p"}]}},
        ])
        found = scan_checks.govulncheck_findings(self.write(stream))
        self.assertEqual([k for k, _ in found], ["GO-1"])

    def test_osv_reads_the_lock_and_requirements(self):
        lock = self.write('version = 1\n[[package]]\nname = "ruff"\nversion = "0.16.10"\nsource = { registry = "https://pypi.org/simple" }\n'
                          '[[package]]\nname = "tradestocker-research"\nversion = "0.1.0"\nsource = { editable = "." }\n')
        reqs = self.write("uv==0.13.0 \\\n    --hash=sha256:aa\n")
        packages = scan_checks.locked_packages(lock, reqs)
        self.assertEqual(packages, [("ruff", "0.16.10"), ("uv", "0.13.0")])
        def fake(queries):
            return [{"vulns": [{"id": "PYSEC-1"}]} if q["package"]["name"] == "uv" else {} for q in queries]
        self.assertEqual([k for k, _ in scan_checks.osv_findings(packages, fake)], ["uv==0.13.0:PYSEC-1"])


if __name__ == "__main__":
    unittest.main()

"""Checks that the synthetic fixtures are deterministic, labelled and safe.

Run with the pinned research environment (make test does this):
    .tools/research-venv/bin/python -I -B fixtures/synthetic/test_fixtures.py
"""
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("generate", HERE / "generate.py")
generate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(generate)
FIXTURES = ["paper-accounts.json", "research/requests.jsonl", "research/expected-results.jsonl"]


class SyntheticFixtureTests(unittest.TestCase):
    def test_generator_reproduces_the_inputs_byte_for_byte(self):
        with tempfile.TemporaryDirectory() as out:
            generate.main(out)
            for rel in generate.files():
                with self.subTest(file=rel):
                    self.assertEqual((Path(out) / rel).read_bytes(), (HERE / rel).read_bytes())

    def pipeline(self):
        """research-worker on the fixture requests, then the fixture labeller."""
        with open(HERE / "research" / "requests.jsonl", "rb") as requests:
            worker = subprocess.run([sys.executable, "-I", "-m", "tradestocker_research.worker"], stdin=requests, capture_output=True, timeout=60)
        self.assertEqual(worker.returncode, 0, worker.stderr)
        return self.label(worker.stdout)

    def label(self, results):
        return subprocess.run([sys.executable, "-I", str(HERE / "label.py"), str(HERE / "research" / "requests.jsonl")], input=results, capture_output=True, timeout=60)

    def test_research_worker_and_labeller_give_exactly_the_expected_results(self):
        out = self.pipeline()
        self.assertEqual(out.returncode, 0, out.stderr)
        self.assertEqual(out.stdout, (HERE / "research" / "expected-results.jsonl").read_bytes())

    def test_stored_results_claim_no_official_source_link_or_permission(self):
        text = (HERE / "research" / "expected-results.jsonl").read_text()
        for word in ["sec.gov", "SEC_PUBLIC", "OFFICIAL_FILINGS", "http://", "https://"]:
            self.assertNotIn(word, text)
        digest = hashlib.sha256((HERE / "research" / "requests.jsonl").read_bytes()).hexdigest()
        results = [json.loads(line) for line in text.splitlines()]
        for n, result in enumerate(results, 1):
            with self.subTest(line=n):
                self.assertEqual(result["provenance"]["data"], "SYNTHETIC")
                self.assertEqual((result["provenance"]["fixture_sha256"], result["provenance"]["line"]), (digest, n))
        report = results[0]
        self.assertEqual((report["source"], report["data_kind"]), ("SYNTHETIC_FIXTURE", "SYNTHETIC"))
        urls = [f["filing_url"] for p in report["periods"] for f in [p["filing"]] + p["facts"]]
        self.assertTrue(urls)
        self.assertEqual(set(urls), {None})

    def test_labeller_fails_closed(self):
        good = [json.loads(line) for line in (HERE / "research" / "expected-results.jsonl").read_text().splitlines()]
        raw = [{k: v for k, v in r.items() if k != "provenance"} for r in good]
        cases = {
            "fewer results than requests": raw[:2],
            "SEC link in another field": [{**raw[0], "note": "see https://www.sec.gov/x"}] + raw[1:],
            "entry permission": [{**raw[0], "eligible_for_entry": True}] + raw[1:],
            "nested write permission": [raw[0], {**raw[1], "inner": {"financial_state_write_access": True}}, raw[2]],
        }
        for name, results in cases.items():
            with self.subTest(case=name):
                out = self.label("".join(json.dumps(r) + "\n" for r in results).encode())
                self.assertEqual((out.returncode, out.stdout), (1, b""), out.stderr)
                self.assertIn(b"nothing written", out.stderr)

    def test_manifest_matches_every_fixture(self):
        manifest = json.loads((HERE / "manifest.json").read_text())
        self.assertTrue(manifest["synthetic"])
        self.assertEqual(sorted(manifest["files"]), sorted(FIXTURES))
        for rel, digest in manifest["files"].items():
            with self.subTest(file=rel):
                self.assertEqual(hashlib.sha256((HERE / rel).read_bytes()).hexdigest(), digest)

    def test_inputs_are_labelled_synthetic(self):
        accounts = json.loads((HERE / "paper-accounts.json").read_text())
        self.assertIn("SYNTHETIC", accounts["notice"])
        sec = json.loads((HERE / "research" / "requests.jsonl").read_text().splitlines()[0])["input"]
        for doc in (sec["submissions"], sec["companyfacts"]):
            self.assertIn("SYNTHETIC", doc["notice"])
        self.assertIn("(invented)", sec["submissions"]["name"])

    def test_results_never_grant_entry_backtest_or_write_access(self):
        results = [json.loads(line) for line in (HERE / "research" / "expected-results.jsonl").read_text().splitlines()]
        self.assertEqual([r.get("version", r.get("status")) for r in results], ["sec-annual-v2", "research-stub-v1", "REJECTED"])
        for result in results[:2]:
            self.assertEqual(result["outcome"], "INSUFFICIENT_DATA")
            self.assertIs(result["financial_state_write_access"], False)
        self.assertIs(results[0]["eligible_for_entry"], False)
        self.assertIs(results[0]["eligible_for_backtest"], False)


if __name__ == "__main__":
    unittest.main()

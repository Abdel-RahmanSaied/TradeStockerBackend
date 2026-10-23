import io
import json
from pathlib import Path
import subprocess
import sys
import unittest

from test_financials import request, two_years
from tradestocker_research import financials, worker
from tradestocker_research.worker import assess


class ResearchBoundaryTests(unittest.TestCase):
    def test_stub_never_claims_research_or_returns(self):
        result = assess({"symbol": "AAPL", "mode": "PAPER"})
        self.assertEqual(result["outcome"], "INSUFFICIENT_DATA")
        self.assertEqual(result["evidence"], [])
        self.assertFalse(result["financial_state_write_access"])
        self.assertNotIn("returns", result)

    def test_actual_live_and_untrusted_fields_rejected(self):
        for request in [
            {"symbol": "AAPL", "mode": "ACTUAL"},
            {"symbol": "AAPL", "mode": "LIVE_ADVISORY"},
            {"symbol": "AAPL", "mode": "PAPER", "approved": True},
            {"symbol": "AAPL", "mode": "PAPER", "cash": 100},
            {"symbol": "", "mode": "PAPER"},
            {"symbol": "A" * 33, "mode": "PAPER"},
        ]:
            with self.subTest(request=request), self.assertRaises(ValueError):
                assess(request)


class WorkerEntrypointTests(unittest.TestCase):
    def run_worker(self, data):
        return subprocess.run([sys.executable, "-I", "-m", "tradestocker_research.worker"], input=data, capture_output=True, timeout=30)

    def test_report_is_byte_identical_to_the_parser_process(self):
        sec = request(*two_years())
        alone = subprocess.run([sys.executable, "-I", str(Path(financials.__file__))], input=json.dumps(sec).encode(), capture_output=True, timeout=30)
        self.assertEqual(alone.returncode, 0, alone.stderr)
        line = json.dumps({"task": "sec-annual-v2", "input": sec}).encode() + b"\n"
        out = self.run_worker(line + line)
        self.assertEqual(out.returncode, 0, out.stderr)
        self.assertEqual(out.stdout, alone.stdout + alone.stdout)

    def test_each_bad_request_is_rejected_and_the_stream_continues(self):
        good = json.dumps({"task": "research-stub-v1", "input": {"symbol": "AAPL", "mode": "PAPER"}})
        bad = [
            "{",
            "[]",
            json.dumps({"task": "sec-annual-v2"}),
            json.dumps({"task": "sec-annual-v2", "input": {}, "extra": 1}),
            json.dumps({"task": "place-order", "input": {}}),
            json.dumps({"task": ["sec-annual-v2"], "input": {}}),
            json.dumps({"task": "sec-annual-v2", "input": {**request(*two_years()), "cik": "0000000002"}}),
            json.dumps({"task": "research-stub-v1", "input": {"symbol": "AAPL", "mode": "LIVE_ADVISORY"}}),
            "[" * 100000 + "]" * 100000,
        ]
        out = self.run_worker(("\n".join(bad + ["", good]) + "\n").encode())
        self.assertEqual(out.returncode, 0, out.stderr)
        results = [json.loads(x) for x in out.stdout.splitlines()]
        self.assertEqual(len(results), len(bad) + 1)
        for result in results[:-1]:
            self.assertEqual(result["status"], "REJECTED", result)
        self.assertEqual(results[-1]["version"], "research-stub-v1")

    def test_oversized_line_is_dropped_whole_without_losing_sync(self):
        stream = io.BytesIO(b"x" * 25 + b"\n" + b"ok\n" + b"y" * 10 + b"\n")
        self.assertEqual(list(worker.lines(stream, 10)), [None, b"ok\n", None])
        self.assertEqual(list(worker.lines(io.BytesIO(b"x" * 9 + b"\n"), 10)), [b"x" * 9 + b"\n"])

    def test_arguments_are_refused(self):
        self.assertEqual(worker.main(["--live"]), 2)


if __name__ == "__main__":
    unittest.main()

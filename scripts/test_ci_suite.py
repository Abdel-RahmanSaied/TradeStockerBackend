"""Tests for scripts/ci_suite.py. Offline; temporary folders only.
    python3 -B scripts/test_ci_suite.py
"""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().with_name("ci_suite.py")
sys.path.insert(0, str(SCRIPT.parent))
import ci_suite  # noqa: E402


def go_event(action, test=None, output=None, package="m/p"):
    event = {"Action": action, "Package": package}
    if test:
        event["Test"] = test
    if output is not None:
        event["Output"] = output
    return json.dumps(event)


class CountTests(unittest.TestCase):
    def test_unittest_summaries(self):
        self.assertEqual(ci_suite.unittest_counts("...\nRan 5 tests in 0.1s\n\nOK\n")["ran"], 5)
        c = ci_suite.unittest_counts("Ran 7 tests in 1s\n\nOK (skipped=2)\n")
        self.assertEqual((c["ran"], c["skipped"], c["failures"]), (7, 2, 0))
        c = ci_suite.unittest_counts("Ran 9 tests in 1s\n\nFAILED (failures=1, errors=2, skipped=3)\n")
        self.assertEqual((c["failures"], c["errors"], c["skipped"]), (1, 2, 3))
        self.assertIsNone(ci_suite.unittest_counts("no summary here"))

    def test_go_top_level_events_and_skip_reasons(self):
        text = "\n".join([
            go_event("run", "TestA"), go_event("pass", "TestA"),
            go_event("run", "TestB"), go_event("output", "TestB", "=== RUN   TestB\n"),
            go_event("output", "TestB", "    x_test.go:9: TEST_DATABASE_URL absent\n"), go_event("output", "TestB", "--- SKIP: TestB\n"),
            go_event("skip", "TestB"),
            go_event("fail", "TestC"), go_event("pass", "TestA/sub"), go_event("pass"),
        ])
        c = ci_suite.go_counts(text)
        self.assertEqual((c["ran"], c["passed"], c["skipped"], c["failures"]), (3, 1, 1, 1))
        self.assertEqual(c["skip_reasons"], {"m/p.TestB": "x_test.go:9: TEST_DATABASE_URL absent"})
        self.assertIsNone(ci_suite.go_counts("not json"))


class RunAndReportTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.out = Path(temp.name) / "results"

    def run_suite(self, name, code):
        script = "import sys; print('Ran 2 tests in 0.0s'); print(); print('OK' if %d == 0 else 'FAILED (failures=1)'); sys.exit(%d)" % (code, code)
        return subprocess.run([sys.executable, str(SCRIPT), "run", "--name", name, "--out", str(self.out), "--", sys.executable, "-c", script],
                              capture_output=True, text=True, timeout=60)

    def report(self):
        return subprocess.run([sys.executable, str(SCRIPT), "report", str(self.out)], capture_output=True, text=True, timeout=60, env={"PATH": "/usr/bin:/bin"})

    def test_passing_suite_is_recorded_and_reported(self):
        self.assertEqual(self.run_suite("good", 0).returncode, 0)
        record = json.loads((self.out / "good.json").read_text())
        self.assertEqual((record["exit"], record["counts"]["ran"]), (0, 2))
        self.assertTrue((self.out / "good.log").exists())
        result = self.report()
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertIn("| good | 0 | 2 | 0 | 0 | 0 |", result.stdout)
        self.assertIn("Not run by CI", result.stdout)
        self.assertIn("Real SEC research", result.stdout)

    def test_failing_suite_fails_the_run_and_the_report(self):
        self.run_suite("good", 0)
        self.assertEqual(self.run_suite("bad", 1).returncode, 1)
        self.assertEqual(self.report().returncode, 1)

    def test_suite_without_counts_fails(self):
        result = subprocess.run([sys.executable, str(SCRIPT), "run", "--name", "silent", "--out", str(self.out), "--", sys.executable, "-c", "pass"],
                                capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(self.report().returncode, 1)

    def test_empty_results_fail_the_report(self):
        self.out.mkdir(parents=True)
        self.assertEqual(self.report().returncode, 1)


if __name__ == "__main__":
    unittest.main()

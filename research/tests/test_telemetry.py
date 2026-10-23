"""Trace continuation and safe log events (S01-T05). All values are synthetic."""
import io
import json
import subprocess
import sys
import unittest

from tradestocker_research import telemetry

PARENT = "00-" + "ab" * 16 + "-" + "cd" * 8 + "-01"


class TraceTests(unittest.TestCase):
    def test_parse_accepts_only_well_formed_version_00(self):
        self.assertEqual(telemetry.parse(PARENT), ("ab" * 16, "cd" * 8))
        for bad in [None, 7, "", PARENT.upper(), "01" + PARENT[2:], PARENT + "x", " " + PARENT,
                    "00-" + "0" * 32 + "-" + "cd" * 8 + "-01", "00-" + "ab" * 16 + "-" + "0" * 16 + "-01"]:
            with self.subTest(header=bad):
                self.assertIsNone(telemetry.parse(bad))

    def test_continue_keeps_the_trace_with_a_new_span(self):
        trace_id, span_id = telemetry.continue_trace(PARENT)
        self.assertEqual(trace_id, "ab" * 16)
        self.assertNotEqual(span_id, "cd" * 8)
        self.assertIsNotNone(telemetry.parse("00-%s-%s-01" % (trace_id, span_id)))

    def test_malformed_header_starts_a_new_trace(self):
        trace_id, _ = telemetry.continue_trace("00-not-a-trace")
        self.assertNotEqual(trace_id, "ab" * 16)
        self.assertRegex(trace_id, r"^[0-9a-f]{32}$")


class EventTests(unittest.TestCase):
    def test_event_keeps_names_and_numbers_and_omits_text(self):
        fragment = "fixture filing text: revenue 1200"
        out = io.StringIO()
        telemetry.event("research task ended", ("ab" * 16, "cd" * 8), stream=out,
                        task="sec-annual-v2", status="ok", duration_ms=3, error_type=fragment)
        line = json.loads(out.getvalue())
        self.assertEqual(line["msg"], "research task ended")
        self.assertEqual((line["trace_id"], line["span_id"]), ("ab" * 16, "cd" * 8))
        self.assertEqual((line["task"], line["status"], line["duration_ms"]), ("sec-annual-v2", "ok", 3))
        self.assertEqual(line["error_type"], "[omitted]")
        self.assertNotIn(fragment, out.getvalue())

    def test_unknown_fields_are_refused(self):
        with self.assertRaises(ValueError):
            telemetry.event("x", ("ab" * 16, "cd" * 8), stream=io.StringIO(), input="anything")


class WorkerLogTests(unittest.TestCase):
    def run_worker(self, *requests):
        data = "".join(json.dumps(r) + "\n" for r in requests).encode()
        out = subprocess.run([sys.executable, "-I", "-m", "tradestocker_research.worker"], input=data, capture_output=True, timeout=30)
        self.assertEqual(out.returncode, 0, out.stderr)
        return out, [json.loads(x) for x in out.stderr.decode().splitlines()]

    def test_each_request_logs_one_line_in_its_trace(self):
        stub = {"symbol": "AAPL", "mode": "PAPER"}
        _, logs = self.run_worker({"task": "research-stub-v1", "input": stub, "traceparent": PARENT},
                                  {"task": "research-stub-v1", "input": stub})
        self.assertEqual(len(logs), 2)
        self.assertEqual(logs[0]["trace_id"], "ab" * 16)
        self.assertNotEqual(logs[0]["span_id"], "cd" * 8)
        self.assertNotEqual(logs[1]["trace_id"], "ab" * 16)
        for line in logs:
            self.assertEqual((line["task"], line["status"]), ("research-stub-v1", "ok"))
            self.assertGreater(line["input_bytes"], 0)

    def test_rejected_input_and_error_text_never_reach_the_log(self):
        fragment = "FIXTUREFRAGMENT"
        out, logs = self.run_worker({"task": fragment, "input": {}, "traceparent": PARENT},
                                    {"task": "research-stub-v1", "input": {"symbol": fragment.lower(), "mode": "PAPER"}})
        self.assertNotIn(fragment.encode(), out.stderr)
        self.assertNotIn(fragment.lower().encode(), out.stderr)
        self.assertEqual([(x["task"], x["status"], x["error_type"]) for x in logs],
                         [("unknown", "REJECTED", "ValueError"), ("research-stub-v1", "REJECTED", "ValueError")])

    def test_traceparent_is_the_only_extra_envelope_field(self):
        out, logs = self.run_worker({"task": "research-stub-v1", "input": {"symbol": "AAPL", "mode": "PAPER"}, "trace": PARENT})
        self.assertEqual(json.loads(out.stdout)["status"], "REJECTED")
        self.assertEqual(logs[0]["status"], "REJECTED")


if __name__ == "__main__":
    unittest.main()

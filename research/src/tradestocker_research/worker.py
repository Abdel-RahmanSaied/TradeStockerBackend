"""research-worker: the Python research entrypoint.

Reads one JSON request per line on stdin and writes one JSON result per line on
stdout. A request is {"task": <name>, "input": <object>}, with an optional
"traceparent" (W3C) that the request's log line continues. Each request logs
one JSON line on stderr with its task, outcome, sizes and duration, never its
input or error text. Its code opens no network, database or file connection and
is given no credentials: the caller hands it stored source documents. It runs
with the caller's OS permissions, so this is a code boundary, not a sandbox.
"""
import json
import re
import sys
import time

from tradestocker_research import financials, telemetry

STUB_VERSION = "research-stub-v1"
# A request line carries the task envelope around at most one parser input.
LINE_LIMIT = financials.INPUT_LIMIT + 4096
# The same errors the parser process treats as a rejected input.
REJECTED = (ValueError, TypeError, KeyError, AttributeError, RecursionError)


def assess(request):
    if not isinstance(request, dict) or set(request) - {"symbol", "mode"}:
        raise ValueError("Only symbol and mode are accepted")
    if request.get("mode") != "PAPER":
        raise ValueError("Only PAPER mode is implemented")
    symbol = request.get("symbol")
    if not isinstance(symbol, str) or not re.fullmatch(r"[A-Z0-9.:-]{1,32}", symbol):
        raise ValueError("A bounded instrument symbol is required")
    return {
        "symbol": symbol,
        "mode": "PAPER",
        "status": "STUB",
        "outcome": "INSUFFICIENT_DATA",
        "evidence": [],
        "version": STUB_VERSION,
        "reason": "No licensed financial statements, historical analysis or model connected",
        "financial_state_write_access": False,
    }


TASKS = {financials.VERSION: financials.assess, STUB_VERSION: assess}


def handle(request):
    if not isinstance(request, dict) or set(request) - {"traceparent"} != {"task", "input"}:
        raise ValueError("A request is exactly {task, input}, with an optional traceparent")
    task = TASKS.get(request["task"]) if isinstance(request["task"], str) else None
    if task is None:
        raise ValueError("Unknown task; known: %s" % ", ".join(sorted(TASKS)))
    return task(request["input"])


def lines(stream, limit):
    """Yields each line, or None for a line over the limit (its rest is dropped)."""
    while True:
        line = stream.readline(limit + 1)
        if not line:
            return
        if len(line) > limit:
            while line and not line.endswith(b"\n"):
                line = stream.readline(limit + 1)
            yield None
        elif line.strip():
            yield line


def main(argv=None):
    if (sys.argv[1:] if argv is None else argv):
        print("usage: research-worker < requests.jsonl", file=sys.stderr)
        return 2
    for line in lines(sys.stdin.buffer, LINE_LIMIT):
        started = time.monotonic()
        request = None
        error_type = None
        try:
            if line is None:
                raise ValueError("Request line exceeds limit")
            request = json.loads(line)
            output = handle(request)
        except REJECTED as error:
            output = {"status": "REJECTED", "error": "%s: %s" % (type(error).__name__, error)}
            error_type = type(error).__name__
        # Same encoding as the parser process, so a report is byte-identical.
        text = json.dumps(output, allow_nan=False, separators=(",", ":"))
        print(text, flush=True)
        log_request(request, line, text, error_type, started)
    return 0


def log_request(request, line, text, error_type, started):
    """One log line per request: names, sizes and timing only."""
    envelope = request if isinstance(request, dict) else {}
    task = envelope.get("task")
    fields = {
        "task": task if isinstance(task, str) and task in TASKS else "unknown",
        "status": "REJECTED" if error_type else "ok",
        "duration_ms": int((time.monotonic() - started) * 1000),
        "output_bytes": len(text),
    }
    if line is not None:
        fields["input_bytes"] = len(line)
    if error_type:
        fields["error_type"] = error_type
    telemetry.event("research task ended", telemetry.continue_trace(envelope.get("traceparent")), **fields)


if __name__ == "__main__":
    sys.exit(main())

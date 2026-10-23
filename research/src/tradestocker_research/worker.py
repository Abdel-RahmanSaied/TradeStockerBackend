"""research-worker: the Python research entrypoint.

Reads one JSON request per line on stdin and writes one JSON result per line on
stdout. A request is {"task": <name>, "input": <object>}. Its code opens no
network, database or file connection and is given no credentials: the caller
hands it stored source documents. It runs with the caller's OS permissions, so
this is a code boundary, not a sandbox.
"""
import json
import re
import sys

from tradestocker_research import financials

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
    if not isinstance(request, dict) or set(request) != {"task", "input"}:
        raise ValueError("A request is exactly {task, input}")
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
        try:
            if line is None:
                raise ValueError("Request line exceeds limit")
            output = handle(json.loads(line))
        except REJECTED as error:
            output = {"status": "REJECTED", "error": "%s: %s" % (type(error).__name__, error)}
        # Same encoding as the parser process, so a report is byte-identical.
        print(json.dumps(output, allow_nan=False, separators=(",", ":")), flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())

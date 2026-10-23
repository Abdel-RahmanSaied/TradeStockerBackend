"""Runs the SEC parser for platform-core and logs one safe event in the job's trace.

platform-core runs this file with `python -I -c`, followed by one line it adds:
sys.exit(launch(<financials source>, <telemetry source>)), with the exact
sources of tradestocker_research/financials.py and telemetry.py (embedded in
the Go binary) as string literals. The job's W3C traceparent is the only
argument.

financials.py runs unchanged, as a module, so its own __main__ block does not
run; main() reads the request on stdin and writes the report on stdout, so
stdout is exactly what the parser alone writes. The event goes to stderr. It
carries only names, the outcome and the duration, never input or error text;
platform-core relays it only if it matches the event schema and the job's trace.
"""
import sys
import time
import types


def load(name, source):
    module = types.ModuleType(name)
    exec(compile(source, name + ".py", "exec"), module.__dict__)
    return module


def launch(financials_source, telemetry_source):
    financials = load("financials", financials_source)
    telemetry = load("telemetry", telemetry_source)
    trace = telemetry.continue_trace(sys.argv[1] if len(sys.argv) > 1 else None)
    started = time.monotonic()
    code = financials.main()
    telemetry.event("research parser ended", trace, task=financials.VERSION, status="ok" if code == 0 else "REJECTED",
                    duration_ms=int((time.monotonic() - started) * 1000))
    return code

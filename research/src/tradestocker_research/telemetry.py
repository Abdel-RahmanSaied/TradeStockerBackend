"""Trace correlation and safe log events for the research worker.

A W3C traceparent ("00-<32 hex>-<16 hex>-<2 hex>") from the caller is
continued with a new span; a missing or malformed one starts a new trace, as
the Go side does. An event is one JSON line on stderr carrying only fixed
fields: event and task names, outcome and error type names, sizes and
durations. Request input, document content and error messages are never
logged, however short.
"""
import datetime
import json
import re
import secrets
import sys

TRACEPARENT = re.compile(r"00-([0-9a-f]{32})-([0-9a-f]{16})-[0-9a-f]{2}")
# The only fields an event may carry. Text values must be short names.
FIELDS = {"task", "status", "error_type", "duration_ms", "input_bytes", "output_bytes"}
NAME = re.compile(r"[A-Za-z][A-Za-z0-9_.-]{0,63}")


def parse(header):
    """Returns (trace_id, span_id) for a well-formed header, else None."""
    if not isinstance(header, str):
        return None
    match = TRACEPARENT.fullmatch(header)
    if match is None or set(match.group(1)) == {"0"} or set(match.group(2)) == {"0"}:
        return None
    return match.group(1), match.group(2)


def continue_trace(header):
    """A new span in the caller's trace, or a new trace."""
    parent = parse(header)
    trace_id = parent[0] if parent else secrets.token_hex(16)
    return trace_id, secrets.token_hex(8)


def event(name, trace, stream=None, **fields):
    """Writes one JSON log line. A field outside FIELDS is a programming error;
    a text value that is not a short name is replaced by "[omitted]"."""
    unknown = set(fields) - FIELDS
    if unknown:
        raise ValueError("unknown log fields: %s" % ", ".join(sorted(unknown)))
    record = {
        "time": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="milliseconds"),
        "level": "INFO",
        "msg": name,
        "trace_id": trace[0],
        "span_id": trace[1],
    }
    for key, value in sorted(fields.items()):
        if isinstance(value, int) and not isinstance(value, bool):
            record[key] = value
        elif isinstance(value, str) and NAME.fullmatch(value):
            record[key] = value
        else:
            record[key] = "[omitted]"
    print(json.dumps(record, separators=(",", ":")), file=stream or sys.stderr, flush=True)

"""Fixture adapter: marks research-worker results from synthetic fixtures as such.

research-worker reports describe official SEC filings, because that is what the
parser normally reads. For the invented fixtures that would be false. This step
reads research-worker's output on stdin and the fixture request file named on
the command line, and writes each result with:
  - source SYNTHETIC_FIXTURE and data_kind SYNTHETIC instead of the SEC values;
  - every filing_url set to null (the filings do not exist);
  - a provenance block naming the fixture file, its SHA-256 and the line.
It fails closed (exit 1, nothing written) if the result count differs from the
request count, or if any official-looking source, SEC link, entry, backtest or
write permission is left. Standard library only; run with python -I.
    research-worker < requests.jsonl | python3 -I label.py requests.jsonl
"""
import hashlib
import json
import sys

NOTICE = "SYNTHETIC: invented for local development and tests; not real market, filing or account data"
FORBIDDEN = ("sec.gov", "SEC_PUBLIC", "OFFICIAL_FILINGS")
GRANTS = ("eligible_for_entry", "eligible_for_backtest", "financial_state_write_access")


def unlink(value):
    """Copy of value with every filing_url set to None."""
    if isinstance(value, dict):
        return {k: (None if k == "filing_url" else unlink(v)) for k, v in value.items()}
    if isinstance(value, list):
        return [unlink(v) for v in value]
    return value


def grants(value):
    """Every permission flag anywhere in value that is not exactly False."""
    found = []
    if isinstance(value, dict):
        for k, v in value.items():
            if k in GRANTS and v is not False:
                found.append(k)
            found.extend(grants(v))
    elif isinstance(value, list):
        for v in value:
            found.extend(grants(v))
    return found


def label(result, fixture, digest, line):
    out = unlink(result)
    if "version" in out:
        out["source"] = "SYNTHETIC_FIXTURE"
        if "data_kind" in out:
            out["data_kind"] = "SYNTHETIC"
        if "scope" in out:
            out["scope"] = "Invented fixture data for local development; not SEC filings and not real company figures"
    out["provenance"] = {"data": "SYNTHETIC", "fixture": fixture, "fixture_sha256": digest, "line": line,
                         "notice": NOTICE, "labelled_by": "fixtures/synthetic/label.py"}
    return out


def main(argv):
    if len(argv) != 2:
        print("usage: research-worker < requests.jsonl | python3 -I label.py requests.jsonl", file=sys.stderr)
        return 2
    with open(argv[1], "rb") as f:
        raw = f.read()
    digest = hashlib.sha256(raw).hexdigest()
    requests = [line for line in raw.splitlines() if line.strip()]
    results = [json.loads(line) for line in sys.stdin.buffer.read().splitlines() if line.strip()]
    if len(results) != len(requests):
        print("label: %d results for %d requests; nothing written" % (len(results), len(requests)), file=sys.stderr)
        return 1
    fixture = "research/" + argv[1].replace("\\", "/").rsplit("/", 1)[-1]
    lines = []
    for n, result in enumerate(results, 1):
        text = json.dumps(label(result, fixture, digest, n), allow_nan=False, separators=(",", ":"), sort_keys=True)
        left = [s for s in FORBIDDEN if s in text] + grants(json.loads(text))
        if left:
            print("label: result %d still claims %s; nothing written" % (n, ", ".join(left)), file=sys.stderr)
            return 1
        lines.append(text + "\n")
    sys.stdout.write("".join(lines))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))

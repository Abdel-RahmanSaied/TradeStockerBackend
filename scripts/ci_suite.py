"""Runs CI test suites and reports their real counts (make ci-*). Standard library only.

    python3 scripts/ci_suite.py run --name NAME --out DIR [--go-json] -- COMMAND...
    python3 scripts/ci_suite.py report DIR

run executes COMMAND, keeps its full output in DIR/NAME.log and writes
DIR/NAME.json with the exit code and the counts it printed: unittest's summary
line, or, with --go-json, the top-level test events of `go test -json`. It
exits with the command's exit code, or 1 if no counts could be read.

report prints every suite in DIR, the Go tests that were skipped and why, and
the suites CI does not run at all. It also appends that to $GITHUB_STEP_SUMMARY
when set. It exits 1 if any suite failed or has no counts.
"""
import json
import os
from pathlib import Path
import re
import subprocess
import sys

# Suites that exist but are not run by CI, and why. Listed in every report so a
# green CI run is never read as covering them.
NOT_RUN = [
    ("Browser E2E (scripts/test-e2e.sh)", "needs the sibling frontend repository and the local PostgreSQL; run locally"),
    ("Development stack smoke (scripts/dev-stack-smoke.sh)", "needs Docker and the sibling frontend; run locally"),
    ("Real SEC research (scripts/research-smoke.sh, TS_E2E_REAL_SEC=1)", "makes requests to data.sec.gov with the owner-approved contact; never in CI"),
    ("Paper read benchmark (scripts/paper-read-bench.sh)", "a performance measurement, not a pass/fail test"),
    ("Frontend unit tests, build and audit", "in the TradeStockerFrontend repository"),
]
UNITTEST_RAN = re.compile(r"^Ran (\d+) tests? in ", re.M)
UNITTEST_RESULT = re.compile(r"^(OK|FAILED)(?: \(([^)]*)\))?\s*$", re.M)


def unittest_counts(text):
    ran = UNITTEST_RAN.findall(text)
    results = UNITTEST_RESULT.findall(text)
    if not ran or not results:
        return None
    counts = {"ran": sum(int(n) for n in ran), "failures": 0, "errors": 0, "skipped": 0, "expected_failures": 0}
    for _, detail in results:
        for part in filter(None, (p.strip() for p in detail.split(","))):
            key, _, value = part.partition("=")
            key = {"expected failures": "expected_failures"}.get(key, key)
            if key in counts and value.isdigit():
                counts[key] += int(value)
    return counts


def go_counts(text):
    tests, reasons, output = {}, {}, {}
    for line in text.splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        name, package = event.get("Test"), event.get("Package")
        if not name or "/" in name:
            continue
        key = "%s.%s" % (package, name)
        if event.get("Action") == "output":
            output.setdefault(key, []).append(event.get("Output", ""))
        elif event.get("Action") in ("pass", "fail", "skip"):
            tests[key] = event["Action"]
    for key, action in tests.items():
        if action == "skip":
            lines = [text.strip() for text in output.get(key, []) if text.strip() and not text.strip().startswith(("=== RUN", "--- SKIP"))]
            reasons[key] = lines[0] if lines else "no reason printed"
    if not tests:
        return None
    return {"ran": len(tests), "passed": sum(a == "pass" for a in tests.values()), "failures": sum(a == "fail" for a in tests.values()),
            "errors": 0, "skipped": sum(a == "skip" for a in tests.values()), "skip_reasons": reasons}


def run(argv):
    args = {"go_json": False}
    while argv and argv[0] != "--":
        flag = argv.pop(0)
        if flag in ("--name", "--out"):
            args[flag[2:]] = argv.pop(0)
        elif flag == "--go-json":
            args["go_json"] = True
        else:
            raise SystemExit("unknown option %s" % flag)
    if not argv or "name" not in args or "out" not in args or not re.fullmatch(r"[a-z0-9-]+", args["name"]):
        raise SystemExit("usage: ci_suite.py run --name NAME --out DIR [--go-json] -- COMMAND...")
    command = argv[1:]
    out = Path(args["out"])
    out.mkdir(parents=True, exist_ok=True)
    proc = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    sys.stdout.write(proc.stdout if not args["go_json"] else "".join(
        json.loads(line).get("Output", "") for line in proc.stdout.splitlines() if line.startswith("{") and '"Output"' in line))
    (out / (args["name"] + ".log")).write_text(proc.stdout)
    counts = go_counts(proc.stdout) if args["go_json"] else unittest_counts(proc.stdout)
    record = {"name": args["name"], "command": command, "exit": proc.returncode, "counts": counts}
    (out / (args["name"] + ".json")).write_text(json.dumps(record, indent=2, sort_keys=True) + "\n")
    if counts is None:
        print("ci_suite: %s printed no test counts" % args["name"], file=sys.stderr)
        return proc.returncode or 1
    return proc.returncode


def report(directory):
    records = [json.loads(p.read_text()) for p in sorted(Path(directory).glob("*.json")) if p.name not in ("gitleaks.json", "govulncheck.json")]
    lines = ["## Test suites", "", "| Suite | Exit | Ran | Skipped | Failed | Errors |", "|---|---|---|---|---|---|"]
    bad = False
    for r in records:
        c = r.get("counts")
        if c is None:
            lines.append("| %s | %s | no counts | | | |" % (r["name"], r["exit"]))
            bad = True
            continue
        lines.append("| %s | %s | %s | %s | %s | %s |" % (r["name"], r["exit"], c["ran"], c["skipped"], c["failures"], c["errors"]))
        bad = bad or r["exit"] != 0 or c["failures"] or c["errors"]
    if not records:
        lines.append("| (none) | | | | | |")
        bad = True
    skips = [(name, reason) for r in records for name, reason in sorted(((r.get("counts") or {}).get("skip_reasons") or {}).items())]
    if skips:
        lines += ["", "## Skipped Go tests", ""] + ["- `%s`: %s" % s for s in skips]
    lines += ["", "## Not run by CI", ""] + ["- %s: %s" % s for s in NOT_RUN]
    text = "\n".join(lines) + "\n"
    print(text)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a") as f:
            f.write(text)
    return 1 if bad else 0


def main(argv):
    if argv[:1] == ["run"]:
        return run(argv[1:])
    if argv[:1] == ["report"] and len(argv) == 2:
        return report(argv[1])
    print(__doc__, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))

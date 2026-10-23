"""Gate for the CI security scans (make ci-security). Standard library only.

    python -I scripts/scan_checks.py waivers
    python -I scripts/scan_checks.py gitleaks    REPORT.json
    python -I scripts/scan_checks.py govulncheck REPORT.json
    python -I scripts/scan_checks.py osv         research/uv.lock tools/uv-requirements.txt

Each scanner command fails (exit 1) on any finding that no valid waiver in
security/scan-waivers.json names exactly. A waiver is valid only with an
owner, a different reviewer, a review date and an expiry date in the future.
The waivers command also fails on an invalid waiver. Findings are printed
without secret values (gitleaks reports are produced with --redact).
"""
import datetime
import json
from pathlib import Path
import re
import sys
import tomllib
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
WAIVERS = ROOT / "security" / "scan-waivers.json"
SCANNERS = {"gitleaks", "govulncheck", "osv"}
OSV_URL = "https://api.osv.dev/v1/querybatch"


def load_waivers(path=WAIVERS, today=None):
    """Returns (valid waivers by scanner, problems)."""
    today = today or datetime.date.today()
    data = json.loads(Path(path).read_text())
    valid, problems, seen = {s: {} for s in SCANNERS}, [], set()
    for w in data.get("waivers", []):
        wid = w.get("id", "?")
        if wid in seen:
            problems.append("%s: duplicate id" % wid)
        seen.add(wid)
        missing = [k for k in ("scanner", "match", "reason", "owner", "reviewed_by", "reviewed_on", "expires") if not w.get(k)]
        if missing:
            problems.append("%s: missing %s" % (wid, ", ".join(missing)))
            continue
        if w["scanner"] not in SCANNERS:
            problems.append("%s: unknown scanner %r" % (wid, w["scanner"]))
            continue
        if w["reviewed_by"] == w["owner"]:
            problems.append("%s: reviewed by its own owner" % wid)
            continue
        try:
            expires = datetime.date.fromisoformat(w["expires"])
            reviewed = datetime.date.fromisoformat(w["reviewed_on"])
        except ValueError:
            problems.append("%s: dates must be YYYY-MM-DD" % wid)
            continue
        if expires <= today:
            problems.append("%s: expired on %s" % (wid, expires))
            continue
        if reviewed > today:
            problems.append("%s: review date is in the future" % wid)
            continue
        valid[w["scanner"]][w["match"]] = wid
    return valid, problems


def gate(scanner, findings, waivers):
    """findings: list of (match key, printable description). Returns exit code."""
    waived = waivers[scanner]
    used = set()
    open_findings = []
    for key, text in findings:
        if key in waived:
            used.add(key)
            print("waived by %s: %s" % (waived[key], text))
        else:
            open_findings.append(text)
    unused = sorted(waived[k] for k in set(waived) - used)
    for text in open_findings:
        print("FINDING: %s" % text)
    for wid in unused:
        print("UNUSED WAIVER: %s matches no %s finding; remove it after review" % (wid, scanner))
    print("%s: %d findings, %d waived, %d open, %d unused waivers" % (scanner, len(findings), len(used), len(open_findings), len(unused)))
    return 1 if open_findings or unused else 0


def gitleaks_findings(report):
    rows = json.loads(Path(report).read_text() or "[]")
    for row in rows:
        if row.get("Secret") not in ("REDACTED", None) and len(row.get("Secret", "")) > 12:
            raise SystemExit("gitleaks report is not redacted; refusing to print it (run with --redact)")
    return [(r["Fingerprint"], "%s in %s line %s (commit %s)" % (r["RuleID"], r["File"], r["StartLine"], r["Commit"][:12])) for r in rows]


def govulncheck_findings(report):
    """Symbol-level findings only: vulnerable code this module actually reaches."""
    findings = {}
    decoder, text, pos = json.JSONDecoder(), Path(report).read_text(), 0
    while True:
        while pos < len(text) and text[pos].isspace():
            pos += 1
        if pos >= len(text):
            break
        obj, pos = decoder.raw_decode(text, pos)
        finding = obj.get("finding")
        if finding and finding.get("trace") and finding["trace"][0].get("function"):
            top = finding["trace"][0]
            findings[finding["osv"]] = "%s via %s.%s (fixed in %s)" % (finding["osv"], top.get("package", "?"), top["function"], finding.get("fixed_version") or "no fix")
    return sorted(findings.items())


def locked_packages(lock, requirements):
    packages = []
    for p in tomllib.loads(Path(lock).read_text()).get("package", []):
        if "registry" in p.get("source", {}):
            packages.append((p["name"], p["version"]))
    for line in Path(requirements).read_text().splitlines():
        m = re.match(r"^([A-Za-z0-9_.-]+)==([^\s\\]+)", line)
        if m:
            packages.append((m.group(1), m.group(2)))
    return sorted(set(packages))


def osv_findings(packages, query=None):
    query = query or osv_query
    results = query([{"package": {"name": n, "ecosystem": "PyPI"}, "version": v} for n, v in packages])
    findings = []
    for (name, version), result in zip(packages, results):
        for vuln in result.get("vulns") or []:
            findings.append(("%s==%s:%s" % (name, version, vuln["id"]), "%s %s: %s" % (name, version, vuln["id"])))
    return findings


def osv_query(queries):
    request = urllib.request.Request(OSV_URL, data=json.dumps({"queries": queries}).encode(), headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=60) as response:
        results = json.load(response)["results"]
    if len(results) != len(queries):
        raise SystemExit("OSV answered %d results for %d queries" % (len(results), len(queries)))
    return results


def main(argv):
    if not argv or argv[0] not in ("waivers", "gitleaks", "govulncheck", "osv"):
        print(__doc__, file=sys.stderr)
        return 2
    waivers, problems = load_waivers()
    for problem in problems:
        print("INVALID WAIVER: %s" % problem)
    if argv[0] == "waivers":
        print("waivers: %d valid, %d invalid" % (sum(len(v) for v in waivers.values()), len(problems)))
        return 1 if problems else 0
    if argv[0] == "gitleaks":
        code = gate("gitleaks", gitleaks_findings(argv[1]), waivers)
    elif argv[0] == "govulncheck":
        code = gate("govulncheck", govulncheck_findings(argv[1]), waivers)
    else:
        packages = locked_packages(argv[1], argv[2])
        print("osv: checking %d PyPI packages" % len(packages))
        code = gate("osv", osv_findings(packages), waivers)
    return 1 if code or problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))

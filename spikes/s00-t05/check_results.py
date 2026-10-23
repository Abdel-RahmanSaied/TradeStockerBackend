"""Check the spike outputs in .local/ and print one summary. Exits 1 on any failure."""
import importlib.metadata
import json
import platform
import sys
from pathlib import Path

out = Path(sys.argv[1])
here = Path(__file__).resolve().parent
load = lambda name: json.loads((out / name).read_text())
failures = []


def expect(ok, what):
    if not ok:
        failures.append(what)


go = load("go-jetstream.json")
expect(go["passed"], "Go outbox/JetStream/inbox: %s" % go["failures"])

nats = load("python-nats.json")
expect(not nats["first_duplicate"] and nats["second_duplicate"], "Python: JetStream must flag only the repeated message id")
expect(nats["delivered"] == 1 and nats["extra_after_ack"] == 0, "Python: exactly one stored and delivered message")

start, resume = load("python-checkpoint-start.json"), load("python-checkpoint-resume.json")
expect(start["steps"] == ["collect", "assess"] and start["next"] == ["report"], "checkpoint: first process must pause before report")
expect(resume["steps"] == ["collect", "assess", "report"] and resume["next"] == [], "checkpoint: second process must finish from the checkpoint")
expect(resume["pid"] != start["pid"], "checkpoint: resume must run in a different process")
expect(resume["node_runs"] == {"collect": 1, "assess": 1, "report": 1}, "checkpoint: every node must run exactly once, got %s" % resume["node_runs"])

pinned = (here / "python" / ".python-version").read_text().strip()
expect(platform.python_version() == pinned, "Python %s ran, but %s is pinned" % (platform.python_version(), pinned))
osv = load("osv.json")
expect(not osv["advisories"], "OSV advisories: %s" % osv["advisories"])
python = {"interpreter": platform.python_version(), "implementation": platform.python_implementation(),
          "packages": {p: importlib.metadata.version(p) for p in ("langgraph", "langgraph-checkpoint", "langgraph-checkpoint-postgres",
                                                                   "langchain-core", "psycopg", "psycopg-binary", "psycopg-pool", "nats-py", "certifi")}}

profile = load("load-profile.json")
for phase in profile["phases"]:
    expect(phase["errors"] == 0 and phase["requests"] > 0, "load %s: %d errors in %d requests" % (phase["name"], phase["errors"], phase["requests"]))

summary = {"versions": go["versions"], "python": python, "osv": {"packages_checked": osv["packages_checked"], "advisories": osv["advisories"]},
           "python_nats_server": nats["server_version"],
           "jetstream": {k: go[k] for k in ("messages", "publish_per_second", "publish_p50_ms", "publish_p95_ms", "republished",
                                            "duplicates_flagged_by_jetstream", "unacked_on_purpose", "redeliveries", "consume_per_second",
                                            "stream_messages", "stream_bytes")},
           "sample_storage_bytes": go.get("sample_storage_bytes"),
           "checkpoint": {"pause_checkpoint": start["checkpoint_id"], "resume_checkpoint": resume["checkpoint_id"], "node_runs": resume["node_runs"]},
           "load": [{k: p[k] for k in ("name", "concurrency", "requests", "per_second", "p50_ms", "p95_ms", "p99_ms", "last_response_bytes")} for p in profile["phases"]],
           "paper_after_load": {"recommendations": profile["recommendations_after"], "audit_events": profile["audit_events_after"], **load("paper-size.json")},
           "failures": failures, "passed": not failures}
print(json.dumps(summary, indent=1))
sys.exit(1 if failures else 0)

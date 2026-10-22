"""Disposable test state; never write this fixture over the real execution state."""
import copy


def bootstrap_state(catalog, current):
    state = copy.deepcopy(current)
    state.update(revision=0, history=[], last_session="fixture-bootstrap",
                 last_handoff="sessions/2026-10-23-plan-bootstrap.md",
                 live_advisory_enabled=False, active_sprint="S00")
    state["repo_observation"] = {"status": "NOT_INSPECTED", "root": None,
                                 "branch": None, "head": None, "working_tree": None}
    for record in state["lanes"].values():
        record.update(state="DISABLED", approval_evidence=None)
    for record in state["decision_status"].values():
        record.update(status="OPEN", evidence=None)
    state["tasks"] = {}
    for task in catalog["tasks"]:
        record = {field: None for field in (
            "owner", "session_id", "started_at", "updated_at", "blocked_reason",
            "verified_commit", "reviewed_by", "completion_summary")}
        record.update(status="TODO", evidence=[])
        state["tasks"][task["id"]] = record
    return state

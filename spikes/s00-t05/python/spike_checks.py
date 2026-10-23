"""S00-T05 compatibility checks for the pinned Python stack. No model or LLM calls.

    spike_checks.py checkpoint start   # run until the graph pauses, then exit
    spike_checks.py checkpoint resume  # a new process resumes from PostgreSQL
    spike_checks.py nats               # JetStream publish de-duplication and pull

Needs SPIKE_PG_URL and SPIKE_NATS_URL (the spike's own disposable services).
"""
import asyncio
import json
import os
import sys
from typing import TypedDict

import nats
import nats.errors
import nats.js.errors
import psycopg
from langgraph.checkpoint.postgres import PostgresSaver
from langgraph.graph import END, START, StateGraph

THREAD = "spike-thread-1"


class State(TypedDict):
    steps: list[str]


def count_run(url, node):
    # A durable side effect per node execution: re-running a node shows up here.
    with psycopg.connect(url, autocommit=True) as conn:
        conn.execute("CREATE TABLE IF NOT EXISTS spike_node_runs(thread text, node text, runs int NOT NULL, PRIMARY KEY (thread, node))")
        conn.execute("INSERT INTO spike_node_runs VALUES (%s, %s, 1) ON CONFLICT (thread, node) DO UPDATE SET runs = spike_node_runs.runs + 1", (THREAD, node))


def graph_builder(url):
    def node(name):
        def run(state):
            count_run(url, name)
            return {"steps": state["steps"] + [name]}
        return run

    builder = StateGraph(State)
    for name in ("collect", "assess", "report"):
        builder.add_node(name, node(name))
    builder.add_edge(START, "collect")
    builder.add_edge("collect", "assess")
    builder.add_edge("assess", "report")
    builder.add_edge("report", END)
    return builder


def checkpoint(phase):
    url = os.environ["SPIKE_PG_URL"]
    with PostgresSaver.from_conn_string(url) as saver:
        saver.setup()
        graph = graph_builder(url).compile(checkpointer=saver, interrupt_before=["report"])
        config = {"configurable": {"thread_id": THREAD}}
        result = graph.invoke({"steps": []} if phase == "start" else None, config)
        snapshot = graph.get_state(config)
    with psycopg.connect(url) as conn:
        runs = dict(conn.execute("SELECT node, runs FROM spike_node_runs WHERE thread = %s", (THREAD,)).fetchall())
    return {"phase": phase, "pid": os.getpid(), "steps": result.get("steps"), "next": list(snapshot.next),
            "checkpoint_id": snapshot.config["configurable"].get("checkpoint_id"), "node_runs": runs}


async def jetstream():
    nc = await nats.connect(os.environ["SPIKE_NATS_URL"], name="tradestocker-spike-python")
    js = nc.jetstream()
    try:
        await js.delete_stream("SPIKE_PY")
    except nats.js.errors.NotFoundError:
        pass
    await js.add_stream(name="SPIKE_PY", subjects=["spike.py"], duplicate_window=120.0)
    body = json.dumps({"id": "py-1", "data_label": "SPIKE_SYNTHETIC"}).encode()
    first = await js.publish("spike.py", body, headers={"Nats-Msg-Id": "py-1"})
    second = await js.publish("spike.py", body, headers={"Nats-Msg-Id": "py-1"})
    sub = await js.pull_subscribe("spike.py", durable="spike-py")
    got = await sub.fetch(1, timeout=5)
    for msg in got:
        await msg.ack()
    try:
        extra = await sub.fetch(1, timeout=1)
    except nats.errors.TimeoutError:
        extra = []
    version = str(nc.connected_server_version)
    await js.delete_stream("SPIKE_PY")
    await nc.close()
    return {"server_version": version, "first_duplicate": bool(first.duplicate), "second_duplicate": bool(second.duplicate),
            "delivered": len(got), "extra_after_ack": len(extra)}


def main():
    if sys.argv[1:2] == ["checkpoint"] and sys.argv[2:3] in (["start"], ["resume"]):
        out = checkpoint(sys.argv[2])
    elif sys.argv[1:] == ["nats"]:
        out = asyncio.run(jetstream())
    else:
        raise SystemExit(__doc__)
    print(json.dumps(out))


if __name__ == "__main__":
    main()

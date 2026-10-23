"""Invalid references stop the generator before it generates anything (S02-T01, 049).

Every case is bounded without a timeout: the documents are in memory, the
shared compiler refuses reference cycles, resolve() is wrapped by a call
counter that fails the test instead of looping, and every generation step
(documents, events, gofmt, compatibility) is a sentinel that fails the test if
it is reached. No file is written and no subprocess runs.
"""
import contextlib
import importlib.util
import io
import json
import pathlib
import types
import unittest
from unittest import mock

TOOLS = pathlib.Path(__file__).resolve().parent
CONTRACTS = TOOLS.parent


def load(name):
    spec = importlib.util.spec_from_file_location(name, TOOLS / (name + ".py"))
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


gen = load("generate_contracts")
compat = load("compat")
C = compat.C

TEXTS = compat.load_documents(CONTRACTS)
RAC = "events/v1/research.assessment.completed.v1.schema.json"
COMMON = "events/v1/common.schema.json"
REAL_RESOLVE = gen.resolve
RESOLVE_LIMIT = 1000


def with_loop(kind):
    """The contract texts with a typeless reference loop that an event field uses."""
    docs = {k: C.parse(v.encode("utf-8")) for k, v in TEXTS.items()}
    defs = docs[COMMON]["$defs"]
    if kind == "self":
        defs["loop_a"] = {"$ref": "#/$defs/loop_a"}
    else:
        defs["loop_a"] = {"$ref": "#/$defs/loop_b"}
        defs["loop_b"] = {"$ref": "#/$defs/loop_a"}
    docs[RAC]["$defs"]["data"]["properties"]["status"] = {"$ref": "common.schema.json#/$defs/loop_a"}
    return {k: compat.dumps(v) for k, v in docs.items()}


def stub_compat(texts):
    """The real compiler and error, reading the given texts instead of contracts/."""
    return types.SimpleNamespace(surfaces=compat.surfaces, CompatError=compat.CompatError,
                                 load_documents=lambda root: texts)


class BoundedResolve:
    """gen.resolve with a call limit, so an unbounded walk fails instead of hanging."""

    def __init__(self):
        self.calls = 0

    def __call__(self, docs, base, ref):
        self.calls += 1
        if self.calls > RESOLVE_LIMIT:
            raise AssertionError("resolve called more than %d times: the reference walk is unbounded" % RESOLVE_LIMIT)
        return REAL_RESOLVE(docs, base, ref)


def reached(step):
    def sentinel(*args, **kwargs):
        raise AssertionError("%s was reached although preflight failed" % step)
    return sentinel


def chain(hops):
    """A document whose field reaches a string type after exactly `hops` typeless references."""
    defs = {"r%d" % i: {"$ref": "#/$defs/r%d" % (i + 1)} for i in range(hops - 1)}
    defs["r%d" % (hops - 1)] = {"type": "string"}
    return {"events/v1/x.json": json.dumps({"$defs": defs})}, {"$ref": "#/$defs/r0"}


class PreflightTests(unittest.TestCase):
    def test_the_real_contracts_pass_preflight(self):
        self.assertIsNone(gen.preflight())

    def test_preflight_refuses_a_typeless_reference_loop(self):
        for kind in ("self", "mutual"):
            with self.subTest(kind=kind), mock.patch.object(gen, "load_compat", lambda: stub_compat(with_loop(kind))):
                problem = gen.preflight()
                self.assertIsNotNone(problem)
                self.assertIn("SCHEMA_DOES_NOT_COMPILE", problem)
                self.assertIn("reference cycle", problem)

    def test_main_stops_before_generation_when_preflight_fails(self):
        for argv in ([], ["--check"], ["--lock"]):
            stderr = io.StringIO()
            with self.subTest(argv=argv), contextlib.redirect_stderr(stderr), \
                    mock.patch.object(gen, "load_compat", lambda: stub_compat(with_loop("mutual"))), \
                    mock.patch.object(gen, "documents", reached("documents")), \
                    mock.patch.object(gen, "events", reached("events")), \
                    mock.patch.object(gen, "compatibility", reached("compatibility")), \
                    mock.patch.object(gen, "subprocess", types.SimpleNamespace(run=reached("gofmt"))):
                self.assertEqual(gen.main(argv), 1)
                self.assertIn("nothing generated", stderr.getvalue())


class ReferenceWalkTests(unittest.TestCase):
    """The generator's own walk is bounded even if preflight were skipped."""

    def walk(self, docs, schema, base="events/v1/x.json"):
        with mock.patch.object(gen, "resolve", BoundedResolve()):
            return gen.field_type(docs, base, schema)

    def assertLoop(self, docs, schema, base="events/v1/x.json"):
        with self.assertRaises(SystemExit) as caught:
            self.walk(docs, schema, base)
        self.assertIn("reference loop", str(caught.exception.code))

    def test_a_self_reference_stops(self):
        docs = {"events/v1/x.json": json.dumps({"$defs": {"a": {"$ref": "#/$defs/a"}}})}
        self.assertLoop(docs, {"$ref": "#/$defs/a"})

    def test_a_mutual_reference_across_documents_stops(self):
        docs = {"events/v1/x.json": json.dumps({"$defs": {"a": {"$ref": "y.json#/$defs/b"}}}),
                "events/v1/y.json": json.dumps({"$defs": {"b": {"$ref": "x.json#/$defs/a"}}})}
        self.assertLoop(docs, {"$ref": "#/$defs/a"})

    def test_an_array_item_loop_stops(self):
        docs = {"events/v1/x.json": json.dumps({"$defs": {"a": {"$ref": "#/$defs/b"}, "b": {"$ref": "#/$defs/a"}}})}
        self.assertLoop(docs, {"type": "array", "items": {"$ref": "#/$defs/a"}})

    def test_the_loop_in_the_contract_documents_stops(self):
        texts = with_loop("mutual")
        self.assertLoop(texts, json.loads(texts[RAC])["$defs"]["data"]["properties"]["status"], RAC)

    def test_a_chain_is_followed_up_to_the_limit(self):
        docs, field = chain(gen.MAX_REF_STEPS)
        self.assertEqual(self.walk(docs, field), ("string", False))
        docs, field = chain(gen.MAX_REF_STEPS + 1)
        self.assertLoop(docs, field)


if __name__ == "__main__":
    unittest.main()

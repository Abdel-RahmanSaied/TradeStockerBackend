"""Cross-language contract checks (S02-T01): the same cases Go runs.

The conformance corpus and the contract documents are read from the
repository's contracts/ folder; the package itself never reads files.
"""
import base64
import json
from pathlib import Path
import unittest

from tradestocker_research import contracts

ROOT = Path(__file__).resolve().parents[2]
CONTRACTS = ROOT / "contracts"


def event_documents():
    return {"events/v1/" + p.name: p.read_text(encoding="utf-8") for p in sorted((CONTRACTS / "events/v1").glob("*.json"))}


def answer(case):
    docs = event_documents() if case.get("contracts") else {}
    docs.update(case.get("documents", {}))
    try:
        registry = contracts.Registry(docs)
    except contracts.SchemaError:
        return "SCHEMA_ERROR"
    raw = case["raw"].encode("utf-8") if "raw" in case else base64.b64decode(case.get("raw_b64", ""))
    try:
        value = contracts.parse(raw)
    except contracts.ParseError:
        return "MALFORMED_JSON"
    violation = registry.validate(case["ref"], value)
    if violation is None:
        return "valid"
    if violation.keyword == "schema":
        return "SCHEMA_ERROR"
    return violation.keyword + "@" + violation.path


def expected(expect):
    if isinstance(expect, str):
        return expect
    return expect.get("code") or expect["keyword"] + "@" + expect["path"]


class ConformanceCorpusTests(unittest.TestCase):
    def test_every_case_gives_the_expected_answer(self):
        corpus = json.loads((CONTRACTS / "fixtures/conformance/validator.v1.json").read_text(encoding="utf-8"))
        self.assertEqual(corpus["dialect"], contracts.DIALECT)
        names = [c["name"] for c in corpus["cases"]]
        self.assertEqual(len(names), len(set(names)))
        self.assertGreaterEqual(len(names), 150)
        for case in corpus["cases"]:
            with self.subTest(case=case["name"]):
                self.assertEqual(answer(case), expected(case["expect"]))


class ParserTests(unittest.TestCase):
    def test_numbers_keep_their_exact_text(self):
        value = contracts.parse(b'{"a":123456789012345678901234567890,"b":0.10,"c":-0}')
        self.assertEqual(value["a"], "123456789012345678901234567890")
        self.assertIsInstance(value["a"], contracts.Number)
        self.assertEqual(value["b"], "0.10")
        self.assertIsNone(value["b"].integer())
        self.assertEqual(contracts.canonical(value["c"]), "0")

    def test_booleans_are_never_integers(self):
        registry = contracts.Registry({"x.json": '{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"integer","minimum":0,"maximum":1}'})
        self.assertEqual(registry.validate("x.json", True).keyword, "type")

    def test_canonical_refuses_what_parse_never_returns(self):
        for value in [contracts.Number("007"), contracts.Number("1.5"), "\ud800", 1.5, object()]:
            with self.subTest(value=repr(value)), self.assertRaises(ValueError):
                contracts.canonical(value)
        nested = []
        for _ in range(70):
            nested = [nested]
        with self.assertRaises(ValueError):
            contracts.canonical(nested)

    def test_canonical_form(self):
        value = contracts.parse('{"b":"\\u00e9\\n\\u0001\\"","a":[true,null,-0,10]}'.encode())
        self.assertEqual(contracts.canonical(value), '{"a":[true,null,0,10],"b":"é\\n\\u0001\\""}')

    def test_input_limits(self):
        with self.assertRaises(contracts.ParseError):
            contracts.parse(b" " * (contracts.MAX_BYTES + 1))



class EventTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.registry = contracts.Registry(event_documents())

    def answer(self, raw):
        try:
            contracts.check_event(self.registry, raw)
        except contracts.EventError as error:
            if error.violation is not None:
                return "%s %s@%s" % (error.code, error.violation.keyword, error.violation.path)
            return error.code
        return "valid"

    def test_event_corpus(self):
        corpus = json.loads((CONTRACTS / "fixtures/conformance/events.v1.json").read_text(encoding="utf-8"))
        self.assertGreaterEqual(len(corpus["cases"]), 90)
        for case in corpus["cases"]:
            expect = case["expect"]
            want = expect if isinstance(expect, str) else (
                "%s %s@%s" % (expect["code"], expect["keyword"], expect["path"]) if "keyword" in expect else expect["code"])
            with self.subTest(case=case["name"]):
                self.assertEqual(self.answer(case["raw"].encode("utf-8")), want)

    def test_golden_events_round_trip_exactly(self):
        from tradestocker_research import events_v1
        fixtures = sorted((CONTRACTS / "fixtures/events/v1").glob("*.v1.json"))
        self.assertEqual(len(fixtures), len(events_v1.EVENT_TYPES))
        for path in fixtures:
            raw = path.read_bytes().rstrip(b"\n")
            with self.subTest(fixture=path.name):
                event = events_v1.decode_event(self.registry, raw)
                self.assertIsInstance(event.data, events_v1.EVENT_TYPES[event.event_type])
                self.assertEqual(events_v1.encode_event(event).encode("utf-8"), raw)

    def test_typed_values(self):
        from tradestocker_research import events_v1
        raw = (CONTRACTS / "fixtures/events/v1/portfolio.execution.recorded.v1.json").read_bytes()
        event = events_v1.decode_event(self.registry, raw.rstrip(b"\n"))
        self.assertEqual((event.data.price, event.data.quantity, event.account_id), ("199.85", "10", "acct_fixture_001"))
        self.assertEqual(event.schema_version, 1)
        self.assertNotIsInstance(event.schema_version, contracts.Number)

    def test_embedded_documents_match_the_contract_files(self):
        from tradestocker_research.contract_documents import DOCUMENTS
        self.assertEqual({k: v for k, v in DOCUMENTS.items() if k.startswith("events/")}, event_documents())


class ResearchContractTests(unittest.TestCase):
    """The Go/Python boundary that exists today: the SEC parser request and report."""

    @classmethod
    def setUpClass(cls):
        cls.registry = contracts.default_registry()

    def test_research_corpus(self):
        corpus = json.loads((CONTRACTS / "fixtures/conformance/research.v1.json").read_text(encoding="utf-8"))
        for case in corpus["cases"]:
            expect = case["expect"]
            want = expect if isinstance(expect, str) else expect["keyword"] + "@" + expect["path"]
            violation = self.registry.validate(case["ref"], contracts.parse(case["raw"].encode("utf-8")))
            with self.subTest(case=case["name"]):
                self.assertEqual("valid" if violation is None else violation.keyword + "@" + violation.path, want)

    def test_parser_writes_exactly_the_golden_report(self):
        from tradestocker_research import financials
        request = (CONTRACTS / "fixtures/research/sec-annual-v2.request.json").read_bytes()
        golden = (CONTRACTS / "fixtures/research/sec-annual-v2.report.json").read_bytes()
        report = financials.assess(json.loads(request))
        self.assertEqual(json.dumps(report, allow_nan=False, separators=(",", ":")).encode() + b"\n", golden)
        self.assertIsNone(self.registry.validate("research/sec-annual-v2.report.schema.json", contracts.parse(golden)))
        self.assertIsNone(self.registry.validate("research/sec-annual-v2.request.schema.json", contracts.parse(request)))

    def test_package_carries_every_contract_it_validates(self):
        from tradestocker_research.contract_documents import DOCUMENTS
        on_disk = {"%s/%s" % (folder, p.name): p.read_text(encoding="utf-8")
                   for folder in ("events/v1", "research") for p in sorted((CONTRACTS / folder).glob("*.json"))}
        self.assertEqual(DOCUMENTS, on_disk)


class CompileOrderTests(unittest.TestCase):
    """Review 044/045: the depth limit holds for subtrees compiled first."""

    def test_depth_limit_holds_for_prewarmed_subtrees(self):
        docs = {"a%02d.json" % i: ('{"$ref":"a%02d.json"}' % (i + 1)) if i < 69 else '{"type":"null"}' for i in range(70)}
        registry = contracts.Registry(docs)
        for i in range(69, -1, -1):
            fits = 70 - i <= contracts.MAX_SCHEMA_DEPTH
            with self.subTest(document=i):
                if fits:
                    registry.compile("a%02d.json" % i)
                else:
                    with self.assertRaises(contracts.SchemaError):
                        registry.compile("a%02d.json" % i)
        for _ in range(2):
            with self.assertRaisesRegex(contracts.SchemaError, "nest deeper"):
                registry.compile("a00.json")
        self.assertEqual(registry.validate("a00.json", None).keyword, "schema")
        registry.compile("a10.json")
        self.assertIsNone(registry.validate("a10.json", None))

    def test_pattern_counts_are_unsigned_digits(self):
        for pattern in ["^a{+1}$", "^a{-1}$", "^a{1,+2}$", "^a{1,-2}$", "^a{ 1}$", "^a{1 }$", "^a{0x1}$", "^a{1e1}$"]:
            with self.subTest(pattern=pattern), self.assertRaises(contracts.SchemaError):
                contracts._Pattern(pattern)
        for pattern in ["^a{0}$", "^a{1000}$", "^a{0,1000}$"]:
            contracts._Pattern(pattern)


class APIContractTests(unittest.TestCase):
    """The local API contract: Python checks the same answers and cases as Go."""

    @classmethod
    def setUpClass(cls):
        docs = {"openapi/local-api.v1.json": (CONTRACTS / "openapi/local-api.v1.json").read_text(encoding="utf-8")}
        for p in sorted((CONTRACTS / "research").glob("*.json")):
            docs["research/" + p.name] = p.read_text(encoding="utf-8")
        cls.registry = contracts.Registry(docs)
        cls.operations = {}
        for path, item in cls.registry.docs["openapi/local-api.v1.json"]["paths"].items():
            for method, op in item.items():
                cls.operations[op["operationId"]] = (path, method)

    def response_ref(self, operation_id, status):
        path, method = self.operations[operation_id]
        return "openapi/local-api.v1.json#/paths/%s/%s/responses/%s/content/application~1json/schema" % (
            path.replace("~", "~0").replace("/", "~1"), method, status)

    def test_captured_answers_match_their_documented_response(self):
        files = sorted((CONTRACTS / "fixtures/api/v1").glob("*.json"))
        self.assertGreaterEqual(len(files), 25)
        for path in files:
            operation_id, status = path.name.split(".")[:2]
            with self.subTest(fixture=path.name):
                violation = self.registry.validate(self.response_ref(operation_id, status), contracts.parse(path.read_bytes()))
                self.assertIsNone(violation)

    def test_api_corpus(self):
        corpus = json.loads((CONTRACTS / "fixtures/conformance/api.v1.json").read_text(encoding="utf-8"))
        for case in corpus["cases"]:
            expect = case["expect"]
            want = expect if isinstance(expect, str) else expect["keyword"] + "@" + expect["path"]
            violation = self.registry.validate(case["ref"], contracts.parse(case["raw"].encode("utf-8")))
            with self.subTest(case=case["name"]):
                self.assertEqual("valid" if violation is None else violation.keyword + "@" + violation.path, want)


if __name__ == "__main__":
    unittest.main()

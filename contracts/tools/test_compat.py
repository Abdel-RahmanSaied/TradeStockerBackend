"""Breaking schema changes fail the compatibility check (S02-T01).

Each test changes an in-memory copy of the contract documents (parsed with the
strict shared parser, so numbers keep their exact tokens); no file is written.
The expected diagnoses are written here by hand, not derived from the changed
schemas.
"""
import copy
import importlib.util
import pathlib
import unittest

TOOLS = pathlib.Path(__file__).resolve().parent
CONTRACTS = TOOLS.parent
spec = importlib.util.spec_from_file_location("compat", TOOLS / "compat.py")
assert spec is not None and spec.loader is not None
compat = importlib.util.module_from_spec(spec)
spec.loader.exec_module(compat)
C = compat.C

TEXTS = compat.load_documents(CONTRACTS)
LOCK = compat.load_lock(CONTRACTS / "compat/v1.lock.json")
RAC = "events/v1/research.assessment.completed.v1.schema.json"
COMMON = "events/v1/common.schema.json"
API = "openapi/local-api.v1.json"
REPORT = "research/sec-annual-v2.report.schema.json"


def parsed():
    return {k: C.parse(v.encode("utf-8")) for k, v in TEXTS.items()}


def texts(docs):
    return {k: compat.dumps(v) for k, v in docs.items()}


def findings(change, lock=LOCK):
    docs = parsed()
    change(docs)
    return compat.check(lock, texts(docs))


def breaking(found):
    return {(surface, kind, path) for surface, kind, _, path, _, is_breaking in found if is_breaking}


def schemas(docs):
    return docs[API]["components"]["schemas"]


class CompatibilityTests(unittest.TestCase):
    def test_current_contracts_match_the_lock(self):
        self.assertEqual(compat.check(LOCK, TEXTS), [])

    def test_documentation_changes_are_not_differences(self):
        def change(docs):
            docs[RAC]["description"] = "reworded"
            docs[COMMON]["$defs"]["lane"]["description"] = "reworded"
            schemas(docs)["Account"]["description"] = "reworded"
            docs[RAC]["x-tradestocker-runtime"] = "reworded"
        self.assertEqual(findings(change), [])

    def test_removing_a_required_field_breaks(self):
        def change(docs):
            data = docs[RAC]["$defs"]["data"]
            del data["properties"]["status"]
            data["required"].remove("status")
        self.assertEqual(breaking(findings(change)), {
            ("event v1/research.assessment.completed.v1", "PROPERTY_REMOVED", "/properties/data/$ref"),
            ("event v1/research.assessment.completed.v1", "REQUIRED_REMOVED", "/properties/data/$ref")})

    def test_renaming_a_field_breaks(self):
        def change(docs):
            data = docs[RAC]["$defs"]["data"]
            data["properties"]["outcome"] = data["properties"].pop("status")
            data["required"] = ["outcome" if r == "status" else r for r in data["required"]]
        kinds = {kind for _, kind, _ in breaking(findings(change))}
        self.assertEqual(kinds, {"PROPERTY_ADDED", "PROPERTY_REMOVED", "REQUIRED_ADDED", "REQUIRED_REMOVED"})

    def test_decimal_string_to_number_breaks_every_event_that_uses_it(self):
        def change(docs):
            d = docs[COMMON]["$defs"]["positive_decimal"]
            d["type"], d["minimum"], d["maximum"] = "integer", 1, 10**15
        found = breaking(findings(change))
        self.assertIn(("event v1/portfolio.execution.recorded.v1", "TYPE_CHANGED", "/properties/data/$ref/properties/price/$ref"), found)
        self.assertIn(("event v1/market.bar.finalized.v1", "TYPE_CHANGED", "/properties/data/$ref/properties/open/$ref"), found)
        self.assertNotIn("event v1/research.assessment.completed.v1", {s for s, _, _ in found})

    def test_removing_an_enum_value_breaks(self):
        def change(docs):
            docs[COMMON]["$defs"]["lane"]["enum"].remove("EGX_INTRADAY")
        found = findings(change)
        self.assertIn(("event v1/research.assessment.completed.v1", "ENUM_VALUE_REMOVED", "/properties/data/$ref/properties/strategy_id/$ref"), breaking(found))
        self.assertTrue(all(f[5] for f in found if f[0].startswith("event ")))

    def test_widening_a_safety_constant_breaks(self):
        def change(docs):
            del docs[RAC]["$defs"]["data"]["properties"]["eligible_for_entry"]["const"]
            del docs[REPORT]["properties"]["financial_state_write_access"]["const"]
        found = breaking(findings(change))
        self.assertIn(("event v1/research.assessment.completed.v1", "CONST_REMOVED", "/properties/data/$ref/properties/eligible_for_entry"), found)
        self.assertIn(("service sec-annual-v2.report", "CONST_REMOVED", "/properties/financial_state_write_access"), found)

    # Review 046 (1): constants compare as strict JSON kinds, never false == 0.
    def test_boolean_constants_never_equal_numbers(self):
        def change(docs):
            docs[RAC]["$defs"]["data"]["properties"]["eligible_for_entry"]["const"] = C.Number("0")
            docs["events/v1/risk.entries.paused.v1.schema.json"]["$defs"]["data"]["properties"]["monitoring_continues"]["const"] = C.Number("1")
        found = breaking(findings(change))
        self.assertIn(("event v1/research.assessment.completed.v1", "CONST_CHANGED", "/properties/data/$ref/properties/eligible_for_entry"), found)
        self.assertIn(("event v1/risk.entries.paused.v1", "CONST_CHANGED", "/properties/data/$ref/properties/monitoring_continues"), found)

    def test_value_comparison_is_strict_at_every_depth(self):
        self.assertNotEqual(compat.tag([False]), compat.tag([C.Number("0")]))
        self.assertNotEqual(compat.tag({"a": True}), compat.tag({"a": C.Number("1")}))
        self.assertNotEqual(compat.tag("1"), compat.tag(C.Number("1")))
        self.assertEqual(compat.tag(C.Number("-0")), compat.tag(C.Number("0")))
        self.assertEqual(compat.tag(C.Number("9007199254740991")), compat.tag(C.Number("9007199254740991")))

    def test_an_enum_value_changing_kind_breaks(self):
        def change(docs):
            enum = docs[COMMON]["$defs"]["lane"]["enum"]
            enum[0] = True
        kinds = {k for _, k, _ in breaking(findings(change))}
        self.assertEqual(kinds, {"ENUM_VALUE_ADDED", "ENUM_VALUE_REMOVED"})

    def test_the_same_integer_written_differently_is_not_a_difference(self):
        def change(docs):
            docs[COMMON]["$defs"]["count"]["minimum"] = C.Number("-0")
        self.assertEqual(findings(change), [])

    def test_opening_unknown_fields_breaks(self):
        def change(docs):
            docs["events/v1/envelope.schema.json"]["additionalProperties"] = True
            docs[RAC]["additionalProperties"] = True
        self.assertEqual(breaking(findings(change)), {
            ("event v1/envelope", "ADDITIONAL_PROPERTIES_OPENED", "/"),
            ("event v1/research.assessment.completed.v1", "ADDITIONAL_PROPERTIES_OPENED", "/")})

    def test_changing_a_reference_target_breaks(self):
        def change(docs):
            docs[RAC]["$defs"]["data"]["properties"]["strategy_id"]["$ref"] = "common.schema.json#/$defs/market"
        found = breaking(findings(change))
        self.assertIn(("event v1/research.assessment.completed.v1", "ENUM_VALUE_REMOVED", "/properties/data/$ref/properties/strategy_id/$ref"), found)
        self.assertIn(("event v1/research.assessment.completed.v1", "ENUM_VALUE_ADDED", "/properties/data/$ref/properties/strategy_id/$ref"), found)

    def test_a_reference_to_an_identical_definition_is_not_a_difference(self):
        def change(docs):
            docs[COMMON]["$defs"]["lane_copy"] = copy.deepcopy(docs[COMMON]["$defs"]["lane"])
            docs[RAC]["$defs"]["data"]["properties"]["strategy_id"]["$ref"] = "common.schema.json#/$defs/lane_copy"
        self.assertEqual(findings(change), [])

    def test_removing_a_schema_breaks(self):
        def change(docs):
            del docs["events/v1/goal.assessment.completed.v1.schema.json"]
        self.assertEqual(breaking(findings(change)), {("event v1/goal.assessment.completed.v1", "SURFACE_REMOVED", "/")})

    def test_a_new_event_is_reported_until_locked(self):
        def change(docs):
            docs["events/v2/goal.assessment.completed.v2.schema.json"] = {"type": "null"}
            docs["events/v1/stray.json"] = {"type": "null"}
        self.assertEqual([(f[0], f[1], f[5]) for f in findings(change)], [
            ("event v1/stray", "SURFACE_NOT_LOCKED", False), ("event v2/goal.assessment.completed.v2", "SURFACE_NOT_LOCKED", False)])

    # Review 046 (3): every surface compiles with the shared strict validator first.
    def test_an_unsupported_keyword_cannot_be_locked(self):
        def change(docs):
            schemas(docs)["Account"]["not"] = {"type": "null"}
        found = findings(change)
        self.assertEqual([(f[1], f[5]) for f in found], [("SCHEMA_DOES_NOT_COMPILE", True)])
        self.assertIn("unsupported keyword", found[0][4])

    def test_a_reference_cycle_is_a_stable_error(self):
        def change(docs):
            schemas(docs)["Account"]["properties"]["id"] = {"$ref": "#/components/schemas/Account"}
        found = findings(change)
        self.assertEqual([(f[1], f[5]) for f in found], [("SCHEMA_DOES_NOT_COMPILE", True)])
        self.assertIn("reference cycle", found[0][4])

    def test_a_malformed_lock_is_refused(self):
        for lock in [{"surfaces": {}}, {"dialect": C.DIALECT, "surfaces": []}, {"dialect": C.DIALECT, "surfaces": {"x": {"direction": "sideways", "shape": {}}}}]:
            with self.subTest(lock=lock):
                self.assertEqual([(f[1], f[5]) for f in compat.check(lock, TEXTS)], [("LOCK_INVALID", True)])

    # Review 047: an empty or unknown surface wrapper must not skip every comparison.
    def test_a_lock_surface_of_the_wrong_shape_is_refused(self):
        def edited(surface, shape):
            lock = copy.deepcopy(LOCK)
            lock["surfaces"][surface]["shape"] = shape
            return [(f[1], f[5]) for f in compat.check(lock, TEXTS)]
        self.assertEqual(edited("service sec-annual-v2.report", {}), [("LOCK_INVALID", True)])
        self.assertEqual(edited("event v1/envelope", {"schema": {"type": ["object"]}, "extra": 1}), [("LOCK_INVALID", True)])
        self.assertEqual(edited("api GET /api/v1/live security", {}), [("LOCK_INVALID", True)])
        self.assertEqual(edited("api GET /api/v1/live response 200", {"body": None}), [("LOCK_INVALID", True)])
        self.assertEqual(edited("api POST /api/v1/paper/entries request", {"required": "yes", "schema": {}}), [("LOCK_INVALID", True)])
        self.assertEqual(edited("event v1/envelope", {"schema": {"not": {}}}), [("LOCK_INVALID", True)])
        self.assertEqual(edited("event v1/envelope", {"schema": {"maximum": "5"}}), [("LOCK_INVALID", True)])

    def test_an_empty_schema_under_a_valid_wrapper_is_compared_not_refused(self):
        lock = copy.deepcopy(LOCK)
        lock["surfaces"]["service sec-annual-v2.report"]["shape"] = {"schema": {}}
        found = compat.check(lock, TEXTS)
        self.assertNotIn("LOCK_INVALID", {f[1] for f in found})
        self.assertIn(("service sec-annual-v2.report", "TYPE_ADDED", True), {(f[0], f[1], f[5]) for f in found})

    def test_api_requests_may_widen_but_not_narrow(self):
        def widen(docs):
            schemas(docs)["CreateAccount"]["properties"]["amount_minor"]["maximum"] = 20_000_000_000
        self.assertEqual([(f[0], f[1], f[5]) for f in findings(widen)], [("api POST /api/v1/paper/accounts request", "BOUND_LOOSENED", False)])

        def narrow(docs):
            schemas(docs)["Candidate"]["properties"]["quantity"]["maximum"] = 10
        self.assertEqual(breaking(findings(narrow)), {("api POST /api/v1/paper/recommendations request", "BOUND_TIGHTENED", "/$ref/properties/quantity")})

    # Review 046 (2): presence of bodies and parameters, security and response headers.
    def test_an_optional_parameter_becoming_required_breaks(self):
        def change(docs):
            op = docs[API]["paths"]["/api/v1/research/jobs/{id}/sources/{kind}"]["get"]
            next(p for p in op["parameters"] if p.get("name") == "sha256")["required"] = True
        self.assertEqual(breaking(findings(change)), {("api GET /api/v1/research/jobs/{id}/sources/{kind} parameter query sha256", "PRESENCE_REQUIRED", "/required")})

    def test_an_optional_body_becoming_required_breaks(self):
        def optional(docs):
            docs[API]["paths"]["/api/v1/paper/entries"]["post"]["requestBody"]["required"] = False
        relaxed = parsed()
        optional(relaxed)
        lock = compat.make_lock(texts(relaxed))
        self.assertEqual(breaking(compat.check(lock, TEXTS)), {("api POST /api/v1/paper/entries request", "PRESENCE_REQUIRED", "/required")})
        self.assertEqual([(f[1], f[5]) for f in findings(optional)], [("PRESENCE_OPTIONAL", False)])

    def test_a_new_required_parameter_on_an_existing_operation_breaks(self):
        def required(docs):
            docs[API]["paths"]["/api/v1/paper/accounts"]["post"]["parameters"].append(
                {"name": "X-Account-Scope", "in": "header", "required": True, "schema": {"type": "string", "maxLength": 20}})
        self.assertEqual(breaking(findings(required)), {("api POST /api/v1/paper/accounts parameter header X-Account-Scope", "REQUIRED_INPUT_ADDED", "/")})

        def optional(docs):
            docs[API]["paths"]["/api/v1/paper/accounts"]["post"]["parameters"].append(
                {"name": "X-Account-Scope", "in": "header", "required": False, "schema": {"type": "string", "maxLength": 20}})
        self.assertEqual([(f[1], f[5]) for f in findings(optional)], [("SURFACE_NOT_LOCKED", False)])

    def test_a_security_change_breaks(self):
        def change(docs):
            del docs[API]["paths"]["/api/v1/live"]["get"]["security"]
        self.assertEqual(breaking(findings(change)), {("api GET /api/v1/live security", "SECURITY_CHANGED", "/security")})

    def test_removing_a_response_header_breaks(self):
        def change(docs):
            del docs[API]["paths"]["/api/v1/live"]["get"]["responses"]["200"]["headers"]["traceparent"]
        self.assertEqual(breaking(findings(change)), {("api GET /api/v1/live response 200", "HEADER_REMOVED", "/headers/traceparent")})

    def test_api_responses_may_narrow_but_not_widen(self):
        def widen(docs):
            schemas(docs)["Recommendation"]["properties"]["status"]["enum"].append("EXECUTED_BY_BROKER")
        found = breaking(findings(widen))
        self.assertIn(("api POST /api/v1/paper/recommendations response 200", "ENUM_VALUE_ADDED", "/body/$ref/properties/status"), found)
        self.assertNotIn("api POST /api/v1/paper/recommendations request", {s for s, _, _ in found})

        def narrow(docs):
            schemas(docs)["Recommendation"]["properties"]["status"]["enum"].remove("EXPIRED")
        narrowed = findings(narrow)
        self.assertTrue(narrowed and not any(f[5] for f in narrowed))
        self.assertEqual({f[1] for f in narrowed}, {"ENUM_VALUE_REMOVED"})

    def test_removing_an_api_operation_breaks(self):
        def change(docs):
            del docs[API]["paths"]["/api/v1/paper/entries"]
        removed = {(s, k) for s, k, _ in breaking(findings(change))}
        for surface in ["security", "request", "response 200", "parameter header Idempotency-Key"]:
            self.assertIn(("api POST /api/v1/paper/entries " + surface, "SURFACE_REMOVED"), removed)


if __name__ == "__main__":
    unittest.main()

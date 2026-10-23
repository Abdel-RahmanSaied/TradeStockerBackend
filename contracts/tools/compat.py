"""Schema compatibility check against the reviewed baseline (contracts/compat/v1.lock.json).

Every contract surface (each document under events/, each research service
schema, and for each OpenAPI operation its security, request body, each
parameter and each response with its headers) is first compiled with the
shared strict validator (tradestocker_research.contracts: same dialect,
keywords, depth and size limits as Go), then reduced to its validation
structure: references followed, documentation dropped, numbers kept as exact
tokens. The current structure is compared with the locked one; every
difference gets a specific diagnosis and an effect: NARROWS (accepts less),
WIDENS (accepts more) or CHANGES (neither, or not decidable, such as a new
pattern).

Whether a difference breaks depends on who reads the payload:
  strict   (events, research service): any difference breaks; publish a new version.
  request  (API security, bodies and parameters): NARROWS or CHANGES breaks old clients,
           including a body or parameter becoming required, or a new required parameter
           on an operation that already exists.
  response (API responses and their headers): WIDENS or CHANGES breaks old clients.
A surface that does not compile, a removed surface and a lock of the wrong shape
always break. A new surface is reported until it is locked. Standard library only.
"""
import importlib.util
import pathlib

ROOT = pathlib.Path(__file__).resolve().parents[2]
_spec = importlib.util.spec_from_file_location("tradestocker_contracts", ROOT / "research/src/tradestocker_research/contracts.py")
assert _spec is not None and _spec.loader is not None
C = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(C)

ANNOTATIONS = {"title", "description", "examples", "$comment", "deprecated", "$schema", "$id", "$defs"}
VALIDATION = {"$ref", "allOf", "properties", "additionalProperties", "items", "type", "enum", "const", "required",
              "minLength", "maxLength", "pattern", "format", "minimum", "maximum", "minItems", "maxItems"}
BOUNDS_MIN = ("minimum", "minLength", "minItems")
BOUNDS_MAX = ("maximum", "maxLength", "maxItems")
STRICT, REQUEST, RESPONSE = "strict", "request", "response"
API = "openapi/local-api.v1.json"


class CompatError(Exception):
    """A document or the lock cannot be compared at all."""

    def __init__(self, kind, message):
        super().__init__("%s: %s" % (kind, message))
        self.kind = kind


def tag(value):
    """A strict, comparable form of a JSON value: false is never 0, 1 is never true, and
    integers compare by value (1 and 1 written alike); a non-integer number keeps its text."""
    if value is None:
        return ("null",)
    if value is True or value is False:
        return ("bool", value)
    if isinstance(value, C.Number):
        n = value.integer()
        return ("int", n) if n is not None else ("number", str(value))
    if isinstance(value, str):
        return ("string", value)
    if isinstance(value, list):
        return ("list",) + tuple(tag(v) for v in value)
    if isinstance(value, dict):
        return ("object",) + tuple(sorted((k, tag(v)) for k, v in value.items()))
    raise CompatError("UNSUPPORTED_VALUE", type(value).__name__)


def dumps(value):
    """JSON text of a parsed value, numbers written exactly as their tokens."""
    if value is None:
        return "null"
    if value is True:
        return "true"
    if value is False:
        return "false"
    if isinstance(value, C.Number):
        return str(value)
    if isinstance(value, int):
        return str(value)
    if isinstance(value, str):
        return C.canonical(value)
    if isinstance(value, list):
        return "[" + ",".join(dumps(v) for v in value) + "]"
    if isinstance(value, dict):
        return "{" + ",".join(C.canonical(k) + ":" + dumps(value[k]) for k in sorted(value)) + "}"
    raise CompatError("UNSUPPORTED_VALUE", type(value).__name__)


def _normalize(registry, doc, node, budget):
    """The validation structure of a compiled schema (bounded by the compiler's own limits)."""
    budget[0] -= 1
    if budget[0] < 0:
        raise CompatError("SCHEMA_TOO_LARGE", "normalized schema exceeds %d nodes" % C.MAX_SCHEMA_NODES)
    if isinstance(node, bool):
        return {"boolean": node}
    out = {}
    for key, value in node.items():
        if key in ANNOTATIONS or key.startswith("x-tradestocker-"):
            continue
        if key not in VALIDATION:
            raise CompatError("UNSUPPORTED_KEYWORD", "%s in %s" % (key, doc))
        if key == "$ref":
            target_doc, _, target = registry.resolve(doc, value)
            out["$ref"] = _normalize(registry, target_doc, target, budget)
        elif key == "properties":
            out[key] = {k: _normalize(registry, doc, v, budget) for k, v in value.items()}
        elif key in ("items", "additionalProperties") and not isinstance(value, bool):
            out[key] = _normalize(registry, doc, value, budget)
        elif key == "allOf":
            out[key] = [_normalize(registry, doc, v, budget) for v in value]
        elif key == "type":
            out[key] = sorted([value] if isinstance(value, str) else value)
        elif key == "required":
            out[key] = sorted(value)
        else:
            out[key] = value
    return out


def _escape(part):
    return part.replace("~", "~0").replace("/", "~1")


def surfaces(texts):
    """Compiles and normalizes every surface: name -> (direction, shape). Raises CompatError."""
    try:
        registry = C.Registry(texts)
    except (C.SchemaError, C.ParseError, ValueError) as error:
        raise CompatError("SCHEMA_DOES_NOT_COMPILE", str(error)) from None

    def schema(ref):
        try:
            registry.compile(ref)
        except C.SchemaError as error:
            raise CompatError("SCHEMA_DOES_NOT_COMPILE", str(error)) from None
        doc, _, node = registry.resolve("", ref)
        return _normalize(registry, doc, node, [C.MAX_SCHEMA_NODES])

    out = {}
    for name in sorted(registry.docs):
        if name.startswith("events/"):
            # Every document under events/ is a surface, so no file is ignored.
            out["event " + name[len("events/"):].removesuffix(".json").removesuffix(".schema")] = (STRICT, {"schema": schema(name)})
        elif name.startswith("research/") and name.endswith(".schema.json"):
            out["service " + name[len("research/"):-len(".schema.json")]] = (STRICT, {"schema": schema(name)})
    if API in registry.docs:
        doc = registry.docs[API]
        top_security = doc.get("security", [])
        for path, item in doc["paths"].items():
            for method, op in item.items():
                label = "api %s %s" % (method.upper(), path)
                base = "%s#/paths/%s/%s" % (API, _escape(path), method)
                out[label + " security"] = (REQUEST, {"security": op.get("security", top_security)})
                for i, p in enumerate(op.get("parameters", [])):
                    where = "%s/parameters/%d" % (base, i)
                    if "$ref" in p:
                        _, pointer, p = registry.resolve(API, p["$ref"])
                        where = API + "#" + pointer
                    out["%s parameter %s %s" % (label, p["in"], p["name"])] = (REQUEST, {
                        "required": p.get("required", False) is True, "schema": schema(where + "/schema")})
                body = op.get("requestBody")
                if body:
                    out[label + " request"] = (REQUEST, {"required": body.get("required", False) is True,
                                                         "schema": schema(base + "/requestBody/content/application~1json/schema")})
                for status, response in op["responses"].items():
                    where = "%s/responses/%s" % (base, status)
                    headers = {}
                    for header, h in response.get("headers", {}).items():
                        hwhere = "%s/headers/%s" % (where, _escape(header))
                        if "$ref" in h:
                            _, pointer, h = registry.resolve(API, h["$ref"])
                            hwhere = API + "#" + pointer
                        headers[header] = {"required": h.get("required", False) is True, "schema": schema(hwhere + "/schema")}
                    body_schema = schema(where + "/content/application~1json/schema") if "content" in response else None
                    out["%s response %s" % (label, status)] = (RESPONSE, {"body": body_schema, "headers": headers})
    return out


def diff(old, new, path=""):
    """Differences between two normalized schemas: [(kind, effect, path, detail)]."""
    out = []

    def add(kind, effect, detail=""):
        out.append((kind, effect, path or "/", detail))

    if "boolean" in old or "boolean" in new:
        if tag(old.get("boolean")) != tag(new.get("boolean")) or set(old) != set(new):
            add("BOOLEAN_SCHEMA_CHANGED", "CHANGES", "%s -> %s" % (dumps(old), dumps(new)))
        return out
    if old.get("type") != new.get("type"):
        before, after = set(old.get("type", [])), set(new.get("type", []))
        if "type" not in old:
            add("TYPE_ADDED", "NARROWS", str(sorted(after)))
        elif "type" not in new:
            add("TYPE_REMOVED", "WIDENS", str(sorted(before)))
        elif after < before:
            add("TYPE_NARROWED", "NARROWS", "%s -> %s" % (sorted(before), sorted(after)))
        elif after > before:
            add("TYPE_WIDENED", "WIDENS", "%s -> %s" % (sorted(before), sorted(after)))
        else:
            add("TYPE_CHANGED", "CHANGES", "%s -> %s" % (sorted(before), sorted(after)))
    if "enum" in old or "enum" in new:
        if "enum" not in old:
            add("ENUM_ADDED", "NARROWS")
        elif "enum" not in new:
            add("ENUM_REMOVED", "WIDENS")
        else:
            was = {tag(v): v for v in old["enum"]}
            now = {tag(v): v for v in new["enum"]}
            for t in sorted(set(was) - set(now)):
                add("ENUM_VALUE_REMOVED", "NARROWS", dumps(was[t]))
            for t in sorted(set(now) - set(was)):
                add("ENUM_VALUE_ADDED", "WIDENS", dumps(now[t]))
    if "const" in old or "const" in new:
        if "const" not in old:
            add("CONST_ADDED", "NARROWS", dumps(new["const"]))
        elif "const" not in new:
            add("CONST_REMOVED", "WIDENS", dumps(old["const"]))
        elif tag(old["const"]) != tag(new["const"]):
            add("CONST_CHANGED", "CHANGES", "%s -> %s" % (dumps(old["const"]), dumps(new["const"])))
    for key in ("pattern", "format"):
        if old.get(key) != new.get(key):
            kind = key.upper()
            if key not in old:
                add(kind + "_ADDED", "NARROWS", new[key])
            elif key not in new:
                add(kind + "_REMOVED", "WIDENS", old[key])
            else:
                add(kind + "_CHANGED", "CHANGES", "%s -> %s" % (old[key], new[key]))
    for key in BOUNDS_MIN + BOUNDS_MAX:
        a = old[key].integer() if key in old else None
        b = new[key].integer() if key in new else None
        if a == b:
            continue
        lower = key in BOUNDS_MIN
        if a is None or (b is not None and (b > a if lower else b < a)):
            add("BOUND_TIGHTENED", "NARROWS", "%s %s -> %s" % (key, a, b))
        else:
            add("BOUND_LOOSENED", "WIDENS", "%s %s -> %s" % (key, a, b))
    before, after = set(old.get("required", [])), set(new.get("required", []))
    for name in sorted(after - before):
        add("REQUIRED_ADDED", "NARROWS", name)
    for name in sorted(before - after):
        add("REQUIRED_REMOVED", "WIDENS", name)
    closed_old = old.get("additionalProperties") is False
    props_old, props_new = old.get("properties", {}), new.get("properties", {})
    for name in sorted(set(props_new) - set(props_old)):
        add("PROPERTY_ADDED", "WIDENS" if closed_old else "NARROWS", name)
    for name in sorted(set(props_old) - set(props_new)):
        add("PROPERTY_REMOVED", "NARROWS" if new.get("additionalProperties") is False else "WIDENS", name)
    for name in sorted(set(props_old) & set(props_new)):
        out.extend(diff(props_old[name], props_new[name], path + "/properties/" + name))
    a, b = old.get("additionalProperties"), new.get("additionalProperties")
    if isinstance(a, dict) and isinstance(b, dict):
        out.extend(diff(a, b, path + "/additionalProperties"))
    elif isinstance(a, dict) or isinstance(b, dict) or tag(a) != tag(b):
        if a is False:
            add("ADDITIONAL_PROPERTIES_OPENED", "WIDENS", "%s -> %s" % (dumps(a), dumps(b)))
        elif b is False:
            add("ADDITIONAL_PROPERTIES_CLOSED", "NARROWS", "%s -> %s" % (dumps(a), dumps(b)))
        else:
            add("ADDITIONAL_PROPERTIES_CHANGED", "CHANGES", "%s -> %s" % (dumps(a), dumps(b)))
    for key in ("items", "$ref"):
        a, b = old.get(key), new.get(key)
        if a is not None and b is not None:
            out.extend(diff(a, b, path + "/" + key))
        elif a is not None or b is not None:
            add(key.strip("$").upper() + "_" + ("ADDED" if b is not None else "REMOVED"), "CHANGES")
    a, b = old.get("allOf"), new.get("allOf")
    if a is not None and b is not None and len(a) == len(b):
        for i, (x, y) in enumerate(zip(a, b)):
            out.extend(diff(x, y, "%s/allOf/%d" % (path, i)))
    elif (a is None) != (b is None) or (a is not None and len(a) != len(b)):
        add("ALLOF_CHANGED", "CHANGES")
    return out


def diff_surface(old, new):
    """Differences between two surface shapes (a schema plus its transport rules)."""
    out = []
    if "security" in old or "security" in new:
        if tag(old.get("security")) != tag(new.get("security")):
            out.append(("SECURITY_CHANGED", "CHANGES", "/security", "%s -> %s" % (dumps(old.get("security")), dumps(new.get("security")))))
        return out
    if "required" in old and old.get("required") != new.get("required"):
        out.append(("PRESENCE_REQUIRED" if new.get("required") else "PRESENCE_OPTIONAL",
                    "NARROWS" if new.get("required") else "WIDENS", "/required", ""))
    if "schema" in old:
        out.extend(diff(old["schema"], new["schema"], ""))
    if "body" in old:
        if (old["body"] is None) != (new["body"] is None):
            out.append(("BODY_CHANGED", "CHANGES", "/body", ""))
        elif old["body"] is not None:
            out.extend(diff(old["body"], new["body"], "/body"))
        for name in sorted(set(new["headers"]) - set(old["headers"])):
            out.append(("HEADER_ADDED", "NARROWS", "/headers/" + name, ""))
        for name in sorted(set(old["headers"]) - set(new["headers"])):
            out.append(("HEADER_REMOVED", "WIDENS", "/headers/" + name, ""))
        for name in sorted(set(old["headers"]) & set(new["headers"])):
            o, n = old["headers"][name], new["headers"][name]
            if o["required"] != n["required"]:
                out.append(("HEADER_PRESENCE_CHANGED", "NARROWS" if n["required"] else "WIDENS", "/headers/" + name, ""))
            out.extend(diff(o["schema"], n["schema"], "/headers/" + name))
    return out


def breaks(direction, effect):
    if direction == STRICT:
        return True
    if direction == REQUEST:
        return effect in ("NARROWS", "CHANGES")
    return effect in ("WIDENS", "CHANGES")


def _valid_schema(node):
    """A normalized schema as _normalize writes it ({} is a legitimate schema)."""
    if not isinstance(node, dict):
        return False
    if "boolean" in node:
        return set(node) == {"boolean"} and isinstance(node["boolean"], bool)
    for key, value in node.items():
        if key not in VALIDATION:
            return False
        if key in ("$ref", "items") and not _valid_schema(value):
            return False
        if key == "additionalProperties" and not (isinstance(value, bool) or _valid_schema(value)):
            return False
        if key == "properties" and not (isinstance(value, dict) and all(_valid_schema(v) for v in value.values())):
            return False
        if key == "allOf" and not (isinstance(value, list) and value and all(_valid_schema(v) for v in value)):
            return False
        if key in ("type", "required", "enum") and not isinstance(value, list):
            return False
        if key in BOUNDS_MIN + BOUNDS_MAX and not (isinstance(value, C.Number) and value.integer() is not None):
            return False
        if key in ("pattern", "format") and not isinstance(value, str):
            return False
    return True


def _valid_shape(direction, shape):
    """The exact wrapper of each surface kind, as surfaces() writes it."""
    if not isinstance(shape, dict):
        return False
    keys = set(shape)
    if direction == STRICT:
        return keys == {"schema"} and _valid_schema(shape["schema"])
    if direction == REQUEST:
        if keys == {"security"}:
            return isinstance(shape["security"], list)
        return keys == {"required", "schema"} and isinstance(shape["required"], bool) and _valid_schema(shape["schema"])
    if keys != {"body", "headers"} or not isinstance(shape["headers"], dict):
        return False
    if shape["body"] is not None and not _valid_schema(shape["body"]):
        return False
    return all(isinstance(h, dict) and set(h) == {"required", "schema"} and isinstance(h["required"], bool) and _valid_schema(h["schema"])
               for h in shape["headers"].values())


def check_lock_shape(lock):
    if not isinstance(lock, dict) or lock.get("dialect") != C.DIALECT or not isinstance(lock.get("surfaces"), dict):
        raise CompatError("LOCK_INVALID", "the lock is not a %s baseline" % C.DIALECT)
    for name, entry in lock["surfaces"].items():
        if not isinstance(entry, dict) or set(entry) != {"direction", "shape"} or entry["direction"] not in (STRICT, REQUEST, RESPONSE):
            raise CompatError("LOCK_INVALID", "surface %s has no direction and shape" % name)
        if not _valid_shape(entry["direction"], entry["shape"]):
            raise CompatError("LOCK_INVALID", "surface %s has a %s shape the checker does not write" % (name, entry["direction"]))


def check(lock, texts):
    """Findings of the current documents against the lock: [(surface, kind, effect, path, detail, breaking)]."""
    try:
        check_lock_shape(lock)
        current = surfaces(texts)
    except CompatError as error:
        return [("(contracts)", error.kind, "CHANGES", "/", str(error), True)]
    findings = []
    locked = lock["surfaces"]
    for name, entry in sorted(locked.items()):
        if name not in current:
            findings.append((name, "SURFACE_REMOVED", "CHANGES", "/", "", True))
            continue
        direction, shape = current[name]
        if direction != entry["direction"]:
            findings.append((name, "DIRECTION_CHANGED", "CHANGES", "/", "%s -> %s" % (entry["direction"], direction), True))
        for kind, effect, path, detail in diff_surface(entry["shape"], shape):
            findings.append((name, kind, effect, path, detail, breaks(entry["direction"], effect)))
    operations = {" ".join(n.split(" ")[:3]) for n in locked if n.startswith("api ")}
    for name in sorted(set(current) - set(locked)):
        direction, shape = current[name]
        operation = " ".join(name.split(" ")[:3])
        if name.startswith("api ") and operation in operations and direction == REQUEST and shape.get("required"):
            # Old clients of an existing operation never send it.
            findings.append((name, "REQUIRED_INPUT_ADDED", "NARROWS", "/", "", True))
        else:
            findings.append((name, "SURFACE_NOT_LOCKED", "WIDENS", "/", "", False))
    return findings


def make_lock(texts):
    return {"dialect": C.DIALECT,
            "description": "Reviewed contract baseline. Change it only with contracts/tools/generate_contracts.py --lock, which adds new surfaces and accepts non-breaking differences but never a breaking one.",
            "surfaces": {name: {"direction": d, "shape": s} for name, (d, s) in surfaces(texts).items()}}


def load_documents(root):
    """The contract documents as text, keyed by their path inside contracts/."""
    docs = {}
    for pattern in ("events/*/*.json", "research/*.json", "openapi/*.json"):
        for path in sorted(root.glob(pattern)):
            docs[path.relative_to(root).as_posix()] = path.read_text(encoding="utf-8")
    return docs


def load_lock(path):
    return C.parse(path.read_bytes())

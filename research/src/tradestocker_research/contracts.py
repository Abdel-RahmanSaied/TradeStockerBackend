"""Strict reading, validation and canonical writing of TradeStocker contract payloads.

The same rules as Go's internal/contracts, so both languages give the same
verdict (and the same first violation: keyword and JSON Pointer) for the same
bytes. Standard library only; schemas come from the embedded documents in
contract_documents.py, never from files or the network.

Run as `python -I -m tradestocker_research.contracts`: one JSON request per
line on stdin ({"ref": ..., "payload_b64": ...} or {"event": true, ...}), one
verdict per line on stdout. Go's tests use it to compare verdicts.
"""
import base64
import json
import re
import sys

DIALECT = "tradestocker-schema-subset/1"
# Input and work limits, the same as Go's.
MAX_BYTES = 48 << 20
MAX_NODES = 4_000_000
MAX_NUMBER_LENGTH = 128
MAX_DEPTH = 64
MAX_SCHEMA_NODES = 100_000
MAX_SCHEMA_DEPTH = 64
MAX_VALIDATION_STEPS = 20_000_000
MAX_PATTERN_LENGTH = 1000
MAX_PATTERN_STATES = 10000
_MAX_REPEAT = 1000

KEYWORDS = {
    "$schema", "$id", "$ref", "$defs", "$comment", "title", "description", "examples", "deprecated",
    "allOf", "properties", "additionalProperties", "items", "type", "enum", "const", "required",
    "minLength", "maxLength", "pattern", "format", "minimum", "maximum", "minItems", "maxItems",
}


class ParseError(ValueError):
    """The bytes are not acceptable JSON."""


_INTEGER_TEXT = re.compile(r"-?(0|[1-9][0-9]*)")


class Number(str):
    """A JSON number exactly as written."""

    def integer(self):
        """The value when the text is exactly a JSON integer, else None."""
        if len(self) > MAX_NUMBER_LENGTH or not _INTEGER_TEXT.fullmatch(self):
            return None
        return int(self)


def _number(text):
    if len(text) > MAX_NUMBER_LENGTH:
        raise ParseError("number longer than MaxNumberLength")
    return Number(text)


class Violation(Exception):
    def __init__(self, keyword, path, message, schema=""):
        super().__init__("%s at %r: %s" % (keyword, path, message))
        self.keyword, self.path, self.message, self.schema = keyword, path, message, schema


def _reject_constant(name):
    raise ParseError("non-standard constant " + name)


def _pairs(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise ParseError("duplicate object key")
        out[key] = value
    return out


def _walk(value, depth, count):
    """Depth, node count and surrogate checks after decoding (Go checks them while parsing)."""
    if depth > MAX_DEPTH:
        raise ParseError("nesting deeper than 64 levels")
    count[0] += 1
    if count[0] > MAX_NODES:
        raise ParseError("more than MaxNodes values")
    if isinstance(value, str) and not isinstance(value, Number):
        _check_text(value)
    elif isinstance(value, dict):
        for key, item in value.items():
            _check_text(key)
            _walk(item, depth + 1, count)
    elif isinstance(value, list):
        for item in value:
            _walk(item, depth + 1, count)


def _check_text(text):
    for ch in text:
        if 0xD800 <= ord(ch) <= 0xDFFF:
            raise ParseError("unpaired surrogate")


def parse(raw):
    """Decodes one JSON value: dict, list, str, Number, bool or None."""
    if len(raw) > MAX_BYTES:
        raise ParseError("input larger than MaxBytes")
    try:
        text = raw.decode("utf-8") if isinstance(raw, (bytes, bytearray)) else raw
        value = json.loads(text, parse_int=_number, parse_float=_number, parse_constant=_reject_constant,
                           object_pairs_hook=_pairs, strict=True)
    except ParseError:
        raise
    except (ValueError, RecursionError) as error:
        raise ParseError(type(error).__name__) from None
    _walk(value, 0, [0])
    return value


_ESCAPES = {'"': '\\"', "\\": "\\\\", "\b": "\\b", "\t": "\\t", "\n": "\\n", "\f": "\\f", "\r": "\\r"}


def _canonical_string(text):
    out = ['"']
    for ch in text:
        if ch in _ESCAPES:
            out.append(_ESCAPES[ch])
        elif ord(ch) < 0x20:
            out.append("\\u%04x" % ord(ch))
        else:
            out.append(ch)
    out.append('"')
    return "".join(out)


def canonical(value, depth=0):
    """The canonical form both languages share; see Go's contracts.Canonical.

    Accepts only values as parse returns them (plus int, tuple from typed events).
    """
    if depth > MAX_DEPTH:
        raise ValueError("value nested deeper than 64 levels")
    if value is None:
        return "null"
    if value is True:
        return "true"
    if value is False:
        return "false"
    if isinstance(value, Number):
        n = value.integer()
        if n is None:
            raise ValueError("non-integer number has no canonical form")
        return str(n)
    if isinstance(value, int):
        return str(value)
    if isinstance(value, str):
        _check_text(value)
        return _canonical_string(value)
    if isinstance(value, (list, tuple)):
        return "[" + ",".join(canonical(v, depth + 1) for v in value) + "]"
    if isinstance(value, dict):
        for key in value:
            if not isinstance(key, str):
                raise ValueError("object keys must be strings")
            _check_text(key)
        return "{" + ",".join(_canonical_string(k) + ":" + canonical(value[k], depth + 1) for k in sorted(value)) + "}"
    raise ValueError("unsupported value %s" % type(value).__name__)


def _escape_pointer(text):
    return text.replace("~", "~0").replace("/", "~1")


def _pointer_get(root, pointer):
    if pointer == "":
        return root
    if not pointer.startswith("/"):
        raise LookupError("pointer must start with /")
    node = root
    for part in pointer[1:].split("/"):
        part = part.replace("~1", "/").replace("~0", "~")
        if isinstance(node, dict) and part in node:
            node = node[part]
        elif isinstance(node, list) and part.isdigit() and int(part) < len(node) and part == str(int(part)):
            node = node[int(part)]
        else:
            raise LookupError("no member %r" % part)
    return node


def _clean(path):
    parts: list = []
    for part in path.split("/"):
        if part in ("", "."):
            continue
        if part == "..":
            if not parts or parts[-1] == "..":
                parts.append("..")
            else:
                parts.pop()
        else:
            parts.append(part)
    return "/".join(parts) or "."


def _dirname(path):
    return path.rsplit("/", 1)[0] if "/" in path else ""


def _is_type(name, value):
    if name == "object":
        return isinstance(value, dict)
    if name == "array":
        return isinstance(value, list)
    if name == "string":
        return isinstance(value, str) and not isinstance(value, Number)
    if name == "integer":
        return isinstance(value, Number) and value.integer() is not None
    if name == "boolean":
        return isinstance(value, bool)
    if name == "null":
        return value is None
    return False


def _equal(a, b):
    if isinstance(a, Number) or isinstance(b, Number):
        if not (isinstance(a, Number) and isinstance(b, Number)):
            return False
        x, y = a.integer(), b.integer()
        return x == y if x is not None and y is not None else str(a) == str(b)
    if isinstance(a, bool) or isinstance(b, bool) or a is None or b is None:
        return a is b
    if isinstance(a, str) and isinstance(b, str):
        return a == b
    if isinstance(a, dict) and isinstance(b, dict):
        return a.keys() == b.keys() and all(_equal(a[k], b[k]) for k in a)
    if isinstance(a, list) and isinstance(b, list):
        return len(a) == len(b) and all(_equal(x, y) for x, y in zip(a, b))
    return False


_DATE_TIME = re.compile(r"([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})([.][0-9]{1,9})?(Z|[+]00:00)")
_DATE = re.compile(r"([0-9]{4})-([0-9]{2})-([0-9]{2})")


def _real_day(year, month, day):
    if year < 1 or not 1 <= month <= 12 or day < 1:
        return False
    days = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1]
    if month == 2 and year % 4 == 0 and (year % 100 != 0 or year % 400 == 0):
        days = 29
    return day <= days


def check_format(fmt, text):
    """Asserts a format (never an annotation); see Go's checkFormat."""
    if fmt == "date-time":
        m = _DATE_TIME.fullmatch(text)
        if not m:
            return "not a UTC RFC 3339 date-time (Z or +00:00)"
        if not _real_day(int(m.group(1)), int(m.group(2)), int(m.group(3))):
            return "not a real calendar date"
        if int(m.group(4)) > 23 or int(m.group(5)) > 59 or int(m.group(6)) > 59:
            return "not a real time of day"
        return ""
    if fmt == "date":
        m = _DATE.fullmatch(text)
        if not m or not _real_day(int(m.group(1)), int(m.group(2)), int(m.group(3))):
            return "not a real calendar date (YYYY-MM-DD)"
        return ""
    return "unsupported format " + str(fmt)


_TYPE_NAMES = {"object", "array", "string", "integer", "boolean", "null"}


class SchemaError(ValueError):
    """A schema does not compile; it never validates anything."""


# The dialect's own regular expressions, the same grammar and automaton as Go's
# pattern.go (see there for the grammar). Python's re is not used for schema
# patterns: its syntax and backtracking differ from Go's.
_METACHARS = "\\^$.|?*+()[]{}"
_LIT, _CONCAT, _ALT, _REPEAT = range(4)
_CACHE_BUDGET = 200_000


class _PatternParser:
    def __init__(self, text):
        self.s, self.i, self.depth = text, 1, 0

    def fail(self, message):
        return SchemaError("pattern %r at %d: %s" % (self.s, self.i, message))

    def alt(self, end, top):
        branches = []
        while True:
            branches.append(self.branch(end))
            if self.i < end and self.s[self.i] == "|":
                if top:
                    raise self.fail("top-level | is refused: write ^(a|b)$")
                self.i += 1
                continue
            break
        return branches[0] if len(branches) == 1 else (_ALT, branches)

    def branch(self, end):
        children = []
        while self.i < end and self.s[self.i] not in "|)":
            children.append(self.quantifier(end, self.atom(end)))
        return (_CONCAT, children)

    def atom(self, end):
        c = self.s[self.i]
        if c == "(":
            self.depth += 1
            if self.depth > 32:
                raise self.fail("groups nested deeper than 32")
            self.i += 1
            if self.i < end and self.s[self.i] == "?":
                raise self.fail("(? groups, flags and lookaround are not supported")
            if self.i < end and self.s[self.i] == ")":
                raise self.fail("empty group")
            node = self.alt(end, False)
            if self.i >= end or self.s[self.i] != ")":
                raise self.fail("missing )")
            self.i += 1
            self.depth -= 1
            return node
        if c == "[":
            return self.charclass(end)
        if c == "\\":
            b = self.escape(end, _METACHARS + "-/")
            return (_LIT, {ord(b)})
        if c in "^$.?*+)]{}":
            raise self.fail("%r must be escaped here" % c)
        self.i += 1
        return (_LIT, {ord(c)})

    def escape(self, end, allowed):
        if self.i + 1 >= end:
            raise self.fail("pattern ends inside an escape")
        e = self.s[self.i + 1]
        if e not in allowed:
            raise self.fail("escape \\%s is not supported" % e)
        self.i += 2
        return e

    def charclass(self, end):
        self.i += 1
        chars: set = set()
        if self.i < end and self.s[self.i] == "^":
            raise self.fail("negated classes are not supported")
        items = 0
        while True:
            if self.i >= end:
                raise self.fail("missing ]")
            if self.s[self.i] == "]":
                break
            lo = hi = self.class_char(end)
            if self.i + 1 < end and self.s[self.i] == "-" and self.s[self.i + 1] != "]":
                self.i += 1
                hi = self.class_char(end)
                if hi < lo:
                    raise self.fail("range %s-%s is reversed" % (lo, hi))
            chars.update(range(ord(lo), ord(hi) + 1))
            items += 1
        if items == 0:
            raise self.fail("empty class")
        self.i += 1
        return (_LIT, chars)

    def class_char(self, end):
        c = self.s[self.i]
        if c == "\\":
            return self.escape(end, "\\]-[^")
        if c in "[^":
            raise self.fail("%r must be escaped inside a class" % c)
        self.i += 1
        return c

    def quantifier(self, end, atom):
        if self.i >= end:
            return atom
        c = self.s[self.i]
        if c == "?":
            low, high = 0, 1
            self.i += 1
        elif c == "*":
            low, high = 0, -1
            self.i += 1
        elif c == "+":
            low, high = 1, -1
            self.i += 1
        elif c == "{":
            close = self.s.find("}", self.i + 1, end)
            if close < 0:
                raise self.fail("missing }")
            counts = _split_count(self.s[self.i + 1:close])
            if counts is None:
                raise self.fail("{%s} is not {m} or {m,n} with m <= n <= %d" % (self.s[self.i + 1:close], _MAX_REPEAT))
            low, high = counts
            self.i = close + 1
        else:
            return atom
        if self.i < end and self.s[self.i] in "?*+{":
            raise self.fail("a quantifier cannot follow a quantifier")
        return (_REPEAT, atom, low, high)


def _split_count(text):
    def digits(t):
        if not t or len(t) > 4 or (len(t) > 1 and t[0] == "0") or not all("0" <= c <= "9" for c in t):
            return None
        n = int(t)
        return n if n <= _MAX_REPEAT else None
    low_text, comma, high_text = text.partition(",")
    low = digits(low_text)
    if low is None:
        return None
    if not comma:
        return low, low
    high = digits(high_text)
    return (low, high) if high is not None and low <= high else None


class _Builder:
    """Thompson automaton: states are [consumes(set or None), out, out1]; accept has neither."""

    def __init__(self):
        self.states: list = []

    def add(self, chars, out, out1):
        if len(self.states) >= MAX_PATTERN_STATES:
            raise SchemaError("pattern needs more than %d automaton states" % MAX_PATTERN_STATES)
        self.states.append([chars, out, out1])
        return len(self.states) - 1

    def patch(self, exits, to):
        for state, slot in exits:
            self.states[state][slot] = to

    def build(self, node):
        kind = node[0]
        if kind == _LIT:
            i = self.add(frozenset(node[1]), -1, -1)
            return i, [(i, 1)]
        if kind == _CONCAT:
            if not node[1]:
                i = self.add(None, -1, -2)
                return i, [(i, 1)]
            start, exits = self.build(node[1][0])
            for child in node[1][1:]:
                nxt, nexits = self.build(child)
                self.patch(exits, nxt)
                exits = nexits
            return start, exits
        if kind == _ALT:
            start, exits = None, []
            for child in node[1]:
                s, e = self.build(child)
                exits.extend(e)
                start = s if start is None else self.add(None, start, s)
            return start, exits
        return self.repeat(node[1], node[2], node[3])

    def repeat(self, child, low, high):
        parts = []
        for _ in range(low):
            parts.append(self.build(child))
        if high == -1:
            s, e = self.build(child)
            split = self.add(None, s, -1)
            self.patch(e, split)
            parts.append((split, [(split, 2)]))
        else:
            for _ in range(low, high):
                s, e = self.build(child)
                split = self.add(None, s, -1)
                parts.append((split, e + [(split, 2)]))
        if not parts:
            i = self.add(None, -1, -2)
            return i, [(i, 1)]
        for prev, nxt in zip(parts, parts[1:]):
            self.patch(prev[1], nxt[0])
        return parts[0][0], parts[-1][1]


class _Pattern:
    def __init__(self, text):
        if len(text) > MAX_PATTERN_LENGTH:
            raise SchemaError("pattern longer than %d characters" % MAX_PATTERN_LENGTH)
        if any(not 0x20 <= ord(c) <= 0x7E for c in text):
            raise SchemaError("pattern %r must be printable ASCII" % text)
        if len(text) < 2 or text[0] != "^" or text[-1] != "$":
            raise SchemaError("pattern %r must be ^body$" % text)
        parser = _PatternParser(text)
        tree = parser.alt(len(text) - 1, True)
        if parser.i != len(text) - 1:
            raise parser.fail("unexpected %r" % text[parser.i])
        builder = _Builder()
        start, exits = builder.build(tree)
        accept = builder.add(None, -1, -1)
        builder.patch(exits, accept)
        self.states, self.start = builder.states, start
        self._closures: dict = {}
        self._steps: dict = {}
        self._cached = 0  # stored elements, at most _CACHE_BUDGET

    def _eps(self, i):
        """The consuming states reachable from i without a character, and whether accept is."""
        cached = self._closures.get(i)
        if cached is not None:
            return cached
        found, accept, seen, stack = [], False, set(), [i]
        while stack:
            j = stack.pop()
            if j < 0 or j in seen:
                continue
            seen.add(j)
            chars, a, b = self.states[j]
            if chars is not None:
                found.append(j)
            elif a == -1 and b == -1:
                accept = True
            else:
                stack.append(a)
                if b >= 0:
                    stack.append(b)
        result = (tuple(found), accept)
        if self._cached + len(found) + 1 <= _CACHE_BUDGET:
            self._closures[i] = result
            self._cached += len(found) + 1
        return result

    def matches(self, text):
        """Whether the whole string matches; linear in its length.

        Closures and steps are cached up to _CACHE_BUDGET stored state numbers per
        pattern (then simply recomputed), which changes only the speed.
        """
        states, accept = self._eps(self.start)
        current = frozenset(states)
        for ch in text:
            code = ord(ch)
            if code >= 128:
                return False  # every class and literal is ASCII
            key = (current, code)
            step = self._steps.get(key)
            if step is None:
                following: set = set()
                following_accept = False
                for i in current:
                    if code in self.states[i][0]:
                        found, a = self._eps(self.states[i][1])
                        following.update(found)
                        following_accept = following_accept or a
                step = (frozenset(following), following_accept)
                if self._cached + len(current) + len(following) + 1 <= _CACHE_BUDGET:
                    self._steps[key] = step
                    self._cached += len(current) + len(following) + 1
            current, accept = step
            if not current and not accept:
                return False
        return accept


def _valid_type(value):
    if isinstance(value, str) and not isinstance(value, Number):
        return value in _TYPE_NAMES
    if isinstance(value, list) and value:
        names = [v for v in value if isinstance(v, str) and not isinstance(v, Number)]
        return len(names) == len(value) and len(set(names)) == len(names) and all(n in _TYPE_NAMES for n in names)
    return False


def _int_keyword(value):
    return value.integer() if isinstance(value, Number) else None


class Registry:
    """A fixed set of contract documents; $ref reaches only these.

    Documents whose root declares $schema are compiled when loaded; validate
    compiles its schema first. See Go's Registry.Compile for the rules.
    """

    def __init__(self, documents):
        self.docs = {}
        self._patterns = {}
        self._state = {}
        self._size = {}
        self._height = {}
        self._failures = {}
        for name, text in documents.items():
            if name != _clean(name) or name.startswith("/") or name.startswith("../") or name == "..":
                raise ValueError("document name %r must be a clean relative path" % name)
            self.docs[name] = parse(text.encode("utf-8") if isinstance(text, str) else text)
        for name in sorted(self.docs):
            root = self.docs[name]
            if isinstance(root, dict) and "$schema" in root:
                self.compile(name)

    def resolve(self, base, ref):
        file, _, pointer = ref.partition("#")
        if ":" in file:
            raise LookupError("reference %r is not a local contract document" % ref)
        doc = _clean(_dirname(base) + "/" + file) if file else base
        if doc not in self.docs:
            raise LookupError("reference %r names no loaded contract document" % ref)
        try:
            return doc, pointer, _pointer_get(self.docs[doc], pointer)
        except LookupError as error:
            raise LookupError("reference %r: %s" % (ref, error)) from None

    def compile(self, ref):
        """Raises SchemaError unless the schema at ref and all it reaches compile."""
        if ref in self._failures:
            raise self._failures[ref]
        try:
            try:
                doc, pointer, node = self.resolve("", ref)
            except LookupError as error:
                raise SchemaError(str(error)) from None
            self._compile_node(doc, pointer, node, 1)
        except SchemaError as error:
            self._failures[ref] = error
            for loc in [loc for loc, st in self._state.items() if st == 1]:
                del self._state[loc]
            raise

    def _compile_node(self, doc, pointer, node, depth):
        """Returns (expanded size, height); see Go's compileNode."""
        loc = doc + "#" + pointer
        deep = SchemaError("%s: schemas nest deeper than %d (subschemas and references)" % (loc, MAX_SCHEMA_DEPTH))
        if depth > MAX_SCHEMA_DEPTH:
            raise deep
        state = self._state.get(loc)
        if state == 1:
            raise SchemaError("%s: reference cycle (recursive schemas are not supported)" % loc)
        if state == 2:
            if depth + self._height[loc] - 1 > MAX_SCHEMA_DEPTH:
                raise deep
            return self._size[loc], self._height[loc]
        self._state[loc] = 1
        size, height = self._compile_shape(doc, pointer, node, depth)
        if size > MAX_SCHEMA_NODES:
            raise SchemaError("%s: schema expands to more than %d nodes" % (loc, MAX_SCHEMA_NODES))
        self._state[loc], self._size[loc], self._height[loc] = 2, size, height
        return size, height

    def _compile_shape(self, doc, pointer, node, depth):
        loc = doc + "#" + pointer

        def bad(message):
            return SchemaError("%s: %s" % (loc, message))

        if isinstance(node, bool):
            return 1, 1
        if not isinstance(node, dict):
            raise bad("a schema must be an object or a boolean")
        size, below = 1, 0

        def child(sub_pointer, sub):
            nonlocal size, below
            n, h = self._compile_node(doc, sub_pointer, sub, depth + 1)
            size += n
            below = max(below, h)
        for key in sorted(node):
            value = node[key]
            text = isinstance(value, str) and not isinstance(value, Number)
            if key.startswith("x-tradestocker-"):
                continue
            if key not in KEYWORDS:
                raise bad("unsupported keyword %r" % key)
            if key == "$schema":
                if pointer != "" or value != "https://json-schema.org/draft/2020-12/schema":
                    raise bad("$schema must be JSON Schema 2020-12, at a document root only")
            elif key == "$id":
                if not text or pointer != "":
                    raise bad("$id must be a string, at a document root only")
            elif key in ("$comment", "title", "description"):
                if not text:
                    raise bad("%s must be a string" % key)
            elif key == "examples":
                if not isinstance(value, list):
                    raise bad("examples must be a list")
            elif key == "deprecated":
                if not isinstance(value, bool):
                    raise bad("deprecated must be a boolean")
            elif key == "$ref":
                if not text:
                    raise bad("$ref must be a string")
                try:
                    tdoc, tpointer, target = self.resolve(doc, value)
                except LookupError as error:
                    raise bad(str(error)) from None
                n, h = self._compile_node(tdoc, tpointer, target, depth + 1)
                size += n
                below = max(below, h)
            elif key in ("$defs", "properties"):
                if not isinstance(value, dict):
                    raise bad("%s must be an object of schemas" % key)
                for name in sorted(value):
                    child(pointer + "/" + key + "/" + _escape_pointer(name), value[name])
            elif key == "allOf":
                if not isinstance(value, list) or not value:
                    raise bad("allOf must be a non-empty list of schemas")
                for i, sub in enumerate(value):
                    child("%s/allOf/%d" % (pointer, i), sub)
            elif key in ("additionalProperties", "items"):
                child(pointer + "/" + key, value)
            elif key == "type":
                if not _valid_type(value):
                    raise bad('type must be one of object, array, string, integer, boolean, null, or a list of distinct names ("number" is not supported)')
            elif key == "enum":
                if not isinstance(value, list) or not value:
                    raise bad("enum must be a non-empty list")
            elif key == "required":
                names = value if isinstance(value, list) else None
                if names is None or any(not isinstance(n, str) or isinstance(n, Number) for n in names) or len(set(names)) != len(names):
                    raise bad("required must be a list of distinct names")
            elif key in ("minLength", "maxLength", "minItems", "maxItems"):
                n = _int_keyword(value)
                if n is None or n < 0 or n > 2**63 - 1:
                    raise bad("%s must be a non-negative integer" % key)
            elif key in ("minimum", "maximum"):
                if _int_keyword(value) is None:
                    raise bad("%s must be an integer" % key)
            elif key == "pattern":
                if not text:
                    raise bad("pattern must be a string")
                try:
                    self._pattern(value)
                except SchemaError as error:
                    raise bad(str(error)) from None
            elif key == "format":
                if value not in ("date-time", "date"):
                    raise bad("format must be date-time or date")
        return size, below + 1

    def validate(self, ref, instance):
        """Returns the first Violation, or None. A schema that does not compile is a "schema" violation."""
        try:
            self.compile(ref)
        except SchemaError as error:
            return Violation("schema", "", str(error), ref)
        doc, pointer, node = self.resolve("", ref)
        try:
            self._check(doc, doc + "#" + pointer, node, instance, "", [0])
        except Violation as violation:
            return violation
        return None

    def _pattern(self, pattern):
        if pattern not in self._patterns:
            self._patterns[pattern] = _Pattern(pattern)
        return self._patterns[pattern]

    def _check(self, doc, where, schema, inst, at, steps):
        steps[0] += 1
        if steps[0] > MAX_VALIDATION_STEPS:
            raise Violation("budget", at, "validation needs more than %d steps" % MAX_VALIDATION_STEPS, where)
        if isinstance(schema, bool):
            if not schema:
                raise Violation("false", at, "no value is allowed here", where)
            return
        if "$ref" in schema:
            tdoc, tpointer, node = self.resolve(doc, schema["$ref"])
            self._check(tdoc, tdoc + "#" + tpointer, node, inst, at, steps)
        if "allOf" in schema:
            for i, sub in enumerate(schema["allOf"]):
                self._check(doc, "%s/allOf/%d" % (where, i), sub, inst, at, steps)
        if "type" in schema:
            t = schema["type"]
            names = [t] if isinstance(t, str) else t
            if not any(_is_type(n, inst) for n in names):
                raise Violation("type", at, "expected %s" % " or ".join(str(n) for n in names), where)
        if "const" in schema and not _equal(schema["const"], inst):
            raise Violation("const", at, "value is not the required constant", where)
        if "enum" in schema and not any(_equal(o, inst) for o in schema["enum"]):
            raise Violation("enum", at, "value is not one of the allowed values", where)
        if isinstance(inst, str) and not isinstance(inst, Number):
            self._check_string(schema, inst, at, where)
        elif isinstance(inst, Number):
            n = inst.integer()
            if n is not None:
                low, high = _bound(schema, "minimum"), _bound(schema, "maximum")
                if low is not None and n < low:
                    raise Violation("minimum", at, "less than %d" % low, where)
                if high is not None and n > high:
                    raise Violation("maximum", at, "greater than %d" % high, where)
        elif isinstance(inst, dict):
            self._check_object(doc, where, schema, inst, at, steps)
        elif isinstance(inst, list):
            low, high = _bound(schema, "minItems"), _bound(schema, "maxItems")
            if low is not None and len(inst) < low:
                raise Violation("minItems", at, "fewer than %d items" % low, where)
            if high is not None and len(inst) > high:
                raise Violation("maxItems", at, "more than %d items" % high, where)
            if "items" in schema:
                for i, item in enumerate(inst):
                    self._check(doc, where + "/items", schema["items"], item, "%s/%d" % (at, i), steps)

    def _check_string(self, schema, inst, at, where):
        low, high = _bound(schema, "minLength"), _bound(schema, "maxLength")
        if low is not None and len(inst) < low:
            raise Violation("minLength", at, "shorter than %d characters" % low, where)
        if high is not None and len(inst) > high:
            raise Violation("maxLength", at, "longer than %d characters" % high, where)
        if "pattern" in schema:
            if not self._pattern(schema["pattern"]).matches(inst):
                raise Violation("pattern", at, "value does not match %s" % schema["pattern"], where)
        if "format" in schema:
            problem = check_format(schema["format"], inst)
            if problem:
                raise Violation("format", at, problem, where)

    def _check_object(self, doc, where, schema, inst, at, steps):
        props = schema.get("properties", {})
        for name in schema.get("required", []):
            if name not in inst:
                raise Violation("required", at + "/" + _escape_pointer(name), "required member is missing", where)
        keys = sorted(inst)
        extra = schema.get("additionalProperties")
        if extra is False:
            for key in keys:
                if key not in props:
                    raise Violation("additionalProperties", at + "/" + _escape_pointer(key), "member is not allowed", where)
        for key in keys:
            if key in props:
                self._check(doc, where + "/properties/" + _escape_pointer(key), props[key], inst[key], at + "/" + _escape_pointer(key), steps)
        if isinstance(extra, dict):
            for key in keys:
                if key not in props:
                    self._check(doc, where + "/additionalProperties", extra, inst[key], at + "/" + _escape_pointer(key), steps)


def _bound(schema, key):
    value = schema.get(key)
    return value.integer() if isinstance(value, Number) else None


class EventError(ValueError):
    """Why a message cannot be accepted as a v1 event; quarantine it."""

    def __init__(self, code, message, violation=None):
        super().__init__("%s: %s" % (code, message))
        self.code, self.violation = code, violation


def event_schema_name(event_type):
    return "events/v1/%s.v1.schema.json" % event_type


def check_event(registry, raw):
    """Parses and validates one event; returns the parsed object.

    Codes: MALFORMED_JSON, UNSUPPORTED_SCHEMA_VERSION (an integer other than
    1), UNKNOWN_EVENT_TYPE (a string not in the v1 catalog) and
    SCHEMA_VIOLATION (the first violated rule, in the envelope or the event).
    """
    try:
        value = parse(raw)
    except ParseError as error:
        raise EventError("MALFORMED_JSON", str(error)) from None
    ref = "events/v1/envelope.schema.json"
    if isinstance(value, dict):
        version = value.get("schema_version")
        if isinstance(version, Number) and version.integer() is not None and version.integer() != 1:
            raise EventError("UNSUPPORTED_SCHEMA_VERSION", "schema_version %s is not 1" % version)
        event_type = value.get("event_type")
        if isinstance(event_type, str) and not isinstance(event_type, Number):
            if event_schema_name(event_type) not in registry.docs:
                raise EventError("UNKNOWN_EVENT_TYPE", "event_type is not in the v1 catalog")
            ref = event_schema_name(event_type)
    violation = registry.validate(ref, value)
    if violation is not None:
        raise EventError("SCHEMA_VIOLATION", violation.message, violation)
    return value


def verdict(registry, request):
    """One CLI answer: {"valid", "code", "keyword", "path", "canonical_b64"}."""
    raw = base64.b64decode(request["payload_b64"], validate=True)
    out = {"valid": True, "code": "", "keyword": "", "path": "", "canonical_b64": ""}
    try:
        if request.get("event"):
            value = check_event(registry, raw)
        else:
            value = parse(raw)
            violation = registry.validate(request["ref"], value)
            if violation is not None:
                raise EventError("SCHEMA_VIOLATION", violation.message, violation)
    except ParseError:
        out.update(valid=False, code="MALFORMED_JSON")
        return out
    except EventError as error:
        out.update(valid=False, code=error.code)
        if error.violation is not None:
            out.update(keyword=error.violation.keyword, path=error.violation.path)
        return out
    try:
        out["canonical_b64"] = base64.b64encode(canonical(value).encode("utf-8")).decode("ascii")
    except ValueError:
        out["canonical_b64"] = ""
    return out


def default_registry():
    from tradestocker_research.contract_documents import DOCUMENTS
    return Registry(DOCUMENTS)


def main(argv=None):
    if (sys.argv[1:] if argv is None else argv):
        print("usage: python -I -m tradestocker_research.contracts < requests.jsonl", file=sys.stderr)
        return 2
    registry = default_registry()
    for line in sys.stdin.buffer:
        if line.strip():
            print(json.dumps(verdict(registry, json.loads(line)), separators=(",", ":"), sort_keys=True), flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())

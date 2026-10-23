package contracts

import (
	"fmt"
	"io/fs"
	"math/big"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// Dialect names the schema rules this package implements: a strict subset of
// JSON Schema 2020-12 (see contracts/README.md). Python's
// tradestocker_research.contracts implements the same rules.
const Dialect = "tradestocker-schema-subset/1"

// Work limits, the same in Python. A schema's full expansion (following every
// $ref and allOf) may not exceed MaxSchemaNodes, schemas may nest (through
// subschemas and references) at most MaxSchemaDepth deep, and one validation
// may not take more than MaxValidationSteps schema-and-value checks.
const (
	MaxSchemaNodes     = 100_000
	MaxSchemaDepth     = 64
	MaxValidationSteps = 20_000_000
)

// Violation is the first rule an instance breaks. Keyword and Path are the same
// in Go and Python for the same input; Message is for people.
type Violation struct {
	Keyword string // the failing rule, such as "type", "required", "enum" or "format"
	Path    string // JSON Pointer to the instance location
	Schema  string // document#pointer of the failing schema
	Message string
}

func (v *Violation) Error() string {
	return fmt.Sprintf("%s at %q: %s", v.Keyword, v.Path, v.Message)
}

// Registry holds a fixed set of contract documents. A $ref resolves only to a
// document in the set; nothing is fetched or read when validating.
type Registry struct {
	docs map[string]any

	mu       sync.Mutex
	state    map[string]int   // compile state per location: 1 in progress, 2 done
	size     map[string]int   // expanded size per compiled location
	height   map[string]int   // longest subschema/reference path from a compiled location
	failures map[string]error // compile failure per requested location
	patterns map[string]*compiledPattern
}

// NewRegistry loads exactly the named documents (paths inside fsys, which is
// the contracts/ folder).
func NewRegistry(fsys fs.FS, names ...string) (*Registry, error) {
	raw := map[string][]byte{}
	for _, name := range names {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		raw[name] = b
	}
	return NewRegistryFromBytes(raw)
}

// NewRegistryFromBytes builds a registry from in-memory documents keyed by
// their path inside contracts/. Every document whose root declares $schema is
// a schema document and is compiled here; a malformed one fails the
// constructor. Schemas inside other documents (OpenAPI) are compiled by
// Compile, which Validate always calls first.
func NewRegistryFromBytes(raw map[string][]byte) (*Registry, error) {
	r := &Registry{docs: map[string]any{}, state: map[string]int{}, size: map[string]int{}, height: map[string]int{},
		failures: map[string]error{}, patterns: map[string]*compiledPattern{}}
	for name, b := range raw {
		if name != path.Clean(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || name == ".." {
			return nil, fmt.Errorf("document name %q must be a clean relative path", name)
		}
		v, err := Parse(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		r.docs[name] = v
	}
	for _, name := range r.Names() {
		if root, ok := r.docs[name].(map[string]any); ok {
			if _, isSchema := root["$schema"]; isSchema {
				if err := r.Compile(name); err != nil {
					return nil, err
				}
			}
		}
	}
	return r, nil
}

// Names lists the loaded documents.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.docs))
	for n := range r.docs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Document returns a loaded document.
func (r *Registry) Document(name string) (any, bool) {
	v, ok := r.docs[name]
	return v, ok
}

// Validate compiles the schema at ref ("doc" or "doc#/pointer") and checks
// instance against it. It returns the first violation, or nil. A schema that
// does not compile is a "schema" violation: it never validates anything.
func (r *Registry) Validate(ref string, instance any) *Violation {
	if err := r.Compile(ref); err != nil {
		return &Violation{Keyword: "schema", Path: "", Schema: ref, Message: err.Error()}
	}
	doc, pointer, node, _ := r.resolve("", ref)
	steps := 0
	return r.check(doc, doc+"#"+pointer, node, instance, "", &steps)
}

// resolve finds the schema a reference points at. Only documents in the
// registry are reachable: absolute URIs, unknown files and bad pointers fail.
func (r *Registry) resolve(base, ref string) (string, string, any, error) {
	file, pointer, _ := strings.Cut(ref, "#")
	if strings.Contains(file, ":") {
		return "", "", nil, fmt.Errorf("reference %q is not a local contract document", ref)
	}
	doc := base
	if file != "" {
		doc = path.Clean(path.Join(path.Dir(base), file))
	}
	root, ok := r.docs[doc]
	if !ok {
		return "", "", nil, fmt.Errorf("reference %q names no loaded contract document", ref)
	}
	node, err := pointerGet(root, pointer)
	if err != nil {
		return "", "", nil, fmt.Errorf("reference %q: %v", ref, err)
	}
	return doc, pointer, node, nil
}

func pointerGet(root any, pointer string) (any, error) {
	if pointer == "" {
		return root, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("pointer %q must start with /", pointer)
	}
	node := root
	for _, part := range strings.Split(pointer[1:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch x := node.(type) {
		case map[string]any:
			next, ok := x[part]
			if !ok {
				return nil, fmt.Errorf("no member %q", part)
			}
			node = next
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(x) || strconv.Itoa(i) != part {
				return nil, fmt.Errorf("no index %q", part)
			}
			node = x[i]
		default:
			return nil, fmt.Errorf("cannot step into %q", part)
		}
	}
	return node, nil
}

func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// keywords are the only schema keywords the dialect knows; any other keyword
// (apart from x-tradestocker- annotations) makes the schema fail to compile.
var keywords = map[string]bool{
	"$schema": true, "$id": true, "$ref": true, "$defs": true, "$comment": true,
	"title": true, "description": true, "examples": true, "deprecated": true,
	"allOf": true, "properties": true, "additionalProperties": true, "items": true,
	"type": true, "enum": true, "const": true, "required": true,
	"minLength": true, "maxLength": true, "pattern": true, "format": true,
	"minimum": true, "maximum": true, "minItems": true, "maxItems": true,
}

func knownKeyword(k string) bool { return keywords[k] || strings.HasPrefix(k, "x-tradestocker-") }

var typeNames = map[string]bool{"object": true, "array": true, "string": true, "integer": true, "boolean": true, "null": true}

// Compile checks the schema at ref, everything it contains and every schema it
// references, independent of any instance: every keyword is known and has the
// right shape, every $ref resolves to a loaded document, patterns are portable
// in the dialect's own grammar (pattern.go), there is no reference cycle (a
// shared, non-cyclic reference is fine), nesting stays within MaxSchemaDepth
// and the full expansion within MaxSchemaNodes. The result is cached.
func (r *Registry) Compile(ref string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err, failed := r.failures[ref]; failed {
		return err
	}
	doc, pointer, node, err := r.resolve("", ref)
	if err == nil {
		_, _, err = r.compileNode(doc, pointer, node, 1)
	}
	if err != nil {
		r.failures[ref] = err
		// Forget in-progress marks from the failed attempt.
		for loc, st := range r.state {
			if st == 1 {
				delete(r.state, loc)
			}
		}
	}
	return err
}

// compileNode returns the expanded size and the height (longest path through
// subschemas and references, counting this node) of the schema at loc. A node
// compiled earlier is reused only if it still fits under MaxSchemaDepth at this
// depth, so the limit does not depend on the order of compilation.
func (r *Registry) compileNode(doc, pointer string, node any, depth int) (int, int, error) {
	loc := doc + "#" + pointer
	deep := fmt.Errorf("%s: schemas nest deeper than %d (subschemas and references)", loc, MaxSchemaDepth)
	if depth > MaxSchemaDepth {
		return 0, 0, deep
	}
	switch r.state[loc] {
	case 1:
		return 0, 0, fmt.Errorf("%s: reference cycle (recursive schemas are not supported)", loc)
	case 2:
		if depth+r.height[loc]-1 > MaxSchemaDepth {
			return 0, 0, deep
		}
		return r.size[loc], r.height[loc], nil
	}
	r.state[loc] = 1
	size, height, err := r.compileShape(doc, pointer, node, depth)
	if err != nil {
		return 0, 0, err
	}
	if size > MaxSchemaNodes {
		return 0, 0, fmt.Errorf("%s: schema expands to more than %d nodes", loc, MaxSchemaNodes)
	}
	r.state[loc], r.size[loc], r.height[loc] = 2, size, height
	return size, height, nil
}

func (r *Registry) compileShape(doc, pointer string, node any, depth int) (int, int, error) {
	loc := doc + "#" + pointer
	bad := func(format string, args ...any) (int, int, error) {
		return 0, 0, fmt.Errorf("%s: %s", loc, fmt.Sprintf(format, args...))
	}
	if _, ok := node.(bool); ok {
		return 1, 1, nil
	}
	s, ok := node.(map[string]any)
	if !ok {
		return bad("a schema must be an object or a boolean")
	}
	size, below := 1, 0
	child := func(sub string, n any) error {
		k, h, err := r.compileNode(doc, pointer+sub, n, depth+1)
		size += k
		below = max(below, h)
		return err
	}
	for _, k := range sortedKeys(s) {
		v := s[k]
		switch {
		case strings.HasPrefix(k, "x-tradestocker-"):
		case !keywords[k]:
			return bad("unsupported keyword %q", k)
		case k == "$schema":
			if pointer != "" || v != "https://json-schema.org/draft/2020-12/schema" {
				return bad("$schema must be JSON Schema 2020-12, at a document root only")
			}
		case k == "$id":
			if _, isString := v.(string); !isString || pointer != "" {
				return bad("$id must be a string, at a document root only")
			}
		case k == "$comment" || k == "title" || k == "description":
			if _, isString := v.(string); !isString {
				return bad("%s must be a string", k)
			}
		case k == "examples":
			if _, isList := v.([]any); !isList {
				return bad("examples must be a list")
			}
		case k == "deprecated":
			if _, isBool := v.(bool); !isBool {
				return bad("deprecated must be a boolean")
			}
		case k == "$ref":
			ref, isString := v.(string)
			if !isString {
				return bad("$ref must be a string")
			}
			tdoc, tpointer, target, err := r.resolve(doc, ref)
			if err != nil {
				return bad("%v", err)
			}
			n, h, err := r.compileNode(tdoc, tpointer, target, depth+1)
			if err != nil {
				return 0, 0, err
			}
			size += n
			below = max(below, h)
		case k == "$defs" || k == "properties":
			m, isMap := v.(map[string]any)
			if !isMap {
				return bad("%s must be an object of schemas", k)
			}
			for _, name := range sortedKeys(m) {
				if err := child("/"+k+"/"+escapePointer(name), m[name]); err != nil {
					return 0, 0, err
				}
			}
		case k == "allOf":
			list, isList := v.([]any)
			if !isList || len(list) == 0 {
				return bad("allOf must be a non-empty list of schemas")
			}
			for i, sub := range list {
				if err := child(fmt.Sprintf("/allOf/%d", i), sub); err != nil {
					return 0, 0, err
				}
			}
		case k == "additionalProperties" || k == "items":
			if err := child("/"+k, v); err != nil {
				return 0, 0, err
			}
		case k == "type":
			if !validType(v) {
				return bad(`type must be one of object, array, string, integer, boolean, null, or a list of distinct names ("number" is not supported)`)
			}
		case k == "enum":
			if list, isList := v.([]any); !isList || len(list) == 0 {
				return bad("enum must be a non-empty list")
			}
		case k == "const":
		case k == "required":
			list, isList := v.([]any)
			if !isList {
				return bad("required must be a list of names")
			}
			seen := map[string]bool{}
			for _, n := range list {
				name, isString := n.(string)
				if !isString || seen[name] {
					return bad("required must be a list of distinct names")
				}
				seen[name] = true
			}
		case k == "minLength" || k == "maxLength" || k == "minItems" || k == "maxItems":
			if n, isInt := intKeyword(v); !isInt || n.Sign() < 0 || !n.IsInt64() {
				return bad("%s must be a non-negative integer", k)
			}
		case k == "minimum" || k == "maximum":
			if _, isInt := intKeyword(v); !isInt {
				return bad("%s must be an integer", k)
			}
		case k == "pattern":
			p, isString := v.(string)
			if !isString {
				return bad("pattern must be a string")
			}
			if _, err := r.patternLocked(p); err != nil {
				return bad("%v", err)
			}
		case k == "format":
			if v != "date-time" && v != "date" {
				return bad("format must be date-time or date")
			}
		}
	}
	return size, below + 1, nil
}

func intKeyword(v any) (*big.Int, bool) {
	n, ok := v.(Number)
	if !ok {
		return nil, false
	}
	return n.Integer()
}

func validType(v any) bool {
	switch x := v.(type) {
	case string:
		return typeNames[x]
	case []any:
		if len(x) == 0 {
			return false
		}
		seen := map[string]bool{}
		for _, e := range x {
			name, ok := e.(string)
			if !ok || !typeNames[name] || seen[name] {
				return false
			}
			seen[name] = true
		}
		return true
	}
	return false
}

func (r *Registry) patternLocked(p string) (*compiledPattern, error) {
	if c, ok := r.patterns[p]; ok {
		return c, nil
	}
	c, err := compilePattern(p)
	if err != nil {
		return nil, err
	}
	r.patterns[p] = c
	return c, nil
}

func (r *Registry) pattern(p string) *compiledPattern {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, _ := r.patternLocked(p) // compiled successfully by Compile
	return c
}

// check applies a compiled schema (at doc, located at where) to inst at path.
// The order of the checks is part of the contract, so both languages report
// the same first violation: $ref, allOf, type, const, enum, string rules,
// integer rules, object rules (required in listed order, unknown members
// sorted, then known members sorted), array rules.
func (r *Registry) check(doc, where string, schema, inst any, at string, steps *int) *Violation {
	fail := func(keyword, format string, args ...any) *Violation {
		return &Violation{Keyword: keyword, Path: at, Schema: where, Message: fmt.Sprintf(format, args...)}
	}
	if *steps++; *steps > MaxValidationSteps {
		return fail("budget", "validation needs more than %d steps", MaxValidationSteps)
	}
	if b, ok := schema.(bool); ok {
		if !b {
			return fail("false", "no value is allowed here")
		}
		return nil
	}
	s := schema.(map[string]any)
	if ref, ok := s["$ref"].(string); ok {
		tdoc, pointer, node, _ := r.resolve(doc, ref)
		if v := r.check(tdoc, tdoc+"#"+pointer, node, inst, at, steps); v != nil {
			return v
		}
	}
	if all, ok := s["allOf"].([]any); ok {
		for i, sub := range all {
			if v := r.check(doc, fmt.Sprintf("%s/allOf/%d", where, i), sub, inst, at, steps); v != nil {
				return v
			}
		}
	}
	if t, ok := s["type"]; ok && !typeMatches(t, inst) {
		return fail("type", "expected %s", typeText(t))
	}
	if c, ok := s["const"]; ok && !equal(c, inst) {
		return fail("const", "value is not the required constant")
	}
	if e, ok := s["enum"].([]any); ok {
		found := false
		for _, option := range e {
			if equal(option, inst) {
				found = true
				break
			}
		}
		if !found {
			return fail("enum", "value is not one of the allowed values")
		}
	}
	switch x := inst.(type) {
	case string:
		length := utf8.RuneCountInString(x)
		if n, ok := limit(s, "minLength"); ok && length < n {
			return fail("minLength", "shorter than %d characters", n)
		}
		if n, ok := limit(s, "maxLength"); ok && length > n {
			return fail("maxLength", "longer than %d characters", n)
		}
		if p, ok := s["pattern"].(string); ok && !r.pattern(p).matches(x) {
			return fail("pattern", "value does not match %s", p)
		}
		if f, ok := s["format"].(string); ok {
			if msg := checkFormat(f, x); msg != "" {
				return fail("format", "%s", msg)
			}
		}
	case Number:
		if n, ok := x.Integer(); ok {
			if m, ok := bound(s, "minimum"); ok && n.Cmp(m) < 0 {
				return fail("minimum", "less than %s", m)
			}
			if m, ok := bound(s, "maximum"); ok && n.Cmp(m) > 0 {
				return fail("maximum", "greater than %s", m)
			}
		}
	case map[string]any:
		props, _ := s["properties"].(map[string]any)
		if req, ok := s["required"].([]any); ok {
			for _, name := range req {
				key := name.(string)
				if _, present := x[key]; !present {
					return &Violation{Keyword: "required", Path: at + "/" + escapePointer(key), Schema: where, Message: "required member is missing"}
				}
			}
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		extra, hasExtra := s["additionalProperties"]
		if b, isBool := extra.(bool); hasExtra && isBool && !b {
			for _, k := range keys {
				if _, known := props[k]; !known {
					return &Violation{Keyword: "additionalProperties", Path: at + "/" + escapePointer(k), Schema: where, Message: "member is not allowed"}
				}
			}
		}
		for _, k := range keys {
			if sub, known := props[k]; known {
				if v := r.check(doc, where+"/properties/"+escapePointer(k), sub, x[k], at+"/"+escapePointer(k), steps); v != nil {
					return v
				}
			}
		}
		if sub, isSchema := extra.(map[string]any); isSchema {
			for _, k := range keys {
				if _, known := props[k]; !known {
					if v := r.check(doc, where+"/additionalProperties", sub, x[k], at+"/"+escapePointer(k), steps); v != nil {
						return v
					}
				}
			}
		}
	case []any:
		if n, ok := limit(s, "minItems"); ok && len(x) < n {
			return fail("minItems", "fewer than %d items", n)
		}
		if n, ok := limit(s, "maxItems"); ok && len(x) > n {
			return fail("maxItems", "more than %d items", n)
		}
		if items, ok := s["items"]; ok {
			for i, e := range x {
				if v := r.check(doc, where+"/items", items, e, at+"/"+strconv.Itoa(i), steps); v != nil {
					return v
				}
			}
		}
	}
	return nil
}

func limit(s map[string]any, key string) (int, bool) {
	n, ok := bound(s, key)
	if !ok || !n.IsInt64() {
		return 0, false
	}
	return int(n.Int64()), true
}

func bound(s map[string]any, key string) (*big.Int, bool) {
	return intKeyword(s[key])
}

func typeMatches(t, inst any) bool {
	switch x := t.(type) {
	case string:
		return isType(x, inst)
	case []any:
		for _, name := range x {
			if s, ok := name.(string); ok && isType(s, inst) {
				return true
			}
		}
	}
	return false
}

func isType(name string, inst any) bool {
	switch name {
	case "object":
		_, ok := inst.(map[string]any)
		return ok
	case "array":
		_, ok := inst.([]any)
		return ok
	case "string":
		_, ok := inst.(string)
		return ok
	case "integer":
		n, ok := inst.(Number)
		if !ok {
			return false
		}
		_, ok = n.Integer()
		return ok
	case "boolean":
		_, ok := inst.(bool)
		return ok
	case "null":
		return inst == nil
	}
	return false
}

func typeText(t any) string {
	switch x := t.(type) {
	case string:
		return x
	case []any:
		parts := make([]string, 0, len(x))
		for _, p := range x {
			parts = append(parts, fmt.Sprint(p))
		}
		return strings.Join(parts, " or ")
	}
	return fmt.Sprint(t)
}

// equal compares parsed JSON values; integers compare by value.
func equal(a, b any) bool {
	switch x := a.(type) {
	case Number:
		y, ok := b.(Number)
		if !ok {
			return false
		}
		xi, xok := x.Integer()
		yi, yok := y.Integer()
		if xok && yok {
			return xi.Cmp(yi) == 0
		}
		return x == y
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !equal(v, w) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equal(x[i], y[i]) {
				return false
			}
		}
		return true
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case nil:
		return b == nil
	}
	return false
}

var (
	dateTimeShape = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})([.][0-9]{1,9})?(Z|[+]00:00)$`)
	dateShape     = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})$`)
)

// checkFormat asserts the two supported formats; it is not an annotation.
// "date-time": RFC 3339 in UTC only (Z or +00:00; -00:00 and other offsets
// are refused), years 0001-9999, real calendar days, hours 00-23, no leap
// second, 1 to 9 fraction digits. "date": a real calendar day.
func checkFormat(format, s string) string {
	switch format {
	case "date-time":
		m := dateTimeShape.FindStringSubmatch(s)
		if m == nil {
			return "not a UTC RFC 3339 date-time (Z or +00:00)"
		}
		if !realDay(m[1], m[2], m[3]) {
			return "not a real calendar date"
		}
		if atoi(m[4]) > 23 || atoi(m[5]) > 59 || atoi(m[6]) > 59 {
			return "not a real time of day"
		}
	case "date":
		m := dateShape.FindStringSubmatch(s)
		if m == nil || !realDay(m[1], m[2], m[3]) {
			return "not a real calendar date (YYYY-MM-DD)"
		}
	default:
		return "unsupported format " + format
	}
	return ""
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func realDay(y, m, d string) bool {
	year, month, day := atoi(y), atoi(m), atoi(d)
	if year < 1 || month < 1 || month > 12 || day < 1 {
		return false
	}
	days := [...]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}[month-1]
	if month == 2 && (year%4 == 0 && (year%100 != 0 || year%400 == 0)) {
		days = 29
	}
	return day <= days
}

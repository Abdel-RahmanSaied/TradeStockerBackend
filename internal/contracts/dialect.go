package contracts

import (
	"fmt"
	"math/big"
	"regexp"
	"sort"
)

// Profile holds the authoring rules a contract document must follow on top of
// the dialect's keywords.
type Profile struct {
	Name string
	// AllRequired: every declared property is required and objects are closed;
	// a value that does not apply is null, never missing.
	AllRequired bool
	// ClosedStrings: every string has a pattern, enum, const, format or $ref,
	// so no free text can appear.
	ClosedStrings bool
	// BoundedText: a string without such a shape must have maxLength.
	BoundedText bool
}

var (
	// EventProfile is for contracts/events/v1.
	EventProfile = Profile{Name: "event", AllRequired: true, ClosedStrings: true}
	// ServiceProfile is for Go/Python service payloads (contracts/research).
	ServiceProfile = Profile{Name: "service", BoundedText: true}
	// APIProfile is for OpenAPI request and response schemas.
	APIProfile = Profile{Name: "api", BoundedText: true}
)

var maxSafeInteger = big.NewInt(9007199254740991)

// Property names are ASCII, so both languages sort and escape them the same
// way. New event contracts also use snake_case; service and API contracts keep
// existing names (the SEC report's reason codes are upper case).
var (
	snakeCase = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	asciiName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
)

// CheckDialect lists every authoring problem in the schema at ref and the
// schemas it contains or references. An empty list means the schema uses only
// the supported dialect, follows profile, and every $ref resolves locally.
func (r *Registry) CheckDialect(ref string, p Profile) []string {
	// Compile first: it guarantees every keyword's shape, so the profile walk
	// below can rely on it.
	if err := r.Compile(ref); err != nil {
		return []string{err.Error()}
	}
	doc, pointer, node, _ := r.resolve("", ref)
	c := &dialectCheck{r: r, p: p, seen: map[string]bool{}}
	c.walk(doc, pointer, node)
	sort.Strings(c.problems)
	return c.problems
}

type dialectCheck struct {
	r        *Registry
	p        Profile
	seen     map[string]bool
	problems []string
}

func (c *dialectCheck) add(where, format string, args ...any) {
	c.problems = append(c.problems, where+": "+fmt.Sprintf(format, args...))
}

// walk applies the profile to the schema at doc#pointer. A document root is
// the empty pointer, wherever the walk came from; Compile has already refused
// reference cycles and $schema or $id below a root.
func (c *dialectCheck) walk(doc, pointer string, node any) {
	where := doc + "#" + pointer
	if c.seen[where] {
		return
	}
	c.seen[where] = true
	if b, ok := node.(bool); ok {
		if b {
			c.add(where, "boolean schema true accepts anything; describe the value")
		}
		return
	}
	s, ok := node.(map[string]any)
	if !ok {
		c.add(where, "schema must be an object")
		return
	}
	types := c.types(where, s)
	if ref, ok := s["$ref"].(string); ok {
		tdoc, tpointer, target, _ := c.r.resolve(doc, ref)
		c.walk(tdoc, tpointer, target)
	}
	if defs, ok := s["$defs"].(map[string]any); ok {
		for _, k := range sortedKeys(defs) {
			c.walk(doc, pointer+"/$defs/"+escapePointer(k), defs[k])
		}
	}
	if all, ok := s["allOf"].([]any); ok {
		for i, sub := range all {
			c.walk(doc, fmt.Sprintf("%s/allOf/%d", pointer, i), sub)
		}
	}
	if types["integer"] {
		lo, okLo := bound(s, "minimum")
		hi, okHi := bound(s, "maximum")
		if !okLo || !okHi {
			c.add(where, "an integer needs minimum and maximum")
		} else if lo.CmpAbs(maxSafeInteger) > 0 || hi.CmpAbs(maxSafeInteger) > 0 {
			c.add(where, "integer bounds must stay within ±(2^53 - 1)")
		}
	}
	if types["string"] {
		_, hasRef := s["$ref"]
		_, hasPattern := s["pattern"]
		_, hasEnum := s["enum"]
		_, hasConst := s["const"]
		_, hasFormat := s["format"]
		_, hasMax := s["maxLength"]
		closed := hasRef || hasPattern || hasEnum || hasConst || hasFormat
		if c.p.ClosedStrings && !closed {
			c.add(where, "an event string needs a pattern, enum, const, format or $ref")
		}
		if c.p.BoundedText && !closed && !hasMax {
			c.add(where, "free text needs maxLength")
		}
	}
	external := s["x-tradestocker-external-original"] == true
	props, hasProps := s["properties"].(map[string]any)
	if hasProps {
		for _, k := range sortedKeys(props) {
			switch {
			case external:
			case c.p.AllRequired && !snakeCase.MatchString(k):
				c.add(where, "event property name %q must be snake_case ASCII", k)
			case !asciiName.MatchString(k):
				c.add(where, "property name %q must be an ASCII identifier", k)
			}
			c.walk(doc, pointer+"/properties/"+escapePointer(k), props[k])
		}
	}
	extra, hasExtra := s["additionalProperties"]
	switch x := extra.(type) {
	case bool:
		if x && !external {
			c.add(where, "additionalProperties true is allowed only on an external original document")
		}
	case map[string]any:
		if c.p.AllRequired {
			c.add(where, "event objects are closed: additionalProperties must be false")
		}
		c.walk(doc, pointer+"/additionalProperties", x)
	}
	if hasProps && !hasExtra && !external {
		c.add(where, "an object with properties must set additionalProperties")
	}
	if c.p.AllRequired && hasProps {
		req, _ := s["required"].([]any)
		have := map[string]bool{}
		for _, n := range req {
			if name, ok := n.(string); ok {
				have[name] = true
			}
		}
		for k := range props {
			if !have[k] {
				c.add(where, "event property %q must be required (use null for a value that does not apply)", k)
			}
		}
	}
	if req, ok := s["required"].([]any); ok {
		for _, n := range req {
			if _, declared := props[n.(string)]; !declared {
				c.add(where, "required %v is not a declared property", n)
			}
		}
	}
	if items, ok := s["items"]; ok {
		c.walk(doc, pointer+"/items", items)
		if _, ok := s["maxItems"]; !ok {
			c.add(where, "an array needs maxItems")
		}
	}
}

func (c *dialectCheck) types(where string, s map[string]any) map[string]bool {
	out := map[string]bool{}
	switch t := s["type"].(type) {
	case string:
		out[t] = true
	case []any:
		for _, x := range t {
			out[x.(string)] = true
		}
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

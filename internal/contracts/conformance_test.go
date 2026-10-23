package contracts

import (
	"encoding/base64"
	"os"
	"testing"
)

// conformanceAnswer runs one case of contracts/fixtures/conformance and
// returns "valid", a code, or "keyword@path". Python's test runs the same
// cases and must give the same answers.
func conformanceAnswer(t *testing.T, c map[string]any) string {
	t.Helper()
	docs := map[string][]byte{}
	if c["contracts"] == true {
		for _, name := range eventFiles(t) {
			b, err := os.ReadFile("../../contracts/" + name)
			if err != nil {
				t.Fatal(err)
			}
			docs[name] = b
		}
	}
	if d, ok := c["documents"].(map[string]any); ok {
		for name, text := range d {
			docs[name] = []byte(text.(string))
		}
	}
	r, err := NewRegistryFromBytes(docs)
	if err != nil {
		return "SCHEMA_ERROR"
	}
	var raw []byte
	if s, ok := c["raw"].(string); ok {
		raw = []byte(s)
	} else if s, ok := c["raw_b64"].(string); ok {
		if raw, err = base64.StdEncoding.DecodeString(s); err != nil {
			t.Fatal(err)
		}
	}
	v, err := Parse(raw)
	if err != nil {
		return "MALFORMED_JSON"
	}
	violation := r.Validate(c["ref"].(string), v)
	switch {
	case violation == nil:
		return "valid"
	case violation.Keyword == "schema":
		return "SCHEMA_ERROR"
	}
	return violation.Keyword + "@" + violation.Path
}

func expectedAnswer(e any) string {
	switch x := e.(type) {
	case string:
		return x
	case map[string]any:
		if code, ok := x["code"].(string); ok {
			return code
		}
		return x["keyword"].(string) + "@" + x["path"].(string)
	}
	return "?"
}

func TestValidatorConformanceCorpus(t *testing.T) {
	raw, err := os.ReadFile("../../contracts/fixtures/conformance/validator.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cases := corpus.(map[string]any)["cases"].([]any)
	if corpus.(map[string]any)["dialect"] != Dialect {
		t.Fatalf("corpus is for dialect %v", corpus.(map[string]any)["dialect"])
	}
	seen := map[string]bool{}
	for _, item := range cases {
		c := item.(map[string]any)
		name := c["name"].(string)
		if seen[name] {
			t.Fatalf("duplicate case name %q", name)
		}
		seen[name] = true
		if got, want := conformanceAnswer(t, c), expectedAnswer(c["expect"]); got != want {
			t.Errorf("%s: got %s, want %s", name, got, want)
		}
	}
	if len(cases) < 150 {
		t.Fatalf("only %d cases", len(cases))
	}
}

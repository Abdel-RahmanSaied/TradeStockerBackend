package contracts

import (
	"fmt"
	"strings"
	"testing"
)

// chainDocs is a00 -> a01 -> ... -> a(n-1), the last one {"type":"null"}.
// Without a root $schema nothing compiles at load, so the test chooses the
// order.
func chainDocs(n int) map[string][]byte {
	docs := map[string][]byte{}
	for i := 0; i < n; i++ {
		body := fmt.Sprintf(`{"$ref":"a%02d.json"}`, i+1)
		if i == n-1 {
			body = `{"type":"null"}`
		}
		docs[fmt.Sprintf("a%02d.json", i)] = []byte(body)
	}
	return docs
}

// Review 044/045: a subtree compiled first must not let a later parent exceed
// MaxSchemaDepth, whatever the order, and a failure is final.
func TestDepthLimitHoldsForPrewarmedSubtrees(t *testing.T) {
	r, err := NewRegistryFromBytes(chainDocs(70))
	if err != nil {
		t.Fatal(err)
	}
	for i := 69; i >= 0; i-- { // leaf first: every parent finds its child cached
		err := r.Compile(fmt.Sprintf("a%02d.json", i))
		if fits := 70-i <= MaxSchemaDepth; fits != (err == nil) {
			t.Fatalf("a%02d (height %d): %v", i, 70-i, err)
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := r.Compile("a00.json"); err == nil || !strings.Contains(err.Error(), "nest deeper") {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if v := r.Validate("a00.json", nil); v == nil || v.Keyword != "schema" {
		t.Fatalf("validation of a schema that does not compile: %v", v)
	}
	if err := r.Compile("a10.json"); err != nil {
		t.Fatalf("a shallower subtree must still compile: %v", err)
	}
	if v := r.Validate("a10.json", nil); v != nil {
		t.Fatal(v)
	}
}

func TestPatternCountsAreUnsignedDigits(t *testing.T) {
	for _, p := range []string{"^a{+1}$", "^a{-1}$", "^a{1,+2}$", "^a{1,-2}$", "^a{ 1}$", "^a{1 }$", "^a{0x1}$", "^a{1e1}$"} {
		if _, err := compilePattern(p); err == nil {
			t.Errorf("%s compiled", p)
		}
	}
	for _, p := range []string{"^a{0}$", "^a{1000}$", "^a{0,1000}$"} {
		if _, err := compilePattern(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}

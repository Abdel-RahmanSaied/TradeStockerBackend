package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func eventAnswer(r *Registry, raw []byte) string {
	_, err := CheckEvent(r, raw)
	var e *EventError
	switch {
	case err == nil:
		return "valid"
	case !errors.As(err, &e):
		return "error: " + err.Error()
	case e.Violation != nil:
		return e.Code + " " + e.Violation.Keyword + "@" + e.Violation.Path
	}
	return e.Code
}

func expectedEventAnswer(expect any) string {
	if s, ok := expect.(string); ok {
		return s
	}
	m := expect.(map[string]any)
	if k, ok := m["keyword"].(string); ok {
		return m["code"].(string) + " " + k + "@" + m["path"].(string)
	}
	return m["code"].(string)
}

func TestEventConformanceCorpus(t *testing.T) {
	r := eventRegistry(t)
	raw, err := os.ReadFile("../../contracts/fixtures/conformance/events.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cases := corpus.(map[string]any)["cases"].([]any)
	for _, item := range cases {
		c := item.(map[string]any)
		if got, want := eventAnswer(r, []byte(c["raw"].(string))), expectedEventAnswer(c["expect"]); got != want {
			t.Errorf("%s: got %s, want %s", c["name"], got, want)
		}
	}
	if len(cases) < 90 {
		t.Fatalf("only %d cases", len(cases))
	}
}

func goldenEvents(t *testing.T) map[string][]byte {
	t.Helper()
	files, err := filepath.Glob("../../contracts/fixtures/events/v1/*.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[strings.TrimSuffix(filepath.Base(f), ".v1.json")] = bytes.TrimSuffix(b, []byte("\n"))
	}
	return out
}

// Cross-language golden: decoding a fixture into its generated Go type and
// encoding it again gives exactly the fixture's canonical bytes. Python's test
// does the same with its generated dataclasses.
func TestGoldenEventsRoundTripExactly(t *testing.T) {
	r := eventRegistry(t)
	golden := goldenEvents(t)
	if len(golden) != len(EventTypes) {
		t.Fatalf("%d golden events for %d event types", len(golden), len(EventTypes))
	}
	for _, eventType := range EventTypes {
		raw, ok := golden[eventType]
		if !ok {
			t.Errorf("no golden fixture for %s", eventType)
			continue
		}
		parsed, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if canon, err := Canonical(parsed); err != nil || !bytes.Equal(canon, raw) {
			t.Errorf("%s: fixture is not in canonical form", eventType)
		}
		typed, err := DecodeEvent(r, raw)
		if err != nil {
			t.Errorf("%s: %v", eventType, err)
			continue
		}
		encoded, err := json.Marshal(typed)
		if err != nil {
			t.Fatal(err)
		}
		back, err := Parse(encoded)
		if err != nil {
			t.Fatal(err)
		}
		canon, err := Canonical(back)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(canon, raw) {
			t.Errorf("%s: typed round trip changed the bytes:\n got %s\nwant %s", eventType, canon, raw)
		}
	}
}

func TestDecodeEventReturnsTheGeneratedType(t *testing.T) {
	r := eventRegistry(t)
	golden := goldenEvents(t)
	typed, err := DecodeEvent(r, golden["research.assessment.completed"])
	if err != nil {
		t.Fatal(err)
	}
	e, ok := typed.(*Event[ResearchAssessmentCompletedV1])
	if !ok || e.Data.Status != "INSUFFICIENT_DATA" || e.Data.EligibleForEntry || e.Data.AssessmentRevision != 1 || e.AccountID != nil {
		t.Fatalf("decoded %#v", typed)
	}
	exec, err := DecodeEvent(r, golden["portfolio.execution.recorded"])
	if err != nil {
		t.Fatal(err)
	}
	x := exec.(*Event[PortfolioExecutionRecordedV1])
	if x.Data.Price != "199.85" || x.Data.Quantity != "10" || x.AccountID == nil || *x.AccountID != "acct_fixture_001" {
		t.Fatalf("execution %#v", x)
	}
	if _, err := DecodeEvent(r, []byte(`{"event_type":"research.assessment.deleted"}`)); !strings.Contains(err.Error(), UnknownEventType) {
		t.Fatalf("unknown type: %v", err)
	}
}

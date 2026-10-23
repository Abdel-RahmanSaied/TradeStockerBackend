package research

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	researchworker "tradestocker/research"
)

// Parser tests run on TS_PYTHON (make test and CI set it to the pinned CPython).
// Without a supported interpreter they skip, unless TS_REQUIRE_PYTHON=1 makes
// that a failure.
func TestMain(m *testing.M) {
	if python := os.Getenv("TS_PYTHON"); python != "" {
		defaultPython = python
	}
	os.Exit(m.Run())
}

func requirePython(t *testing.T) {
	t.Helper()
	if _, err := CheckPython(context.Background(), defaultPython); err != nil {
		if os.Getenv("TS_REQUIRE_PYTHON") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
}

func fixtureJob() Job {
	return Job{ID: "research_test", Symbol: fixtureSymbol, CIK: fixtureCIK, Attempts: 1}
}

func TestEmbeddedParserProducesGatedReport(t *testing.T) {
	requirePython(t)
	out, err := (&Service{}).analyze(context.Background(), fixtureJob(), fixtureSources())
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Version string `json:"version"`
		Status  string `json:"status"`
		Periods []struct {
			End     string `json:"fiscal_period_end"`
			Missing []string
		} `json:"periods"`
	}
	if json.Unmarshal(out, &r) != nil || r.Version != ParserVersion || r.Status != "PARTIAL" || len(r.Periods) != 1 || r.Periods[0].End != "2025-09-27" {
		t.Fatalf("unexpected report %s", out)
	}
}

func TestPythonReceivesNoSecrets(t *testing.T) {
	requirePython(t)
	t.Setenv("DATABASE_URL", "postgres://secret")
	t.Setenv("TS_LOCAL_DEV_TOKEN", "secret-token")
	t.Setenv("TS_SEC_USER_AGENT", "TradeStocker Research secret@example.com")
	s := &Service{Script: "import json,os,sys; print(json.dumps(sorted(os.environ)), json.dumps(sys.flags.isolated))"}
	cmd := s.pythonCommand(context.Background())
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, leaked := range []string{"DATABASE_URL", "TS_LOCAL_DEV_TOKEN", "TS_SEC_USER_AGENT", "PYTHONPATH"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("%s reached Python: %s", leaked, text)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(text), "1") {
		t.Fatalf("Python not isolated: %s", text)
	}
}

func TestReportGateRejectsUnsafeReports(t *testing.T) {
	sources := fixtureSources()
	sub, facts := sources[0].SHA256, sources[1].SHA256
	good := map[string]any{"version": ParserVersion, "symbol": fixtureSymbol, "cik": fixtureCIK, "data_kind": "OFFICIAL_FILINGS", "outcome": "INSUFFICIENT_DATA",
		"eligible_for_entry": false, "eligible_for_backtest": false, "financial_state_write_access": false,
		"sources": []map[string]string{{"kind": "submissions", "sha256": sub}, {"kind": "companyfacts", "sha256": facts}}}
	encode := func(change map[string]any) []byte {
		r := map[string]any{}
		for k, v := range good {
			r[k] = v
		}
		for k, v := range change {
			if v == nil {
				delete(r, k)
			} else {
				r[k] = v
			}
		}
		b, _ := json.Marshal(r)
		return b
	}
	if err := checkReport(encode(nil), fixtureJob(), sub, facts); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]map[string]any{
		"entry authority":    {"eligible_for_entry": true},
		"backtest authority": {"eligible_for_backtest": true},
		"write authority":    {"financial_state_write_access": true},
		"missing entry flag": {"eligible_for_entry": nil},
		"actionable outcome": {"outcome": "ACTIONABLE"},
		"other issuer":       {"cik": "0000000002"},
		"other symbol":       {"symbol": "OTHER"},
		"old parser":         {"version": "sec-annual-v1"},
		"other data kind":    {"data_kind": "LICENSED_PRICES"},
		"substituted source": {"sources": []map[string]string{{"kind": "submissions", "sha256": sub}, {"kind": "companyfacts", "sha256": sub}}},
		"missing sources":    {"sources": nil},
	} {
		if err := checkReport(encode(change), fixtureJob(), sub, facts); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if checkReport([]byte(`[]`), fixtureJob(), sub, facts) == nil || checkReport([]byte(`{`), fixtureJob(), sub, facts) == nil {
		t.Fatal("non-object report accepted")
	}
}

func TestParserFailuresFailClosed(t *testing.T) {
	requirePython(t)
	for name, script := range map[string]string{
		"crash":            "raise SystemExit(3)",
		"claims entry":     `import json; print(json.dumps({"version":"sec-annual-v2","symbol":"TEST","cik":"0000000001","data_kind":"OFFICIAL_FILINGS","outcome":"INSUFFICIENT_DATA","eligible_for_entry":True,"eligible_for_backtest":False,"financial_state_write_access":False}))`,
		"oversized output": "import sys; sys.stdout.write('x' * (2 * 1024 * 1024))",
		"not json":         "print('ok')",
		// Reviewer reproduction: a report that passes every gate except size.
		"oversized valid report": gatedReportScript(2 * 1024 * 1024),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := (&Service{Script: script}).analyze(context.Background(), fixtureJob(), fixtureSources()); err == nil {
				t.Fatal("unsafe parser result accepted")
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := (&Service{Script: "import time; time.sleep(30)"}).analyze(ctx, fixtureJob(), fixtureSources()); err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("hung parser not stopped: %v after %v", err, time.Since(start))
	}
	if _, err := (&Service{}).analyze(context.Background(), fixtureJob(), fixtureSources()[:1]); err == nil {
		t.Fatal("analysis ran without both sources")
	}
}

// gatedReportScript prints a report that passes checkReport, padded to pad bytes.
func gatedReportScript(pad int) string {
	sources := fixtureSources()
	return fmt.Sprintf(`import json,sys
report={"version":"sec-annual-v2","symbol":"TEST","cik":"0000000001","data_kind":"OFFICIAL_FILINGS","outcome":"INSUFFICIENT_DATA",
"eligible_for_entry":False,"eligible_for_backtest":False,"financial_state_write_access":False,
"sources":[{"kind":"submissions","sha256":%q},{"kind":"companyfacts","sha256":%q}],"padding":"x"*%d}
sys.stdout.write(json.dumps(report))`, sources[0].SHA256, sources[1].SHA256, pad)
}

func TestParserOutputLimitsCannotBeBypassed(t *testing.T) {
	requirePython(t)
	// A small valid report is accepted, so the oversized case fails only for size.
	if _, err := (&Service{Script: gatedReportScript(16)}).analyze(context.Background(), fixtureJob(), fixtureSources()); err != nil {
		t.Fatalf("small gated report rejected: %v", err)
	}
	out, err := (&Service{Script: gatedReportScript(2 * 1024 * 1024)}).analyze(context.Background(), fixtureJob(), fixtureSources())
	if err == nil || len(out) != 0 {
		t.Fatalf("oversized report returned %d bytes, err=%v", len(out), err)
	}
	// io.Copy prefers ReaderFrom; the buffer must not expose one.
	var w io.Writer = &boundedBuffer{limit: 8}
	if _, ok := w.(io.ReaderFrom); ok {
		t.Fatal("boundedBuffer exposes ReadFrom, which bypasses its limit")
	}
	if n, err := io.Copy(w, strings.NewReader(strings.Repeat("x", 100))); err == nil || n > 8 {
		t.Fatalf("io.Copy wrote %d bytes past the limit, err=%v", n, err)
	}
	// A stderr flood stays bounded and fails closed without hanging.
	s := &Service{Script: "import sys\nsys.stderr.write('e' * (1024 * 1024))\nsys.stderr.flush()\nprint('{}')"}
	cmd := s.pythonCommand(context.Background())
	cmd.Stdin = strings.NewReader("{}")
	stdout, stderr := &boundedBuffer{limit: reportLimit}, &boundedBuffer{limit: 4096}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("stderr flood hung the parser")
	}
	if err == nil || stderr.Len() > 4096 {
		t.Fatalf("stderr flood: err=%v kept=%d bytes", err, stderr.Len())
	}
}

// Regression: RFC3339Nano gave ".74672", which Python 3.9 rejected intermittently.
func TestAsOfAlwaysHasSixFractionalDigits(t *testing.T) {
	requirePython(t)
	for _, nanos := range []int{0, 7e8, 746720000, 123456789, 999999999} {
		at := time.Date(2026, 10, 23, 9, 11, 24, nanos, time.FixedZone("AST", 3*3600))
		text := asOfText(at)
		if len(text) != len("2026-10-23T06:11:24.000000Z") || !strings.HasPrefix(text, "2026-10-23T06:11:24.") || !strings.HasSuffix(text, "Z") {
			t.Fatalf("as_of %q", text)
		}
		check := exec.Command(defaultPython, "-I", "-c", "import datetime,sys; datetime.datetime.fromisoformat(sys.argv[1].replace('Z','+00:00'))", text)
		if out, err := check.CombinedOutput(); err != nil {
			t.Fatalf("Python rejected %q: %s", text, out)
		}
	}
}

func TestRetryDelays(t *testing.T) {
	if retryDelay("SEC_REQUEST_FAILED", 1) != 30*time.Second || retryDelay("SEC_HTTP_ERROR", 2) != 2*time.Minute || retryDelay("SEC_ACCESS_LIMITED", 1) != 10*time.Minute {
		t.Fatal("unexpected retry schedule")
	}
}

func TestCheckPythonAcceptsOnlyThePinnedRelease(t *testing.T) {
	requirePython(t)
	if got, err := CheckPython(context.Background(), defaultPython); err != nil || got != "CPython "+researchworker.PythonVersion {
		t.Fatalf("pinned interpreter: %q, %v", got, err)
	}
	dir := t.TempDir()
	for name, output := range map[string]string{
		"other release":        "CPython 3.9.6",
		"other implementation": "PyPy " + researchworker.PythonVersion,
		"extra text":           "CPython " + researchworker.PythonVersion + " debug",
	} {
		fake := dir + "/" + strings.ReplaceAll(name, " ", "-")
		if err := os.WriteFile(fake, []byte("#!/bin/sh\necho '"+output+"'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		if got, err := CheckPython(context.Background(), fake); err == nil {
			t.Fatalf("%s accepted as %q", name, got)
		}
	}
	if _, err := CheckPython(context.Background(), dir+"/missing"); err == nil {
		t.Fatal("missing interpreter accepted")
	}
}

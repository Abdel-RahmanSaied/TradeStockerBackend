package contracts

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// verdict is one answer, as Python's `python -m tradestocker_research.contracts`
// prints it.
type verdict struct {
	Valid     bool   `json:"valid"`
	Code      string `json:"code"`
	Keyword   string `json:"keyword"`
	Path      string `json:"path"`
	Canonical string `json:"canonical_b64"`
}

func goVerdict(r *Registry, raw []byte) verdict {
	v, err := CheckEvent(r, raw)
	if err != nil {
		var e *EventError
		if !errors.As(err, &e) {
			return verdict{Code: "GO_ERROR " + err.Error()}
		}
		out := verdict{Code: e.Code}
		if e.Violation != nil {
			out.Keyword, out.Path = e.Violation.Keyword, e.Violation.Path
		}
		return out
	}
	out := verdict{Valid: true}
	if canon, err := Canonical(v); err == nil {
		out.Canonical = base64.StdEncoding.EncodeToString(canon)
	}
	return out
}

// contractsPython is the research package's interpreter, where
// tradestocker_research is installed (scripts/toolchain.sh).
func contractsPython(t *testing.T) string {
	t.Helper()
	python, _ := filepath.Abs("../../.tools/research-venv/bin/python")
	if _, err := os.Stat(python); err != nil {
		if os.Getenv("TS_REQUIRE_PYTHON") == "1" {
			t.Fatal("research package interpreter missing; run scripts/toolchain.sh")
		}
		t.Skip("research package interpreter missing; run scripts/toolchain.sh")
	}
	return python
}

// mutations builds inputs from every golden event: each field removed or
// replaced by a value of another kind or shape, plus byte-level damage. They
// are generated here, not listed with answers; only agreement is checked.
func mutations(t *testing.T) [][]byte {
	t.Helper()
	replacements := []string{
		`null`, `""`, `" "`, `"x"`, `0`, `-1`, `1.5`, `1e3`, `12345678901234567890123`, `true`, `[]`, `{}`,
		`"` + strings.Repeat("a", 200) + `"`, `"0"`, `"0.000000000000000001"`, `"0.0000000000000000001"`, `"-0.5"`,
		`"2024-02-29T00:00:00Z"`, `"2025-02-29T00:00:00Z"`, `"2026-10-23T09:00:00+00:00"`, `"2026-10-23T09:00:00.1234567890Z"`,
		`"\u00e9"`, `"\ud83d\ude00"`, `"ACTUAL"`, `"US_SWING"`, `"UNKNOWN_VALUE"`, `9007199254740992`, `["A"]`, `["a"]`,
	}
	var out [][]byte
	golden := goldenEvents(t)
	names := make([]string, 0, len(golden))
	for n := range golden {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := golden[name]
		out = append(out, raw)
		doc, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		obj := doc.(map[string]any)
		data := obj["data"].(map[string]any)
		paths := [][]string{}
		for k := range obj {
			paths = append(paths, []string{k})
		}
		for k := range data {
			paths = append(paths, []string{"data", k})
		}
		sort.Slice(paths, func(i, j int) bool { return strings.Join(paths[i], "/") < strings.Join(paths[j], "/") })
		for _, p := range paths {
			out = append(out, withField(t, obj, p, ""))
			for _, rep := range replacements {
				out = append(out, withField(t, obj, p, rep))
			}
		}
		for cut := 1; cut < len(raw); cut += len(raw)/37 + 1 {
			out = append(out, raw[:cut])
		}
		out = append(out, append(append([]byte{}, raw...), ' ', '{', '}'))
		out = append(out, bytes.Replace(raw, []byte(`"mode":"PAPER"`), []byte(`"mode":"PAPER","mode":"PAPER"`), 1))
	}
	return out
}

// withField returns obj with the member at p removed (rep "") or replaced by
// the raw JSON text rep, written without canonicalizing it.
func withField(t *testing.T, obj map[string]any, p []string, rep string) []byte {
	t.Helper()
	marker := "\x00MARK\x00"
	clone := deepCopy(obj).(map[string]any)
	target := clone
	for _, k := range p[:len(p)-1] {
		target = target[k].(map[string]any)
	}
	if rep == "" {
		delete(target, p[len(p)-1])
	} else {
		target[p[len(p)-1]] = marker
	}
	canon, err := Canonical(clone)
	if err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(marker)
	return bytes.Replace(canon, []byte(strings.ReplaceAll(string(quoted), `\u0000`, `\u0000`)), []byte(rep), 1)
}

func deepCopy(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = deepCopy(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = deepCopy(e)
		}
		return out
	}
	return v
}

func TestGoAndPythonAgreeOnGeneratedInputs(t *testing.T) {
	python := contractsPython(t)
	r := eventRegistry(t)
	inputs := mutations(t)
	var stdin bytes.Buffer
	for _, raw := range inputs {
		line, _ := json.Marshal(map[string]any{"event": true, "payload_b64": base64.StdEncoding.EncodeToString(raw)})
		stdin.Write(append(line, '\n'))
	}
	cmd := exec.Command(python, "-I", "-B", "-m", "tradestocker_research.contracts")
	cmd.Stdin = &stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("python: %v\n%s", err, stderr.String())
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	i, valid, kinds := 0, 0, map[string]int{}
	for scanner.Scan() {
		var py verdict
		if err := json.Unmarshal(scanner.Bytes(), &py); err != nil {
			t.Fatal(err)
		}
		if i >= len(inputs) {
			t.Fatal("python returned more answers than inputs")
		}
		goV := goVerdict(r, inputs[i])
		if goV != py {
			t.Errorf("input %d %q:\n  go     %+v\n  python %+v", i, inputs[i], goV, py)
		}
		if goV.Valid {
			valid++
		}
		kinds[goV.Code+" "+goV.Keyword]++
		i++
	}
	if i != len(inputs) {
		t.Fatalf("python answered %d of %d inputs", i, len(inputs))
	}
	if valid == 0 || len(kinds) < 10 {
		t.Fatalf("inputs too uniform: %d valid, kinds %v", valid, kinds)
	}
	t.Logf("%d generated inputs: %d valid; outcomes %v", len(inputs), valid, kinds)
}

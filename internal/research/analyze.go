package research

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	researchworker "tradestocker/research"
)

const ParserVersion = "sec-annual-v2"
const reportLimit = 1 << 20

// boundedBuffer keeps at most limit bytes. The buffer is a named field, not
// embedded: an embedded bytes.Buffer would promote ReadFrom, and io.Copy (used by
// os/exec for pipes) would call it and bypass this Write limit.
type boundedBuffer struct {
	buf   bytes.Buffer
	limit int
}

var errOutputLimit = errors.New("output limit exceeded")

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len()+len(p) > b.limit {
		return 0, errOutputLimit
	}
	return b.buf.Write(p)
}
func (b *boundedBuffer) Bytes() []byte  { return b.buf.Bytes() }
func (b *boundedBuffer) String() string { return b.buf.String() }
func (b *boundedBuffer) Len() int       { return b.buf.Len() }

// defaultPython is used when Service.Python is empty. platform-core always sets
// Python explicitly after CheckPython.
var defaultPython = "python3"

// isolatedPython runs python isolated (-I) with a fixed environment, so no
// database URL, API token or SEC contact from this process reaches Python.
func isolatedPython(ctx context.Context, python, script string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, python, "-I", "-c", script)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/local/bin", "LANG=C.UTF-8"}
	cmd.WaitDelay = 2 * time.Second
	return cmd
}

func (s *Service) pythonCommand(ctx context.Context) *exec.Cmd {
	python, script := s.Python, s.Script
	if python == "" {
		python = defaultPython
	}
	if script == "" {
		script = researchworker.Financials
	}
	return isolatedPython(ctx, python, script)
}

// CheckPython confirms python is exactly the CPython release the research
// package is tested on (research/.python-version) and returns what it found.
func CheckPython(ctx context.Context, python string) (string, error) {
	out, err := isolatedPython(ctx, python, `import platform,sys; print(platform.python_implementation(), "%d.%d.%d" % sys.version_info[:3])`).Output()
	if err != nil {
		return "", fmt.Errorf("research Python %q did not run: %w", python, err)
	}
	got := strings.TrimSpace(string(out))
	if want := "CPython " + researchworker.PythonVersion; got != want {
		return got, fmt.Errorf("research Python %q is %q, want %q: run scripts/toolchain.sh and set TS_PYTHON", python, got, want)
	}
	return got, nil
}

func (s *Service) analyze(ctx context.Context, j Job, sources []Source) ([]byte, error) {
	byKind := map[string]Source{}
	for _, src := range sources {
		byKind[src.Kind] = src
	}
	sub, okSub := byKind["submissions"]
	facts, okFacts := byKind["companyfacts"]
	if !okSub || !okFacts || len(sources) != 2 {
		return nil, errors.New("submissions and companyfacts sources required")
	}
	body, err := json.Marshal(map[string]any{
		"symbol": j.Symbol, "cik": j.CIK, "as_of": asOfText(time.Now()),
		"submissions": sub.Body, "companyfacts": facts.Body, "submissions_sha256": sub.SHA256, "facts_sha256": facts.SHA256,
	})
	if err != nil {
		return nil, err
	}
	cmd := s.pythonCommand(ctx)
	cmd.Stdin = bytes.NewReader(body)
	out := &boundedBuffer{limit: reportLimit}
	stderr := &boundedBuffer{limit: 4096}
	cmd.Stdout, cmd.Stderr = out, stderr
	if err = cmd.Run(); err != nil {
		slog.Warn("research parser failed", "job", j.ID, "error", err, "stderr", stderr.String())
		return nil, err
	}
	// Explicit gate as well, in case a future writer path skips Write.
	if out.Len() > reportLimit {
		return nil, errOutputLimit
	}
	if err = checkReport(out.Bytes(), j, sub.SHA256, facts.SHA256); err != nil {
		slog.Warn("research report rejected by output gate", "job", j.ID, "error", err)
		return nil, err
	}
	return out.Bytes(), nil
}

// asOfText always uses six fractional digits. RFC3339Nano trims trailing zeros
// (".74672"), which Python 3.9 datetime.fromisoformat rejects.
func asOfText(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000Z07:00") }

// checkReport is the Go-side gate: whatever Python returns, a stored report must
// describe the requested issuer and its stored sources, and must never claim
// entry, backtest or financial-state authority.
func checkReport(raw []byte, j Job, subSHA, factsSHA string) error {
	var r struct {
		Version  string `json:"version"`
		Symbol   string `json:"symbol"`
		CIK      string `json:"cik"`
		DataKind string `json:"data_kind"`
		Outcome  string `json:"outcome"`
		Entry    *bool  `json:"eligible_for_entry"`
		Backtest *bool  `json:"eligible_for_backtest"`
		Writes   *bool  `json:"financial_state_write_access"`
		Sources  []struct {
			Kind   string `json:"kind"`
			SHA256 string `json:"sha256"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("report is not a JSON object: %w", err)
	}
	switch {
	case r.Version != ParserVersion:
		return fmt.Errorf("unexpected parser version %q", r.Version)
	case r.Symbol != j.Symbol || r.CIK != j.CIK:
		return errors.New("report issuer differs from the job")
	case r.DataKind != "OFFICIAL_FILINGS" || r.Outcome != "INSUFFICIENT_DATA":
		return errors.New("report claims an unsupported data kind or outcome")
	case r.Entry == nil || *r.Entry || r.Backtest == nil || *r.Backtest || r.Writes == nil || *r.Writes:
		return errors.New("report claims entry, backtest or write authority")
	case len(r.Sources) != 2 || r.Sources[0].Kind != "submissions" || r.Sources[0].SHA256 != subSHA || r.Sources[1].Kind != "companyfacts" || r.Sources[1].SHA256 != factsSHA:
		return errors.New("report sources differ from the stored originals")
	}
	return nil
}

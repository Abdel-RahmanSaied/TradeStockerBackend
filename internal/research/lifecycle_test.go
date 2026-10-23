package research

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"tradestocker/internal/telemetry"
	researchworker "tradestocker/research"
)

// lockedBuffer collects log lines written by concurrent workers.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// captureLogs sends the default logger (used by the worker) to a buffer
// through the same redacting handler platform-core uses.
func captureLogs(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	old := slog.Default()
	slog.SetDefault(telemetry.NewLogger(buf, &telemetry.Redactor{}, slog.LevelDebug))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

func logLines(t *testing.T, text string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON: %s", line)
		}
		out = append(out, m)
	}
	return out
}

func TestWorkersAreBounded(t *testing.T) {
	for in, want := range map[int]int{-1: 1, 0: 1, 1: 1, 3: 3, 4: 4, 9: 4} {
		if got := (&Service{Workers: in}).workers(); got != want {
			t.Errorf("Workers=%d gives %d, want %d", in, got, want)
		}
	}
}

// slowFetcher is the slow-provider fixture: each fetch takes delay (or until
// cancelled) and the most fetches ever in flight at once is recorded.
type slowFetcher struct {
	delay time.Duration
	mu    sync.Mutex
	now   int
	max   int
	calls int
}

func (f *slowFetcher) Fetch(ctx context.Context, symbol, cik string) ([]Source, error) {
	f.mu.Lock()
	f.calls++
	f.now++
	if f.now > f.max {
		f.max = f.now
	}
	delay := f.delay
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.now--
		f.mu.Unlock()
	}()
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return fixtureSources(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *slowFetcher) stats() (inFlight, max, calls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now, f.max, f.calls
}

func statuses(t *testing.T, s *Service) map[string]int {
	t.Helper()
	rows, err := s.Pool.Query(context.Background(), `SELECT status,count(*) FROM research_jobs GROUP BY status`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err = rows.Scan(&status, &n); err != nil {
			t.Fatal(err)
		}
		out[status] = n
	}
	return out
}

// Validation: bounded concurrency with a slow-provider fixture, then a stop
// while every worker is busy drains (releases) the work instead of losing it.
func TestWorkerPoolBoundsConcurrencyAndDrainsOnStop(t *testing.T) {
	requirePython(t)
	f := &slowFetcher{delay: 300 * time.Millisecond}
	s := integrationService(t, f)
	s.Workers = 2
	for i := 0; i < 6; i++ {
		createJob(t, s, fmt.Sprintf("pool-bound-%d", i))
	}
	ctx, stop := context.WithCancel(context.Background())
	leader := make(chan error, 1)
	go func() { leader <- s.runLeader(ctx) }()
	// Stop and join the leader on every exit, including an early t.Fatal. This
	// cleanup runs before the pool closes (cleanups run in reverse order), so
	// the leader never still holds a connection then. The wait is bounded and
	// shared with the explicit join below.
	var joinOnce sync.Once
	var joinErr error
	join := func() error {
		joinOnce.Do(func() {
			stop()
			select {
			case joinErr = <-leader:
			case <-time.After(10 * time.Second):
				joinErr = errors.New("leader did not stop within 10s")
			}
		})
		return joinErr
	}
	t.Cleanup(func() {
		if err := join(); err != nil {
			t.Error(err)
		}
	})
	for deadline := time.Now().Add(30 * time.Second); statuses(t, s)["SUCCEEDED"] < 6; {
		if time.Now().After(deadline) {
			t.Fatalf("jobs not finished: %v", statuses(t, s))
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, most, _ := f.stats(); most != 2 {
		t.Fatalf("at most %d fetches ran at once, want exactly the pool size 2", most)
	}

	// Stop while both workers wait on the slow provider.
	f.mu.Lock()
	f.delay = time.Minute
	f.mu.Unlock()
	createJob(t, s, "pool-drain-a")
	createJob(t, s, "pool-drain-b")
	for deadline := time.Now().Add(10 * time.Second); ; {
		if inFlight, _, _ := f.stats(); inFlight == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("workers never picked up the drain jobs")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopped := time.Now()
	if err := join(); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(stopped); d > 5*time.Second {
		t.Fatalf("stop took %v", d)
	}
	if got := statuses(t, s); got["RUNNING"] != 0 || got["QUEUED"] != 2 || got["SUCCEEDED"] != 6 {
		t.Fatalf("after stop: %v", got)
	}
	var interrupted int
	if err := s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM research_events WHERE action='ATTEMPT_INTERRUPTED'`).Scan(&interrupted); err != nil || interrupted != 2 {
		t.Fatalf("interrupted events %d %v", interrupted, err)
	}
}

// Reviewer race path: a stop that arrives after the parser produced a valid
// report must not leave the job RUNNING until its lease expires.
func TestStopAfterAValidReportStillSettles(t *testing.T) {
	requirePython(t)
	s := integrationService(t, &scriptedFetcher{})
	j := createJob(t, s, "stop-after-report")
	ctx, cancel := context.WithCancel(context.Background())
	s.afterAnalysis = cancel // the stop lands between analysis and settlement
	if processed, err := s.ProcessOne(ctx); err != nil || !processed {
		t.Fatalf("process: %v %v", processed, err)
	}
	done := loadJob(t, s, j.ID)
	if done.Status != "SUCCEEDED" || done.Attempts != 1 || done.Result == nil {
		t.Fatalf("after stop %+v", done)
	}
	if got := strings.Join(actions(t, s, j.ID), ","); !strings.HasSuffix(got, "SEC_SOURCES_STORED,SUCCEEDED") {
		t.Fatalf("events %s", got)
	}
	// The fallback release, as after a commit whose outcome was unknown, is
	// fenced: once the success is written it changes nothing.
	if err := s.release(Job{ID: j.ID, Attempts: 1}); !errors.Is(err, errLeaseLost) {
		t.Fatalf("release after success: %v", err)
	}
	if loadJob(t, s, j.ID).Status != "SUCCEEDED" {
		t.Fatal("fallback changed a settled job")
	}

	// A stop after a failed analysis requeues the attempt instead.
	s.Script = "raise SystemExit(4)"
	k := createJob(t, s, "stop-after-failed-analysis")
	ctx, cancel = context.WithCancel(context.Background())
	s.afterAnalysis = cancel
	if _, err := s.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	if again := loadJob(t, s, k.ID); again.Status != "QUEUED" || again.LastError == nil || *again.LastError != "WORKER_STOPPED" {
		t.Fatalf("failed analysis during stop %+v", again)
	}
}

// Trace continuity: HTTP request -> stored job -> worker attempt logs.
func TestJobContinuesTheRequestTrace(t *testing.T) {
	requirePython(t)
	logs := captureLogs(t)
	s := integrationService(t, &scriptedFetcher{})
	h := telemetry.HTTP(s.Handler(), slog.Default())
	parent := telemetry.NewTrace()
	send := func(traceparent string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/research/jobs", strings.NewReader(fixtureRequest))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "trace-continues")
		r.Header.Set("traceparent", traceparent)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := send(parent.Traceparent())
	j := decode[Job](t, w)
	if j.TraceID == nil || *j.TraceID != parent.TraceID {
		t.Fatalf("job trace %v, want %s", j.TraceID, parent.TraceID)
	}
	if back, ok := telemetry.ParseTraceparent(w.Header().Get("traceparent")); !ok || back.TraceID != parent.TraceID {
		t.Fatalf("response traceparent %q", w.Header().Get("traceparent"))
	}
	// A replay in another trace returns the job without changing its trace.
	if replay := decode[Job](t, send(telemetry.NewTrace().Traceparent())); replay.TraceID == nil || *replay.TraceID != parent.TraceID {
		t.Fatalf("replay changed the trace: %v", replay.TraceID)
	}
	processOnce(t, s)
	seen := map[string]bool{}
	for _, line := range logLines(t, logs.String()) {
		if line["job"] != j.ID {
			continue
		}
		if line["trace_id"] != parent.TraceID {
			t.Fatalf("job log line outside the request trace: %v", line)
		}
		seen[line["msg"].(string)] = true
	}
	for _, msg := range []string{"research job created", "research attempt started", "research attempt ended"} {
		if !seen[msg] {
			t.Fatalf("missing %q in the trace; saw %v", msg, seen)
		}
	}
	var detail map[string]string
	if err := s.Pool.QueryRow(context.Background(), `SELECT detail FROM research_events WHERE job_id=$1 AND action='REQUESTED'`, j.ID).Scan(&detail); err != nil || detail["trace_id"] != parent.TraceID {
		t.Fatalf("REQUESTED event %v %v", detail, err)
	}
}

func TestParserProcessReceivesTheTrace(t *testing.T) {
	requirePython(t)
	parent := telemetry.NewTrace()
	ctx := telemetry.WithTrace(context.Background(), parent)
	cmd := (&Service{Script: "import sys; print(sys.argv[1:])"}).pythonCommand(ctx)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("['%s']", parent.Traceparent()); strings.TrimSpace(string(out)) != want {
		t.Fatalf("parser arguments %q, want %s", out, want)
	}
	if out, err = (&Service{Script: "import sys; print(len(sys.argv))"}).pythonCommand(context.Background()).Output(); err != nil || strings.TrimSpace(string(out)) != "1" {
		t.Fatalf("untraced parser arguments %q %v", out, err)
	}
}

// parserInput is the exact stdin analyze gives the parser for the fixture job.
func parserInput(t *testing.T) []byte {
	t.Helper()
	sources := fixtureSources()
	body, err := json.Marshal(map[string]any{"symbol": fixtureSymbol, "cik": fixtureCIK, "as_of": asOfText(time.Now()),
		"submissions": sources[0].Body, "companyfacts": sources[1].Body, "submissions_sha256": sources[0].SHA256, "facts_sha256": sources[1].SHA256})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// Reviewer finding: the parser path Go actually runs logs a safe event in the
// job's trace, and its report bytes stay exactly those of financials.py alone.
func TestLauncherKeepsTheReportBytesAndLogsInTheJobTrace(t *testing.T) {
	requirePython(t)
	parent := telemetry.NewTrace()
	ctx := telemetry.WithTrace(context.Background(), parent)
	input := parserInput(t)
	run := func(cmd *exec.Cmd) ([]byte, []byte) {
		var stdout, stderr bytes.Buffer
		cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(input), &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%v: %s", err, stderr.String())
		}
		return stdout.Bytes(), stderr.Bytes()
	}
	alone, aloneErr := run(isolatedPython(ctx, defaultPython, researchworker.Financials))
	launched, events := run((&Service{}).pythonCommand(ctx))
	if !bytes.Equal(alone, launched) || len(aloneErr) != 0 {
		t.Fatalf("launcher changed the report:\n%s\n%s", alone, launched)
	}
	relayed, other := parserEvents(events, parent.TraceID)
	if len(relayed) != 1 || other != 0 {
		t.Fatalf("stderr %q gave %d events and %d other bytes", events, len(relayed), other)
	}
	// The sources Python ran are byte-for-byte the embedded ones.
	check := "import hashlib\nprint(hashlib.sha256(" + pyString(researchworker.Financials) + ".encode()).hexdigest(), hashlib.sha256(" + pyString(researchworker.Telemetry) + ".encode()).hexdigest())\n"
	if !strings.HasSuffix(parserScript, "sys.exit(launch("+pyString(researchworker.Financials)+", "+pyString(researchworker.Telemetry)+"))\n") {
		t.Fatal("parser script does not end with the launch call")
	}
	out, err := isolatedPython(ctx, defaultPython, check).Output()
	if err != nil {
		t.Fatal(err)
	}
	sum := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	if want := sum(researchworker.Financials) + " " + sum(researchworker.Telemetry); strings.TrimSpace(string(out)) != want {
		t.Fatalf("Python saw sources %s, want %s", out, want)
	}

	logs := captureLogs(t)
	if _, err = (&Service{}).analyze(ctx, fixtureJob(), fixtureSources()); err != nil {
		t.Fatal(err)
	}
	lines := logLines(t, logs.String())
	if len(lines) != 1 || lines[0]["msg"] != "research parser ended" || lines[0]["trace_id"] != parent.TraceID ||
		lines[0]["parser_status"] != "ok" || lines[0]["parser_task"] != ParserVersion || lines[0]["parser_span_id"] == parent.SpanID {
		t.Fatalf("parser log %v", lines)
	}
}

// A rejected input logs the outcome in the trace and only the size of the
// parser's own error text, which can quote the input.
func TestRejectedParserInputLogsOnlyTheOutcome(t *testing.T) {
	requirePython(t)
	logs := captureLogs(t)
	parent := telemetry.NewTrace()
	ctx := telemetry.WithTrace(context.Background(), parent)
	sources := fixtureSources()
	fragment := "FixtureFragmentCompany"
	sources[0] = fixtureSource("submissions", strings.Replace(fixtureSubmissions, `"cik":"0000000001"`, `"cik":"`+fragment+`"`, 1))
	if _, err := (&Service{}).analyze(ctx, fixtureJob(), sources); err == nil {
		t.Fatal("rejected input accepted")
	}
	// The parser writes "research input rejected: ValueError: CIK must be ..."
	// on stderr for this input; none of that text may reach the log.
	text := logs.String()
	if strings.Contains(text, fragment) || strings.Contains(text, "CIK must be") || strings.Contains(text, "input rejected") {
		t.Fatalf("parser error text reached the log:\n%s", text)
	}
	var ended, failed map[string]any
	for _, line := range logLines(t, text) {
		switch line["msg"] {
		case "research parser ended":
			ended = line
		case "research parser failed":
			failed = line
		}
	}
	if ended == nil || ended["parser_status"] != "REJECTED" || ended["trace_id"] != parent.TraceID {
		t.Fatalf("outcome event %v", ended)
	}
	if failed == nil || failed["stderr_omitted_bytes"].(float64) <= 0 || failed["trace_id"] != parent.TraceID {
		t.Fatalf("failure line %v", failed)
	}
}

// Only the exact event shape in the job's trace is relayed; anything else on
// stderr, including look-alike events, counts as omitted bytes.
func TestOnlyTheLaunchersEventIsRelayed(t *testing.T) {
	parent := telemetry.NewTrace()
	good := `{"time":"2026-10-23T12:00:00.000+00:00","level":"INFO","msg":"research parser ended","trace_id":"` + parent.TraceID + `","span_id":"0123456789abcdef","task":"sec-annual-v2","status":"ok","duration_ms":3}`
	if events, other := parserEvents([]byte(good+"\n"), parent.TraceID); len(events) != 1 || other != 0 {
		t.Fatalf("good event: %d %d", len(events), other)
	}
	for name, line := range map[string]string{
		"extra field":     strings.Replace(good, `"duration_ms":3`, `"duration_ms":3,"input":"fixture"`, 1),
		"other trace":     strings.Replace(good, parent.TraceID, telemetry.NewTrace().TraceID, 1),
		"text in a name":  strings.Replace(good, `"task":"sec-annual-v2"`, `"task":"fixture filing text"`, 1),
		"other message":   strings.Replace(good, "research parser ended", "fixture filing text", 1),
		"other status":    strings.Replace(good, `"status":"ok"`, `"status":"fixture"`, 1),
		"no duration":     strings.Replace(good, `,"duration_ms":3`, "", 1),
		"trailing data":   good + ` {}`,
		"plain text":      "research input rejected: ValueError: fixture filing text",
		"oversized":       strings.Replace(good, `"task":"sec-annual-v2"`, `"task":"`+strings.Repeat("a", 2000)+`"`, 1),
		"negative number": strings.Replace(good, `"duration_ms":3`, `"duration_ms":-1`, 1),
		// Short single words pass a name pattern, so the task must be this parser.
		"one-word task":   strings.Replace(good, `"task":"sec-annual-v2"`, `"task":"fixture"`, 1),
		"older parser":    strings.Replace(good, `"task":"sec-annual-v2"`, `"task":"sec-annual-v1"`, 1),
		"zero span":       strings.Replace(good, `"span_id":"0123456789abcdef"`, `"span_id":"0000000000000000"`, 1),
		"upper-case span": strings.Replace(good, `"span_id":"0123456789abcdef"`, `"span_id":"0123456789ABCDEF"`, 1),
	} {
		events, other := parserEvents([]byte(line+"\n"), parent.TraceID)
		if len(events) != 0 || other != len(line)+1 {
			t.Errorf("%s: relayed %d events, %d other bytes", name, len(events), other)
		}
	}
	if events, _ := parserEvents([]byte(good+"\n"), ""); len(events) != 0 {
		t.Fatal("event relayed without a job trace")
	}
}

// Reviewer finding: parser stderr is logged by size only, in the job's trace,
// and output-gate errors never quote the report.
func TestParserFailureLogsNoOutputContent(t *testing.T) {
	requirePython(t)
	logs := captureLogs(t)
	fragment := "fixture-filing-fragment"
	ctx := telemetry.WithTrace(context.Background(), telemetry.NewTrace())
	trace, _ := telemetry.FromContext(ctx)
	failing := &Service{Script: "import sys; sys.stderr.write('" + fragment + "'); raise SystemExit(5)"}
	if _, err := failing.analyze(ctx, fixtureJob(), fixtureSources()); err == nil {
		t.Fatal("failing parser accepted")
	}
	echoing := &Service{Script: `print('{"version":"` + fragment + `"}')`}
	_, err := echoing.analyze(ctx, fixtureJob(), fixtureSources())
	if err == nil || strings.Contains(err.Error(), fragment) {
		t.Fatalf("gate error %v", err)
	}
	notJSON := &Service{Script: `print('` + fragment + `')`}
	if _, err = notJSON.analyze(ctx, fixtureJob(), fixtureSources()); err == nil || strings.Contains(err.Error(), fragment) || strings.Contains(err.Error(), "'f'") {
		t.Fatalf("gate error %v", err)
	}
	text := logs.String()
	if strings.Contains(text, fragment) {
		t.Fatalf("parser output reached the log:\n%s", text)
	}
	lines := logLines(t, text)
	if len(lines) != 3 {
		t.Fatalf("want 3 warnings, got:\n%s", text)
	}
	if lines[0]["stderr_omitted_bytes"] != float64(len(fragment)) {
		t.Fatalf("stderr logged as %v", lines[0])
	}
	for _, line := range lines {
		if line["trace_id"] != trace.TraceID {
			t.Fatalf("warning outside the job trace: %v", line)
		}
	}
}

// The SEC limiter is shared: workers that fetch at the same time still start
// requests at least the minimum interval apart, one request at a time.
func TestSECSpacingHoldsAcrossConcurrentWorkers(t *testing.T) {
	timing := &timingTransport{delay: 15 * time.Millisecond}
	s := newSEC("TradeStocker Research test@example.com", timing)
	s.minInterval = 40 * time.Millisecond
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Fetch(context.Background(), "AAPL", "0000320193"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	starts, most := timing.result()
	if len(starts) != 6 || most != 1 {
		t.Fatalf("%d requests, %d at once", len(starts), most)
	}
	for i := 1; i < len(starts); i++ {
		// An allowance for timing noise under -race, still well above the 15ms a
		// merely serialized (unspaced) client would show.
		if gap := starts[i].Sub(starts[i-1]); gap < 30*time.Millisecond {
			t.Fatalf("requests %d and %d started %v apart", i-1, i, gap)
		}
	}
}

type timingTransport struct {
	delay  time.Duration
	mu     sync.Mutex
	starts []time.Time
	now    int
	most   int
}

func (f *timingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.starts = append(f.starts, time.Now())
	f.now++
	if f.now > f.most {
		f.most = f.now
	}
	f.mu.Unlock()
	time.Sleep(f.delay)
	f.mu.Lock()
	f.now--
	f.mu.Unlock()
	return respond200(testFacts)(r)
}

func (f *timingTransport) result() ([]time.Time, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.starts...), f.most
}

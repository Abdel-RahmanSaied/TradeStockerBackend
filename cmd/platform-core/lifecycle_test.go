package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"

	"tradestocker/internal/research"
	"tradestocker/internal/telemetry"
)

// These tests run the real serve wiring: HTTP over TCP, a disposable
// PostgreSQL database (TEST_DATABASE_URL, tradestocker_test_* only), NATS
// inside the test process and the pinned research Python. SEC is replaced by
// synthetic sources; no external service is contacted.

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL absent; run scripts/test-isolated.sh")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(u.Path, "/tradestocker_test_") {
		t.Fatal("only explicitly disposable tradestocker_test_* databases permitted")
	}
	return raw
}

func testPython(t *testing.T) (string, string) {
	t.Helper()
	python := os.Getenv("TS_PYTHON")
	version, err := research.CheckPython(context.Background(), python)
	if err != nil {
		if os.Getenv("TS_REQUIRE_PYTHON") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	return python, version
}

func randomText(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

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

// syntheticSEC returns a fixed synthetic issuer (not a real filing). It never
// contacts SEC.
type syntheticSEC struct{}

const syntheticSubmissions = `{"cik":"0000000001","name":"Synthetic Test Issuer","tickers":["TEST"],"sic":"3571","filings":{"recent":{
"accessionNumber":["0000000001-25-000020"],"form":["10-K"],"filingDate":["2025-10-31"],"reportDate":["2025-09-27"],"acceptanceDateTime":["2025-10-31T14:01:26.000Z"]}}}`

const syntheticFacts = `{"cik":1,"entityName":"Synthetic Test Issuer","facts":{"us-gaap":{
"Assets":{"units":{"USD":[{"end":"2025-09-27","val":5500,"accn":"0000000001-25-000020","form":"10-K","filed":"2025-10-31"}]}}}}}`

func (syntheticSEC) Fetch(ctx context.Context, symbol, cik string) ([]research.Source, error) {
	source := func(kind, path, body string) research.Source {
		sum := sha256.Sum256([]byte(body))
		return research.Source{Kind: kind, URL: "https://data.sec.gov/" + path + "/CIK" + cik + ".json", Body: []byte(body),
			Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), ReceivedAt: time.Now().UTC()}
	}
	return []research.Source{source("submissions", "submissions", syntheticSubmissions), source("companyfacts", "api/xbrl/companyfacts", syntheticFacts)}, nil
}

// startNATS runs NATS inside the test process on 127.0.0.1 (port 0 picks a
// free one). Shutdown is the NATS outage.
func startNATS(t *testing.T, port int) *server.Server {
	t.Helper()
	if port == 0 {
		port = server.RANDOM_PORT
	}
	s, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: port, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("in-process NATS did not start")
	}
	t.Cleanup(s.Shutdown)
	return s
}

// relay forwards TCP from a local port to the test database. Cutting it is
// the database outage: it closes only the connections it relayed itself and
// refuses new ones until restored. The database is never touched.
type relay struct {
	target string
	ln     net.Listener
	mu     sync.Mutex
	conns  map[net.Conn]bool
	down   bool
}

func startRelay(t *testing.T, target string) *relay {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r := &relay{target: target, ln: ln, conns: map[net.Conn]bool{}}
	t.Cleanup(func() { _ = ln.Close(); r.cut() })
	go func() {
		for {
			client, err := ln.Accept()
			if err != nil {
				return
			}
			go r.serve(client)
		}
	}()
	return r
}

func (r *relay) serve(client net.Conn) {
	r.mu.Lock()
	down := r.down
	r.mu.Unlock()
	if down {
		_ = client.Close()
		return
	}
	upstream, err := net.Dial("tcp", r.target)
	if err != nil {
		_ = client.Close()
		return
	}
	// A cut may have happened while dialing: check again before tracking, so no
	// connection stays live during an outage.
	r.mu.Lock()
	if r.down {
		r.mu.Unlock()
		_ = upstream.Close()
		_ = client.Close()
		return
	}
	r.conns[client], r.conns[upstream] = true, true
	r.mu.Unlock()
	go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
}

func (r *relay) cut() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.down = true
	for c := range r.conns {
		_ = c.Close()
	}
	r.conns = map[net.Conn]bool{}
}

func (r *relay) restore() {
	r.mu.Lock()
	r.down = false
	r.mu.Unlock()
}

type app struct {
	t      *testing.T
	base   string
	token  string
	client *http.Client
}

func (a *app) do(method, path, key, traceparent, body string) (*http.Response, []byte) {
	a.t.Helper()
	req, err := http.NewRequest(method, a.base+path, strings.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if traceparent != "" {
		req.Header.Set("traceparent", traceparent)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp, out
}

func (a *app) readiness() (int, string, map[string]any) {
	a.t.Helper()
	resp, body := a.do(http.MethodGet, "/api/v1/ready", "", "", "")
	var v map[string]any
	_ = json.Unmarshal(body, &v)
	status, _ := v["status"].(string)
	return resp.StatusCode, status, v
}

func (a *app) waitReady(want string) map[string]any {
	a.t.Helper()
	for deadline := time.Now().Add(15 * time.Second); ; {
		_, status, v := a.readiness()
		if status == want {
			return v
		}
		if time.Now().After(deadline) {
			a.t.Fatalf("readiness stayed %v, want %s", v, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (a *app) waitJob(id, status string) {
	a.t.Helper()
	for deadline := time.Now().Add(20 * time.Second); ; {
		_, body := a.do(http.MethodGet, "/api/v1/research/jobs/"+id, "", "", "")
		var v struct {
			Job struct{ Status string } `json:"job"`
		}
		_ = json.Unmarshal(body, &v)
		if v.Job.Status == status {
			return
		}
		if time.Now().After(deadline) {
			a.t.Fatalf("job %s is %s, want %s", id, v.Job.Status, status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Acceptance: one trace crosses HTTP, PostgreSQL and NATS to the worker;
// liveness differs from readiness; NATS and database outages change readiness
// without leaking secrets; a NATS outage never fails a committed job; stop is
// graceful.
func TestTraceCrossesHTTPAndNATSAndOutagesChangeReadiness(t *testing.T) {
	database := testDatabaseURL(t)
	python, version := testPython(t)
	dbURL, _ := url.Parse(database)
	password, _ := dbURL.User.Password()
	pg := startRelay(t, dbURL.Host)
	dbURL.Host = pg.ln.Addr().String()
	ns := startNATS(t, 0)
	natsPort := ns.Addr().(*net.TCPAddr).Port

	token := randomText(20)
	logs := &lockedBuffer{}
	old := slog.Default()
	slog.SetDefault(newLogger(logs, token, dbURL.String(), password))
	t.Cleanup(func() { slog.SetDefault(old) })

	cfg := config{token: token, databaseURL: dbURL.String(), bind: "127.0.0.1:0", python: python, pythonVersion: version,
		fetcher: syntheticSEC{}, researchConfigured: true, natsURL: ns.ClientURL(), workers: 2}
	ctx, stop := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	// finished is closed once serve has returned; serveErr is then its result.
	// Any number of readers may wait on it.
	finished := make(chan struct{})
	var serveErr error
	go func() { serveErr = serve(ctx, cfg, func(a string) { addr <- a }); close(finished) }()
	var stopOnce sync.Once
	var stopErr error
	shutdown := func() error {
		stopOnce.Do(func() {
			stop()
			select {
			case <-finished:
				stopErr = serveErr
			case <-time.After(30 * time.Second):
				stopErr = errors.New("serve did not stop within 30s")
			}
		})
		return stopErr
	}
	t.Cleanup(func() {
		if err := shutdown(); err != nil {
			t.Error(err)
		}
	})
	var a *app
	select {
	case got := <-addr:
		a = &app{t: t, base: "http://" + got, token: token, client: &http.Client{Timeout: 10 * time.Second}}
	case <-finished:
		t.Fatalf("serve stopped at start: %v", serveErr)
	case <-time.After(20 * time.Second):
		t.Fatal("serve did not start")
	}

	v := a.waitReady("ready")
	if checks := v["checks"].(map[string]any); checks["postgres"] != "ok" || checks["nats"] != "ok" {
		t.Fatalf("checks %v", checks)
	}

	// One request trace across HTTP -> PostgreSQL -> NATS -> worker.
	parent := telemetry.NewTrace()
	resp, body := a.do(http.MethodPost, "/api/v1/research/jobs", "e2e-trace-"+randomText(4), parent.Traceparent(), `{"symbol":"TEST","cik":"0000000001"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var job struct {
		ID      string `json:"id"`
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal(body, &job); err != nil || job.TraceID != parent.TraceID {
		t.Fatalf("job %s", body)
	}
	if back, ok := telemetry.ParseTraceparent(resp.Header.Get("traceparent")); !ok || back.TraceID != parent.TraceID {
		t.Fatalf("response traceparent %q", resp.Header.Get("traceparent"))
	}
	a.waitJob(job.ID, "SUCCEEDED")
	want := map[string]bool{"http request": false, "research job created": false, "research job notification received": false,
		"research attempt started": false, "research attempt ended": false}
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil {
			t.Fatalf("log line is not JSON: %s", line)
		}
		if m["trace_id"] != parent.TraceID {
			continue
		}
		if msg, _ := m["msg"].(string); msg != "" {
			if _, tracked := want[msg]; tracked {
				want[msg] = true
			}
		}
	}
	for msg, seen := range want {
		if !seen {
			t.Errorf("no %q log line in the request trace", msg)
		}
	}

	// NATS outage: readiness degrades (still 200: workers poll PostgreSQL),
	// a new job is still accepted and finished, and NATS recovers.
	ns.Shutdown()
	if code, _, _ := a.readiness(); code != http.StatusOK {
		t.Fatalf("NATS outage made the API unready: %d", code)
	}
	a.waitReady("degraded")
	resp, body = a.do(http.MethodPost, "/api/v1/research/jobs", "e2e-no-nats-"+randomText(4), "", `{"symbol":"TEST","cik":"0000000001"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("create during NATS outage: %d %s", resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, &job); err != nil {
		t.Fatal(err)
	}
	a.waitJob(job.ID, "SUCCEEDED")
	if !strings.Contains(logs.String(), "research job notification not sent; workers will find the job by polling") {
		t.Fatal("failed notification was not logged")
	}
	startNATS(t, natsPort)
	a.waitReady("ready")

	// Database outage: readiness turns 503, liveness stays 200, then recovers.
	pg.cut()
	v = a.waitReady("not_ready")
	if code, _, _ := a.readiness(); code != http.StatusServiceUnavailable || v["checks"].(map[string]any)["postgres"] != "unavailable" {
		t.Fatalf("database outage readiness %d %v", code, v)
	}
	if resp, _ = a.do(http.MethodGet, "/api/v1/live", "", "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("liveness followed the database: %d", resp.StatusCode)
	}
	// The worker checks its own session once a second; keep the outage until it
	// has noticed, then restore and expect it to lead again.
	leading := strings.Count(logs.String(), `"msg":"research worker leading"`)
	for deadline := time.Now().Add(5 * time.Second); !strings.Contains(logs.String(), `"msg":"research worker database unavailable"`); {
		if time.Now().After(deadline) {
			t.Fatal("the worker did not report the database outage")
		}
		time.Sleep(50 * time.Millisecond)
	}
	pg.restore()
	a.waitReady("ready")
	for deadline := time.Now().Add(10 * time.Second); strings.Count(logs.String(), `"msg":"research worker leading"`) == leading; {
		if time.Now().After(deadline) {
			t.Fatal("the worker did not lead again after the outage")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := shutdown(); err != nil {
		t.Fatal(err)
	}
	text := logs.String()
	for _, leaked := range []string{token, password} {
		if leaked != "" && strings.Contains(text, leaked) {
			t.Fatal("a secret reached the log")
		}
	}
	for _, msg := range []string{`"msg":"platform-core stopping"`, `"msg":"platform-core stopped"`, `"msg":"nats disconnected"`, `"msg":"research worker database unavailable"`} {
		if !strings.Contains(text, msg) {
			t.Errorf("missing %s in the log", msg)
		}
	}
	fmt.Fprintf(os.Stderr, "e2e: %d log lines, trace %s\n", strings.Count(text, "\n"), parent.TraceID)
}

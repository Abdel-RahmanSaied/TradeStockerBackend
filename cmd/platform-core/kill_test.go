//go:build t05kill

// Run with GOFLAGS=-tags=t05kill (it starts and signals helper processes).

package main

// Validation harness (S01-T05): a research worker process is killed (SIGKILL)
// or stopped (SIGTERM) during a synthetic job and then restarted, and a paper
// write is killed inside its open transaction and then retried.
//
// Safety: the only processes signalled are children this test starts from its
// own test binary. A child runs only when its parent PID is the one it was
// given and the database is a disposable tradestocker_test_* one. The parent
// signals a child only after the child reports its own PID and that PID equals
// the started process; signals go through Go's os.Process, which refuses a
// process that was already waited for. A cleanup kills and waits for any child
// still running. Nothing is deleted except rows in the disposable database.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"tradestocker/internal/paper"
	"tradestocker/internal/research"
	"tradestocker/internal/store"
)

const helperEnv = "TS_T05_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "":
		os.Exit(m.Run())
	case "research-worker":
		os.Exit(helperResearchWorker())
	case "paper-write":
		os.Exit(helperPaperWrite())
	default:
		fmt.Fprintln(os.Stderr, "unknown helper role")
		os.Exit(2)
	}
}

// helperDatabase returns the disposable database a helper may use, after the
// helper confirms it was started by the expected test process.
func helperDatabase() (string, error) {
	if os.Getenv("TS_T05_PARENT_PID") != strconv.Itoa(os.Getppid()) {
		return "", errors.New("helper not started by its test process")
	}
	raw := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(u.Path, "/tradestocker_test_") {
		return "", errors.New("helper needs a disposable tradestocker_test_* database")
	}
	return raw, nil
}

// blockingSEC announces the fetch, then waits until the attempt ends (stop or
// kill). It never contacts SEC.
type blockingSEC struct{}

func (blockingSEC) Fetch(ctx context.Context, symbol, cik string) ([]research.Source, error) {
	fmt.Printf("FETCHING %d\n", os.Getpid())
	<-ctx.Done()
	return nil, ctx.Err()
}

// helperResearchWorker runs the research worker loop until SIGTERM.
func helperResearchWorker() int {
	raw, err := helperDatabase()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	db, err := store.Open(ctx, raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		return 4
	}
	defer db.Close()
	lease, _ := strconv.Atoi(os.Getenv("TS_T05_LEASE_SECONDS"))
	var fetcher research.Fetcher = syntheticSEC{}
	if os.Getenv("TS_T05_FETCH") == "block" {
		fetcher = blockingSEC{}
	}
	s := &research.Service{Pool: db.Pool, Provider: fetcher, Configured: true, Python: os.Getenv("TS_PYTHON"), LeaseSeconds: lease}
	fmt.Printf("READY %d\n", os.Getpid())
	s.Run(ctx)
	return 0
}

var helperKey = regexp.MustCompile(`^t05-[a-z0-9-]{8,60}$`)

// helperPaperWrite starts one paper write and stays inside its open
// transaction (holding the paper row lock) until it is killed.
func helperPaperWrite() int {
	raw, err := helperDatabase()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}
	key := os.Getenv("TS_T05_KEY")
	if !helperKey.MatchString(key) {
		fmt.Fprintln(os.Stderr, "helper key refused")
		return 3
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		return 4
	}
	defer db.Close()
	_, err = db.Execute(ctx, key, paperHash, func(s *paper.State, now time.Time) (any, error) {
		account, err := s.CreateAccount("USD", 100000, now)
		if err != nil {
			return nil, err
		}
		fmt.Printf("IN_TRANSACTION %d\n", os.Getpid())
		time.Sleep(10 * time.Minute) // the parent kills this process here
		return account, nil
	})
	fmt.Fprintln(os.Stderr, "paper write was not killed:", err)
	return 5
}

const paperHash = "t05-financial-retry"

// child is one helper process started by this test.
type child struct {
	cmd       *exec.Cmd
	lines     chan string
	exited    chan struct{}
	waitErr   error
	stderr    lockedBuffer
	confirmed bool
}

func startChild(t *testing.T, role string, env ...string) *child {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), helperEnv+"="+role, "TS_T05_PARENT_PID="+strconv.Itoa(os.Getpid()))
	cmd.Env = append(cmd.Env, env...)
	c := &child{cmd: cmd, lines: make(chan string, 16), exited: make(chan struct{})}
	cmd.Stderr = &c.stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			c.lines <- scanner.Text()
		}
		close(c.lines)
		c.waitErr = cmd.Wait() // after all reads, as os/exec requires
		close(c.exited)
	}()
	t.Cleanup(func() {
		select {
		case <-c.exited:
		default:
			_ = cmd.Process.Kill() // this test's own child only
			select {
			case <-c.exited:
			case <-time.After(10 * time.Second):
				t.Errorf("helper %d did not exit", cmd.Process.Pid)
			}
		}
	})
	return c
}

// expect waits for the next line with this prefix and checks that the child
// reported its own PID, which confirms its identity before any signal.
func (c *child) expect(t *testing.T, prefix string) {
	t.Helper()
	timeout := time.After(30 * time.Second)
	for {
		select {
		case line, ok := <-c.lines:
			if !ok {
				t.Fatalf("helper ended before %s: %s", prefix, c.stderr.String())
			}
			fields := strings.Fields(line)
			if len(fields) != 2 || fields[0] != prefix {
				continue
			}
			if fields[1] != strconv.Itoa(c.cmd.Process.Pid) {
				t.Fatalf("helper reported PID %s, started %d; refusing to signal it", fields[1], c.cmd.Process.Pid)
			}
			c.confirmed = true
			return
		case <-timeout:
			t.Fatalf("no %s from helper: %s", prefix, c.stderr.String())
		}
	}
}

func (c *child) signal(t *testing.T, sig os.Signal) {
	t.Helper()
	if !c.confirmed {
		t.Fatal("helper identity not confirmed; refusing to signal it")
	}
	if err := c.cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
}

func (c *child) wait(t *testing.T) syscall.WaitStatus {
	t.Helper()
	select {
	case <-c.exited:
	case <-time.After(20 * time.Second):
		t.Fatal("helper did not exit")
	}
	status, _ := c.cmd.ProcessState.Sys().(syscall.WaitStatus)
	return status
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(context.Background(), testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func researchState(t *testing.T, db *store.Store, id string) (status string, attempts int, actions string) {
	t.Helper()
	ctx := context.Background()
	if err := db.Pool.QueryRow(ctx, `SELECT status,attempts FROM research_jobs WHERE id=$1`, id).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT string_agg(action, ',' ORDER BY sequence) FROM research_events WHERE job_id=$1`, id).Scan(&actions); err != nil {
		t.Fatal(err)
	}
	return status, attempts, actions
}

func createResearchJob(t *testing.T, db *store.Store, key string) string {
	t.Helper()
	h := (&research.Service{Pool: db.Pool, Provider: syntheticSEC{}, Configured: true}).Handler()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/research/jobs", strings.NewReader(`{"symbol":"TEST","cik":"0000000001"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var j struct {
		ID string `json:"id"`
	}
	if w.Code != http.StatusAccepted || json.Unmarshal(w.Body.Bytes(), &j) != nil {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	return j.ID
}

func waitResearch(t *testing.T, db *store.Store, id, want string) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); ; {
		if status, _, _ := researchState(t, db, id); status == want {
			return
		}
		if time.Now().After(deadline) {
			status, attempts, actions := researchState(t, db, id)
			t.Fatalf("job %s: %s after %d attempts (%s), want %s", id, status, attempts, actions, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func cleanResearchTables(t *testing.T, db *store.Store) {
	t.Helper()
	// TRUNCATE is allowed on the disposable database; triggers refuse row deletes.
	if _, err := db.Pool.Exec(context.Background(), `TRUNCATE research_events, research_sources, research_jobs, research_provider_state`); err != nil {
		t.Fatal(err)
	}
}

// Validation: a worker killed during a synthetic job settles nothing; a
// restarted worker recovers the job once the lease ends and finishes it.
func TestKilledWorkerIsRecoveredAfterItsLease(t *testing.T) {
	db := openTestStore(t)
	testPython(t)
	cleanResearchTables(t, db)
	id := createResearchJob(t, db, "t05-kill-"+randomText(6))

	first := startChild(t, "research-worker", "TS_T05_FETCH=block", "TS_T05_LEASE_SECONDS=3")
	first.expect(t, "READY")
	first.expect(t, "FETCHING")
	if status, attempts, _ := researchState(t, db, id); status != "RUNNING" || attempts != 1 {
		t.Fatalf("before the kill: %s %d", status, attempts)
	}
	first.signal(t, syscall.SIGKILL)
	if ws := first.wait(t); !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
		t.Fatalf("helper was not killed: %v", ws)
	}
	if status, attempts, actions := researchState(t, db, id); status != "RUNNING" || attempts != 1 || actions != "REQUESTED,ATTEMPT_STARTED" {
		t.Fatalf("after the kill: %s %d %s", status, attempts, actions)
	}

	second := startChild(t, "research-worker", "TS_T05_LEASE_SECONDS=3")
	second.expect(t, "READY")
	waitResearch(t, db, id, "SUCCEEDED")
	if _, attempts, actions := researchState(t, db, id); attempts != 2 || actions != "REQUESTED,ATTEMPT_STARTED,LEASE_RECOVERED,ATTEMPT_STARTED,SEC_SOURCES_STORED,SUCCEEDED" {
		t.Fatalf("after recovery: %d %s", attempts, actions)
	}
	second.signal(t, syscall.SIGTERM)
	if ws := second.wait(t); !ws.Exited() || ws.ExitStatus() != 0 {
		t.Fatalf("graceful stop: %v %s", ws, second.stderr.String())
	}
}

// A worker stopped with SIGTERM during a job releases it at once; a restarted
// worker finishes it without waiting for the lease.
func TestStoppedWorkerReleasesItsJobAtOnce(t *testing.T) {
	db := openTestStore(t)
	testPython(t)
	cleanResearchTables(t, db)
	id := createResearchJob(t, db, "t05-stop-"+randomText(6))

	first := startChild(t, "research-worker", "TS_T05_FETCH=block", "TS_T05_LEASE_SECONDS=120")
	first.expect(t, "READY")
	first.expect(t, "FETCHING")
	first.signal(t, syscall.SIGTERM)
	if ws := first.wait(t); !ws.Exited() || ws.ExitStatus() != 0 {
		t.Fatalf("graceful stop: %v %s", ws, first.stderr.String())
	}
	if status, attempts, actions := researchState(t, db, id); status != "QUEUED" || attempts != 1 || actions != "REQUESTED,ATTEMPT_STARTED,ATTEMPT_INTERRUPTED" {
		t.Fatalf("after the stop: %s %d %s", status, attempts, actions)
	}
	second := startChild(t, "research-worker", "TS_T05_LEASE_SECONDS=120")
	second.expect(t, "READY")
	waitResearch(t, db, id, "SUCCEEDED") // well before the 120s lease
	if _, attempts, actions := researchState(t, db, id); attempts != 2 || strings.Contains(actions, "LEASE_RECOVERED") {
		t.Fatalf("after restart: %d %s", attempts, actions)
	}
	second.signal(t, syscall.SIGTERM)
	if ws := second.wait(t); !ws.Exited() || ws.ExitStatus() != 0 {
		t.Fatalf("graceful stop: %v %s", ws, second.stderr.String())
	}
}

// Validation: a paper write killed inside its transaction changes nothing, and
// retrying it with the same idempotency key applies it exactly once.
func TestKilledPaperWriteChangesNothingAndRetriesOnce(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	document := func() []byte {
		var raw []byte
		if err := db.Pool.QueryRow(ctx, `SELECT document FROM paper_workspace WHERE id=1`).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	accounts := func() int {
		var p paper.Persisted
		if err := json.Unmarshal(document(), &p); err != nil {
			t.Fatal(err)
		}
		return len(p.State.Accounts)
	}
	before, beforeAccounts := document(), accounts()
	key := "t05-paper-" + randomText(6)

	writer := startChild(t, "paper-write", "TS_T05_KEY="+key)
	writer.expect(t, "IN_TRANSACTION")
	writer.signal(t, syscall.SIGKILL)
	if ws := writer.wait(t); !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
		t.Fatalf("helper was not killed: %v", ws)
	}
	if after := document(); !bytes.Equal(before, after) {
		t.Fatal("a killed transaction changed the paper document")
	}

	op := func(s *paper.State, now time.Time) (any, error) { return s.CreateAccount("USD", 100000, now) }
	firstBody, err := db.Execute(ctx, key, paperHash, op)
	if err != nil {
		t.Fatal(err)
	}
	replayBody, err := db.Execute(ctx, key, paperHash, op)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBody, replayBody) || accounts() != beforeAccounts+1 {
		t.Fatalf("retry applied %d times (replay equal: %v)", accounts()-beforeAccounts, bytes.Equal(firstBody, replayBody))
	}
}

package research

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"tradestocker/internal/store"
)

// These tests need a disposable PostgreSQL database; run scripts/test-integration.sh.
func integrationService(t *testing.T, f Fetcher) *Service {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL absent; run scripts/test-integration.sh")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(u.Path, "/tradestocker_test_") {
		t.Fatal("only explicitly disposable tradestocker_test_* databases permitted")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	for i := 0; i < 2; i++ {
		if err = db.Migrate(ctx); err != nil {
			t.Fatal("migration not repeatable: ", err)
		}
	}
	// TRUNCATE is allowed on the test database; row UPDATE/DELETE is refused by triggers.
	if _, err = db.Pool.Exec(ctx, `TRUNCATE research_events, research_sources, research_jobs, research_provider_state`); err != nil {
		t.Fatal(err)
	}
	return &Service{Pool: db.Pool, Provider: f, Configured: true}
}

func post(h http.Handler, key, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/research/jobs", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("status %d body %s: %v", w.Code, w.Body, err)
	}
	return v
}

const fixtureRequest = `{"symbol":"TEST","cik":"0000000001"}`

func createJob(t *testing.T, s *Service, key string) Job {
	t.Helper()
	w := post(s.Handler(), key, fixtureRequest)
	if w.Code != http.StatusAccepted {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	return decode[Job](t, w)
}

func loadJob(t *testing.T, s *Service, id string) Job {
	t.Helper()
	j, err := scanJob(s.Pool.QueryRow(context.Background(), `SELECT `+jobColumns+` FROM research_jobs WHERE id=$1`, id))
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func actions(t *testing.T, s *Service, id string) []string {
	t.Helper()
	rows, err := s.Pool.Query(context.Background(), `SELECT action FROM research_events WHERE job_id=$1 ORDER BY sequence`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var a string
		if err = rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func processOnce(t *testing.T, s *Service) bool {
	t.Helper()
	processed, err := s.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return processed
}

func TestCreateIsValidatedIdempotentAndBounded(t *testing.T) {
	s := integrationService(t, &scriptedFetcher{})
	h := s.Handler()
	for name, tc := range map[string]struct {
		key, body string
		want      int
	}{
		"missing key":       {"", fixtureRequest, 422},
		"short key":         {"short", fixtureRequest, 422},
		"lowercase symbol":  {"key-lower-1", `{"symbol":"test","cik":"0000000001"}`, 422},
		"nine digit cik":    {"key-nine-01", `{"symbol":"TEST","cik":"000000001"}`, 422},
		"zero cik":          {"key-zero-01", `{"symbol":"TEST","cik":"0000000000"}`, 422},
		"numeric cik":       {"key-num-001", `{"symbol":"TEST","cik":1}`, 422},
		"unknown field":     {"key-field-1", `{"symbol":"TEST","cik":"0000000001","mode":"LIVE"}`, 422},
		"trailing document": {"key-trail-1", fixtureRequest + `{}`, 422},
		"oversized body":    {"key-large-1", `{"symbol":"` + strings.Repeat("A", 5000) + `"}`, 413},
	} {
		if w := post(h, tc.key, tc.body); w.Code != tc.want {
			t.Errorf("%s: got %d want %d (%s)", name, w.Code, tc.want, w.Body)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/research/jobs", strings.NewReader(fixtureRequest))
	r.Header.Set("Idempotency-Key", "key-ctype-1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("missing content type: %d", w.Code)
	}

	// Twelve concurrent submissions of one operation create exactly one job.
	var wg sync.WaitGroup
	ids := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := post(h, "same-operation-key", fixtureRequest)
			if w.Code != http.StatusAccepted {
				t.Errorf("concurrent create: %d %s", w.Code, w.Body)
				return
			}
			var j Job
			_ = json.Unmarshal(w.Body.Bytes(), &j)
			ids <- j.ID
		}()
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("one key produced jobs %s and %s", first, id)
		}
	}
	var count int
	if err := s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM research_jobs`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("got %d jobs, %v", count, err)
	}
	if got := actions(t, s, first); len(got) != 1 || got[0] != "REQUESTED" {
		t.Fatalf("events %v", got)
	}
	j := loadJob(t, s, first)
	if j.Status != "QUEUED" || j.Attempts != 0 || j.MaxAttempts != 3 || j.NextAttemptAt == nil {
		t.Fatalf("new job %+v", j)
	}
	if w := post(h, "same-operation-key", `{"symbol":"AAPL","cik":"0000320193"}`); w.Code != http.StatusConflict {
		t.Fatalf("key reuse with another body: %d", w.Code)
	}

	// Without an SEC contact, replays still answer but new work is refused.
	unconfigured := &Service{Pool: s.Pool, Provider: s.Provider}
	if w := post(unconfigured.Handler(), "same-operation-key", fixtureRequest); w.Code != http.StatusAccepted {
		t.Fatalf("replay while unconfigured: %d", w.Code)
	}
	if w := post(unconfigured.Handler(), "new-operation-key", fixtureRequest); w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "SEC_CONTACT_NOT_CONFIGURED") {
		t.Fatalf("unconfigured create: %d %s", w.Code, w.Body)
	}

	for i := 0; i < maxActiveJobs-1; i++ {
		createJob(t, s, fmt.Sprintf("queue-fill-%03d", i))
	}
	if w := post(h, "one-too-many", fixtureRequest); w.Code != http.StatusTooManyRequests {
		t.Fatalf("queue bound: %d %s", w.Code, w.Body)
	}
}

func TestWorkerStoresOriginalsThenReport(t *testing.T) {
	f := &scriptedFetcher{}
	s := integrationService(t, f)
	h := s.Handler()
	j := createJob(t, s, "worker-success-1")
	if !processOnce(t, s) || processOnce(t, s) {
		t.Fatal("expected exactly one due job")
	}
	done := loadJob(t, s, j.ID)
	if done.Status != "SUCCEEDED" || done.Attempts != 1 || done.Error != nil || done.LastError != nil {
		t.Fatalf("finished job %+v", done)
	}
	var report struct {
		Version string                    `json:"version"`
		Sources []struct{ SHA256 string } `json:"sources"`
	}
	if json.Unmarshal(done.Result, &report) != nil || report.Version != ParserVersion {
		t.Fatalf("report %s", done.Result)
	}
	if got := strings.Join(actions(t, s, j.ID), ","); got != "REQUESTED,ATTEMPT_STARTED,SEC_SOURCES_STORED,SUCCEEDED" {
		t.Fatalf("events %s", got)
	}

	detail := decode[struct {
		Job     Job      `json:"job"`
		Sources []Source `json:"sources"`
		Events  []Event  `json:"events"`
	}](t, get(h, "/api/v1/research/jobs/"+j.ID))
	originals := fixtureSources()
	if len(detail.Sources) != 2 || len(detail.Events) != 4 || detail.Job.Result == nil {
		t.Fatalf("detail %+v", detail)
	}
	for i, src := range detail.Sources {
		// Sources are listed by attempt then kind: companyfacts sorts before submissions.
		want := originals[1-i]
		if src.Kind != want.Kind || src.SHA256 != want.SHA256 || src.Size != want.Size || src.Attempt != 1 || src.URL != want.URL {
			t.Fatalf("source %d %+v", i, src)
		}
		if report.Sources[1-i].SHA256 != src.SHA256 {
			t.Fatal("report does not cite the stored source")
		}
		w := get(h, "/api/v1/research/jobs/"+j.ID+"/sources/"+src.Kind+"?sha256="+src.SHA256)
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), want.Body) || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
			t.Fatalf("download %s: %d", src.Kind, w.Code)
		}
	}
	for path, want := range map[string]int{
		"/api/v1/research/jobs/" + j.ID + "/sources/companyfacts?sha256=" + strings.Repeat("0", 64): 404,
		"/api/v1/research/jobs/" + j.ID + "/sources/companyfacts?sha256=XYZ":                        422,
		"/api/v1/research/jobs/" + j.ID + "/sources/prices":                                         404,
		"/api/v1/research/jobs/research_missing":                                                    404,
	} {
		if w := get(h, path); w.Code != want {
			t.Errorf("%s: got %d want %d", path, w.Code, want)
		}
	}
	list := decode[struct{ Jobs []Job }](t, get(h, "/api/v1/research/jobs"))
	if len(list.Jobs) != 1 || list.Jobs[0].ID != j.ID || list.Jobs[0].Result != nil {
		t.Fatalf("list %+v", list)
	}

	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `UPDATE research_sources SET body='\x7b7d'`); err == nil {
		t.Fatal("original source was modified")
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM research_events`); err == nil {
		t.Fatal("event log was deleted")
	}
}

func TestRetryableFailureRetriesThenSucceeds(t *testing.T) {
	f := &scriptedFetcher{results: []error{retryable("SEC_HTTP_ERROR")}}
	s := integrationService(t, f)
	j := createJob(t, s, "retry-then-ok")
	before := time.Now()
	processOnce(t, s)
	waiting := loadJob(t, s, j.ID)
	if waiting.Status != "QUEUED" || waiting.Attempts != 1 || waiting.LastError == nil || *waiting.LastError != "SEC_HTTP_ERROR" || waiting.Error != nil {
		t.Fatalf("after retryable failure %+v", waiting)
	}
	if d := waiting.NextAttemptAt.Sub(before); d < 25*time.Second || d > 40*time.Second {
		t.Fatalf("retry scheduled after %v", d)
	}
	if processOnce(t, s) {
		t.Fatal("job retried before its backoff")
	}
	if _, err := s.Pool.Exec(context.Background(), `UPDATE research_jobs SET not_before=now() WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	processOnce(t, s)
	done := loadJob(t, s, j.ID)
	if done.Status != "SUCCEEDED" || done.Attempts != 2 || done.LastError != nil || f.count() != 2 {
		t.Fatalf("after retry %+v calls=%d", done, f.count())
	}
	if got := strings.Join(actions(t, s, j.ID), ","); got != "REQUESTED,ATTEMPT_STARTED,ATTEMPT_FAILED_RETRY_SCHEDULED,ATTEMPT_STARTED,SEC_SOURCES_STORED,SUCCEEDED" {
		t.Fatalf("events %s", got)
	}

	// Three retryable failures exhaust the job; the last code is kept.
	f.results = []error{retryable("SEC_ACCESS_LIMITED"), retryable("SEC_ACCESS_LIMITED"), retryable("SEC_ACCESS_LIMITED")}
	k := createJob(t, s, "retry-exhausted")
	for i := 0; i < 3; i++ {
		// End both the job's retry wait and the provider-wide cooldown each time.
		if _, err := s.Pool.Exec(context.Background(), `UPDATE research_jobs SET not_before=now() WHERE id=$1`, k.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Pool.Exec(context.Background(), `UPDATE research_provider_state SET not_before=now()-interval '1 second'`); err != nil {
			t.Fatal(err)
		}
		processOnce(t, s)
	}
	failed := loadJob(t, s, k.ID)
	if failed.Status != "FAILED" || failed.Attempts != 3 || failed.Error == nil || *failed.Error != "SEC_ACCESS_LIMITED" {
		t.Fatalf("exhausted retries %+v", failed)
	}
}

// Reviewer reproduction: an SEC access limit pauses every queued job, durably.
func TestAccessLimitPausesAllJobsAcrossRestart(t *testing.T) {
	f := &scriptedFetcher{results: []error{retryable("SEC_ACCESS_LIMITED")}}
	s := integrationService(t, f)
	ctx := context.Background()
	a := createJob(t, s, "cooldown-job-a")
	b := createJob(t, s, "cooldown-job-b")
	processOnce(t, s)
	if got := loadJob(t, s, a.ID); got.Status != "QUEUED" || *got.LastError != "SEC_ACCESS_LIMITED" {
		t.Fatalf("job A %+v", got)
	}
	if processOnce(t, s) || f.count() != 1 {
		t.Fatalf("job B contacted SEC during the cooldown (calls=%d)", f.count())
	}
	if got := loadJob(t, s, b.ID); got.Status != "QUEUED" || got.Attempts != 0 {
		t.Fatalf("job B %+v", got)
	}
	status := decode[struct {
		Cooldown *time.Time `json:"provider_cooldown_until"`
	}](t, get(s.Handler(), "/api/v1/research/status"))
	if status.Cooldown == nil || time.Until(*status.Cooldown) < 9*time.Minute {
		t.Fatalf("status cooldown %v", status.Cooldown)
	}
	if got := strings.Join(actions(t, s, a.ID), ","); !strings.Contains(got, "ATTEMPT_FAILED_RETRY_SCHEDULED,PROVIDER_COOLDOWN_STARTED") {
		t.Fatalf("events %s", got)
	}
	// The pause survives a process restart (a new pool and service).
	db, err := store.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	restarted := &Service{Pool: db.Pool, Provider: f, Configured: true}
	if processed, err := restarted.ProcessOne(ctx); err != nil || processed {
		t.Fatalf("restarted worker ignored the cooldown: %v %v", processed, err)
	}
	// A second limit never shortens the pause.
	before := *status.Cooldown
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE research_provider_state SET not_before=now()+interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	if err = startCooldown(ctx, tx, a.ID, "SEC_ACCESS_LIMITED"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var until time.Time
	_ = s.Pool.QueryRow(ctx, `SELECT not_before FROM research_provider_state`).Scan(&until)
	if time.Until(until) < 55*time.Minute || !until.After(before) {
		t.Fatalf("cooldown shortened to %v", until)
	}
	// When the pause ends, the oldest due job runs; job A keeps its own retry wait.
	if _, err = s.Pool.Exec(ctx, `UPDATE research_provider_state SET not_before=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	processOnce(t, s)
	if got := loadJob(t, s, b.ID); got.Status != "SUCCEEDED" || f.count() != 2 {
		t.Fatalf("job B after cooldown %+v calls=%d", got, f.count())
	}
	if got := loadJob(t, s, a.ID); got.Status != "QUEUED" || got.NextAttemptAt == nil || time.Until(*got.NextAttemptAt) < 9*time.Minute {
		t.Fatalf("job A lost its own retry wait %+v", got)
	}
}

// Reviewer reproduction: SEC answers 403/429 while shutdown starts. The observed
// limit must still pause every job, on a middle attempt and on the last one.
func TestAccessLimitObservedDuringShutdownIsKept(t *testing.T) {
	for _, tc := range []struct {
		name         string
		prior        int
		status, code string
	}{
		{"middle attempt", 0, "QUEUED", ""},
		{"last attempt", 2, "FAILED", "SEC_ACCESS_LIMITED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &scriptedFetcher{results: []error{retryable("SEC_ACCESS_LIMITED")}, onFetch: cancel}
			s := integrationService(t, f)
			j := createJob(t, s, "limit-at-shutdown")
			other := createJob(t, s, "limit-at-shutdown-other")
			if _, err := s.Pool.Exec(context.Background(), `UPDATE research_jobs SET attempts=$2 WHERE id=$1`, j.ID, tc.prior); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ProcessOne(ctx); err != nil {
				t.Fatal(err)
			}
			got := loadJob(t, s, j.ID)
			if got.Status != tc.status || *got.LastError != "SEC_ACCESS_LIMITED" || (tc.code != "" && *got.Error != tc.code) {
				t.Fatalf("job after limit during shutdown %+v", got)
			}
			var until time.Time
			if err := s.Pool.QueryRow(context.Background(), `SELECT not_before FROM research_provider_state WHERE provider='SEC'`).Scan(&until); err != nil || time.Until(until) < 9*time.Minute {
				t.Fatalf("cooldown not kept: %v %v", until, err)
			}
			// The next worker process respects the pause.
			next := &Service{Pool: s.Pool, Provider: f, Configured: true}
			if processed, err := next.ProcessOne(context.Background()); err != nil || processed || f.count() != 1 {
				t.Fatalf("next worker contacted SEC: processed=%v calls=%d err=%v", processed, f.count(), err)
			}
			if loadJob(t, s, other.ID).Attempts != 0 {
				t.Fatal("another job was attempted during the cooldown")
			}
		})
	}
}

func TestPermanentFailureIsNotRetried(t *testing.T) {
	f := &scriptedFetcher{results: []error{permanent("SEC_SYMBOL_CIK_MISMATCH")}}
	s := integrationService(t, f)
	j := createJob(t, s, "permanent-fail")
	processOnce(t, s)
	failed := loadJob(t, s, j.ID)
	if failed.Status != "FAILED" || failed.Attempts != 1 || *failed.Error != "SEC_SYMBOL_CIK_MISMATCH" || f.count() != 1 {
		t.Fatalf("permanent failure %+v", failed)
	}
	var stored int
	_ = s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM research_sources WHERE job_id=$1`, j.ID).Scan(&stored)
	if stored != 0 || processOnce(t, s) {
		t.Fatal("failed job stored sources or was retried")
	}
}

func TestCrashedAttemptRecoversAfterLease(t *testing.T) {
	s := integrationService(t, &scriptedFetcher{})
	ctx := context.Background()
	j := createJob(t, s, "crash-recovery")
	// A worker claims the job and then dies without settling it.
	crashed, err := s.claim(ctx)
	if err != nil || crashed == nil || crashed.Attempts != 1 {
		t.Fatalf("claim %+v %v", crashed, err)
	}
	if processOnce(t, s) {
		t.Fatal("job recovered before its lease expired")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE research_jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	processOnce(t, s)
	done := loadJob(t, s, j.ID)
	if done.Status != "SUCCEEDED" || done.Attempts != 2 {
		t.Fatalf("recovered job %+v", done)
	}
	if got := strings.Join(actions(t, s, j.ID), ","); !strings.Contains(got, "ATTEMPT_STARTED,LEASE_RECOVERED,ATTEMPT_STARTED") {
		t.Fatalf("events %s", got)
	}
	// The stale first attempt cannot overwrite the newer result.
	if err = s.settle(ctx, *crashed, "FAILED", nil, `status='FAILED',error_code='STALE'`); !errors.Is(err, errLeaseLost) {
		t.Fatalf("stale attempt settled: %v", err)
	}
	if loadJob(t, s, j.ID).Status != "SUCCEEDED" {
		t.Fatal("stale attempt changed the job")
	}

	k := createJob(t, s, "crash-exhausted")
	if _, err = s.Pool.Exec(ctx, `UPDATE research_jobs SET status='RUNNING',attempts=3,lease_until=now()-interval '1 second' WHERE id=$1`, k.ID); err != nil {
		t.Fatal(err)
	}
	processOnce(t, s)
	exhausted := loadJob(t, s, k.ID)
	if exhausted.Status != "FAILED" || *exhausted.Error != "WORKER_ATTEMPTS_EXHAUSTED" || exhausted.Attempts != 3 {
		t.Fatalf("exhausted crash %+v", exhausted)
	}
}

func TestShutdownReleasesAttemptAndRestartFinishes(t *testing.T) {
	f := &scriptedFetcher{block: make(chan struct{})}
	s := integrationService(t, f)
	j := createJob(t, s, "shutdown-restart")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := s.ProcessOne(ctx); done <- err }()
	for deadline := time.Now().Add(5 * time.Second); f.count() == 0; {
		if time.Now().After(deadline) {
			t.Fatal("worker never fetched")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	released := loadJob(t, s, j.ID)
	if released.Status != "QUEUED" || released.Attempts != 1 || *released.LastError != "WORKER_STOPPED" {
		t.Fatalf("released job %+v", released)
	}
	// A fresh process (new pool, new service) completes the job.
	db, err := store.Open(context.Background(), os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	restarted := &Service{Pool: db.Pool, Provider: &scriptedFetcher{}, Configured: true}
	if processed, err := restarted.ProcessOne(context.Background()); err != nil || !processed {
		t.Fatalf("restart: %v %v", processed, err)
	}
	if final := loadJob(t, s, j.ID); final.Status != "SUCCEEDED" || final.Attempts != 2 {
		t.Fatalf("after restart %+v", final)
	}
	if got := strings.Join(actions(t, s, j.ID), ","); !strings.Contains(got, "ATTEMPT_INTERRUPTED") {
		t.Fatalf("events %s", got)
	}
}

// Reviewer reproduction: interrupting the last attempt must not leave a queued job
// whose next claim would exceed the attempt limit.
func TestShutdownDuringLastAttemptFailsVisibly(t *testing.T) {
	f := &scriptedFetcher{block: make(chan struct{})}
	s := integrationService(t, f)
	ctx := context.Background()
	j := createJob(t, s, "shutdown-last-attempt")
	if _, err := s.Pool.Exec(ctx, `UPDATE research_jobs SET attempts=2 WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	run, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := s.ProcessOne(run); done <- err }()
	for deadline := time.Now().Add(5 * time.Second); f.count() == 0; {
		if time.Now().After(deadline) {
			t.Fatal("worker never fetched")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	failed := loadJob(t, s, j.ID)
	if failed.Status != "FAILED" || failed.Attempts != 3 || *failed.Error != "WORKER_ATTEMPTS_EXHAUSTED" || *failed.LastError != "WORKER_STOPPED" {
		t.Fatalf("interrupted last attempt %+v", failed)
	}
	if processOnce(t, s) {
		t.Fatal("exhausted job was claimed")
	}
	// A queued job that already used every attempt (for example, from an older
	// build) becomes a visible failure instead of blocking the worker.
	k := createJob(t, s, "queued-no-attempts-left")
	if _, err := s.Pool.Exec(ctx, `UPDATE research_jobs SET attempts=3 WHERE id=$1`, k.ID); err != nil {
		t.Fatal(err)
	}
	if processOnce(t, s) {
		t.Fatal("exhausted queued job was claimed")
	}
	if stuck := loadJob(t, s, k.ID); stuck.Status != "FAILED" || *stuck.Error != "WORKER_ATTEMPTS_EXHAUSTED" {
		t.Fatalf("queued exhausted job %+v", stuck)
	}
}

func TestAnalysisFailureKeepsOriginals(t *testing.T) {
	s := integrationService(t, &scriptedFetcher{})
	s.Script = "raise SystemExit(4)"
	j := createJob(t, s, "analysis-fails")
	processOnce(t, s)
	failed := loadJob(t, s, j.ID)
	if failed.Status != "FAILED" || *failed.Error != "RESEARCH_ANALYSIS_FAILED" || failed.Result != nil {
		t.Fatalf("analysis failure %+v", failed)
	}
	var stored int
	if err := s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM research_sources WHERE job_id=$1`, j.ID).Scan(&stored); err != nil || stored != 2 {
		t.Fatalf("originals kept: %d %v", stored, err)
	}
}

func TestOnlyTheLeaderProcesses(t *testing.T) {
	s := integrationService(t, &scriptedFetcher{})
	j := createJob(t, s, "leader-only")
	ctx := context.Background()
	other, err := s.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.Exec(ctx, `SELECT pg_advisory_lock(81342104)`); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err = s.runLeader(short); err != nil {
		t.Fatal(err)
	}
	if loadJob(t, s, j.ID).Status != "QUEUED" {
		t.Fatal("a non-leader processed a job")
	}
	_, _ = other.Exec(ctx, `SELECT pg_advisory_unlock(81342104)`)
	other.Release()
	run, stop := context.WithCancel(ctx)
	finished := make(chan struct{})
	go func() { s.Run(run); close(finished) }()
	for deadline := time.Now().Add(10 * time.Second); loadJob(t, s, j.ID).Status != "SUCCEEDED"; {
		if time.Now().After(deadline) {
			t.Fatal("leader did not process the job")
		}
		time.Sleep(50 * time.Millisecond)
	}
	stop()
	<-finished
}

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"tradestocker/internal/httpapi"
	"tradestocker/internal/paper"
)

// testClock lets a test move time exactly to an expiry instant.
type testClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *testClock) Now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.at }
func (c *testClock) Set(t time.Time) { c.mu.Lock(); c.at = t; c.mu.Unlock() }

// readTestStore opens the disposable database with a pool large enough that
// readers wait on the row lock, not on pool connections.
func readTestStore(t *testing.T) *Store {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL absent; run scripts/test-integration.sh")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(u.Path, "/tradestocker_test_") {
		t.Fatal("only explicitly disposable tradestocker_test_* databases permitted")
	}
	q := u.Query()
	q.Set("pool_max_conns", "40")
	u.RawQuery = q.Encode()
	s, err := Open(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err = s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

// countUpdates installs a test-only trigger that counts every UPDATE of the paper
// row exactly (statistics views can lag by seconds).
func countUpdates(t *testing.T, s *Store) (reset func(), count func() int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS test_paper_writes(id int PRIMARY KEY, updates bigint NOT NULL);
INSERT INTO test_paper_writes VALUES (1, 0) ON CONFLICT (id) DO UPDATE SET updates = 0;
CREATE OR REPLACE FUNCTION test_count_paper_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN UPDATE test_paper_writes SET updates = updates + 1 WHERE id = 1; RETURN NEW; END $$;
DROP TRIGGER IF EXISTS test_count_paper_write ON paper_workspace;
CREATE TRIGGER test_count_paper_write AFTER UPDATE ON paper_workspace FOR EACH ROW EXECUTE FUNCTION test_count_paper_write();`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.Pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS test_count_paper_write ON paper_workspace; DROP FUNCTION IF EXISTS test_count_paper_write(); DROP TABLE IF EXISTS test_paper_writes;`)
	})
	reset = func() {
		if _, err := s.Pool.Exec(ctx, `UPDATE test_paper_writes SET updates = 0`); err != nil {
			t.Fatal(err)
		}
	}
	count = func() int64 {
		var n int64
		if err := s.Pool.QueryRow(ctx, `SELECT updates FROM test_paper_writes WHERE id = 1`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	return reset, count
}

func freshDocument(t *testing.T, s *Store) {
	t.Helper()
	b, err := json.Marshal(paper.Persisted{State: paper.NewState(), Receipts: map[string]paper.Receipt{}})
	if err == nil {
		_, err = s.Pool.Exec(context.Background(), `UPDATE paper_workspace SET document=$1 WHERE id=1`, b)
	}
	if err != nil {
		t.Fatal(err)
	}
}

type rowVersion struct{ xmin, updatedAt, digest string }

func version(t *testing.T, s *Store) rowVersion {
	t.Helper()
	var v rowVersion
	if err := s.Pool.QueryRow(context.Background(), `SELECT xmin::text, updated_at::text, md5(document::text) FROM paper_workspace WHERE id=1`).Scan(&v.xmin, &v.updatedAt, &v.digest); err != nil {
		t.Fatal(err)
	}
	return v
}

// waitForLockedReaders waits until PostgreSQL reports n sessions blocked on the
// paper row lock, so all of them really queued behind the held lock.
func waitForLockedReaders(t *testing.T, s *Store, n int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var waiting int
		if err := s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE 'SELECT document FROM paper_workspace WHERE id=1 FOR UPDATE%'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d sessions waited on the row lock", waiting, n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func countBy[T any](items []T, match func(T) bool) int {
	n := 0
	for _, item := range items {
		if match(item) {
			n++
		}
	}
	return n
}

// expiryRecords counts the refund, journal line, audit event and notice for one decision.
func expiryRecords(t *testing.T, s *Store, rec paper.Recommendation) (journal, audit, notices int, final paper.State) {
	t.Helper()
	var raw []byte
	if err := s.Pool.QueryRow(context.Background(), `SELECT document FROM paper_workspace WHERE id=1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	final, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	journal = countBy(final.Journal, func(j paper.Journal) bool { return j.EntityID == rec.ID && j.Reason == "EXPIRE_RESERVATION" })
	audit = countBy(final.Audit, func(a paper.Audit) bool { return a.EntityID == rec.ID && a.Action == "PAPER_DECISION_EXPIRED" })
	notices = countBy(final.Notifications, func(n paper.Notification) bool { return strings.Contains(n.Text, "Paper decision expired") })
	return journal, audit, notices, final
}

// Reviewer reproduction: HEAD on GET routes went to the write path. Through the
// real HTTP handler and real PostgreSQL, GET and HEAD must never update the row.
func TestHTTPReadMethodsNeverWriteThePaperRow(t *testing.T) {
	s := readTestStore(t)
	freshDocument(t, s)
	t.Cleanup(func() { freshDocument(t, s) })
	resetCount, updates := countUpdates(t, s)
	token := strings.Repeat("h", 32)
	handler := (&httpapi.API{Store: s, Token: token}).Handler()
	send := func(method, path, body, key string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		if method == http.MethodPost {
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", key)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	if code := send("POST", "/api/v1/paper/accounts", `{"currency":"USD","amount_minor":1000000}`, "http-read-account"); code != 200 {
		t.Fatalf("create account: %d", code)
	}
	if code := send("POST", "/api/v1/fixtures", `{}`, "http-read-fixtures"); code != 200 {
		t.Fatalf("fixtures: %d", code)
	}
	resetCount()
	before := version(t, s)
	for _, method := range []string{"GET", "HEAD"} {
		for _, path := range []string{"/api/v1/health", "/api/v1/dashboard", "/api/v1/risk/policy"} {
			for i := 0; i < 10; i++ {
				if code := send(method, path, "", ""); code != 200 {
					t.Fatalf("%s %s: %d", method, path, code)
				}
			}
		}
	}
	for _, method := range []string{"PUT", "DELETE", "OPTIONS"} {
		if code := send(method, "/api/v1/dashboard", "", ""); code != http.StatusMethodNotAllowed {
			t.Fatalf("%s dashboard: %d", method, code)
		}
	}
	if n := updates(); n != 0 || version(t, s) != before {
		t.Fatalf("60 GET/HEAD requests and 3 refused methods caused %d updates", n)
	}
	// A POST still writes, through the unchanged path.
	if code := send("POST", "/api/v1/paper/entries", `{"paused":true}`, "http-read-pause"); code != 200 {
		t.Fatalf("pause: %d", code)
	}
	if n := updates(); n != 1 {
		t.Fatalf("one POST caused %d updates, want 1", n)
	}
}

func TestReadPathWritesOnlyForDueExpiry(t *testing.T) {
	ctx := context.Background()
	s := readTestStore(t)
	clock := &testClock{at: time.Now().UTC().Truncate(time.Second)}
	s.Now = clock.Now
	freshDocument(t, s)
	t.Cleanup(func() { freshDocument(t, s) })
	resetCount, updates := countUpdates(t, s)

	exec := func(key, hash string, fn func(*paper.State, time.Time) (any, error)) []byte {
		t.Helper()
		b, err := s.Execute(ctx, key, hash, fn)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var account paper.Account
	_ = json.Unmarshal(exec("read-test-account", "account", func(st *paper.State, n time.Time) (any, error) { return st.CreateAccount("USD", 1_000_000, n) }), &account)
	exec("read-test-fixtures", "fixtures", func(st *paper.State, n time.Time) (any, error) { return st.LoadFixtures(n), nil })
	evaluate := func(key string) paper.Recommendation {
		var r paper.Recommendation
		_ = json.Unmarshal(exec(key, key, func(st *paper.State, n time.Time) (any, error) {
			return st.Evaluate(paper.Candidate{AccountID: account.ID, Symbol: "AAPL", Lane: "US_SWING", Quantity: 1, Entry: 20000, Stop: 19500}, n)
		}), &r)
		if r.Status != "PROPOSED" || r.Reserved == 0 {
			t.Fatalf("candidate not reserved: %+v", r)
		}
		return r
	}
	first := evaluate("read-test-first")
	clock.Set(clock.Now().Add(time.Minute))
	second := evaluate("read-test-second")
	dashboard := func(st *paper.State, _ time.Time) (any, error) { return st, nil }
	health := func(*paper.State, time.Time) (any, error) { return map[string]any{"status": "ok"}, nil }

	// 1. Unchanged reads and health checks, in parallel, write nothing.
	resetCount()
	before := version(t, s)
	var wg sync.WaitGroup
	bodies := make(chan string, 200)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				b, err := s.Read(ctx, dashboard)
				if err == nil {
					_, err = s.Read(ctx, health)
				}
				if err != nil {
					t.Error(err)
					return
				}
				bodies <- string(b)
			}
		}()
	}
	wg.Wait()
	close(bodies)
	firstBody := ""
	for b := range bodies {
		if firstBody == "" {
			firstBody = b
		}
		if b != firstBody {
			t.Fatal("unchanged reads returned different documents")
		}
	}
	if n := updates(); n != 0 || version(t, s) != before {
		t.Fatalf("400 unchanged reads caused %d updates; version %+v -> %+v", n, before, version(t, s))
	}

	// 2. At the first expiry instant, 12 readers queue behind a held row lock.
	// Each saw "due" in its snapshot; only the first may write.
	clock.Set(first.ExpiresAt)
	resetCount()
	hold, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = hold.Exec(ctx, `SELECT document FROM paper_workspace WHERE id=1 FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	const readers = 12
	results := make(chan []byte, readers)
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := s.Read(ctx, dashboard)
			if err != nil {
				t.Error(err)
				return
			}
			results <- b
		}()
	}
	waitForLockedReaders(t, s, readers)
	if err = hold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	close(results)
	for b := range results {
		var shown paper.State
		_ = json.Unmarshal(b, &shown)
		for _, r := range shown.Recommendations {
			if r.ID == first.ID && (r.Status != "EXPIRED" || r.Reserved != 0) {
				t.Fatalf("a reader saw the due decision unexpired: %+v", r)
			}
		}
	}
	journal, audit, notices, final := expiryRecords(t, s, first)
	if n := updates(); n != 1 {
		t.Fatalf("expiry with %d queued readers caused %d updates, want exactly 1", readers, n)
	}
	if journal != 1 || audit != 1 || notices != 1 || final.Accounts[0].Reserved != second.Reserved {
		t.Fatalf("expiry records journal=%d audit=%d notices=%d reserved=%d", journal, audit, notices, final.Accounts[0].Reserved)
	}

	// 3. At the second expiry, readers race competing writes and a retried write.
	clock.Set(second.ExpiresAt)
	resetCount()
	retryBodies := make(chan string, 2)
	executes := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Read(ctx, dashboard); err != nil {
				t.Error(err)
			}
		}()
	}
	for i := 0; i < 3; i++ {
		executes++
		key := fmt.Sprintf("read-test-refresh-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Execute(ctx, key, key, func(st *paper.State, n time.Time) (any, error) { return st.LoadFixtures(n), nil }); err != nil {
				t.Error(err)
			}
		}()
	}
	for i := 0; i < 2; i++ {
		executes++
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := s.Execute(ctx, "read-test-retried", "same-body", func(st *paper.State, n time.Time) (any, error) {
				st.EntriesPaused = false
				return map[string]bool{"paused": false}, nil
			})
			if err != nil {
				t.Error(err)
				return
			}
			retryBodies <- string(b)
		}()
	}
	wg.Wait()
	close(retryBodies)
	if a, b := <-retryBodies, <-retryBodies; a != b {
		t.Fatalf("retried write returned %q then %q", a, b)
	}
	journal, audit, _, final = expiryRecords(t, s, second)
	if journal != 1 || audit != 1 || final.Accounts[0].Reserved != 0 {
		t.Fatalf("second expiry journal=%d audit=%d reserved=%d", journal, audit, final.Accounts[0].Reserved)
	}
	// Execute always writes; a reader adds at most one write, and only if it applied the expiry.
	if n := updates(); n < int64(executes) || n > int64(executes)+1 {
		t.Fatalf("got %d updates for %d writes", n, executes)
	}

	// 4. With nothing due, reads write nothing again; a failed write rolls back.
	resetCount()
	before = version(t, s)
	for i := 0; i < 20; i++ {
		if _, err = s.Read(ctx, dashboard); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.Execute(ctx, "read-test-rollback", "rollback", func(st *paper.State, n time.Time) (any, error) {
		st.Accounts[0].Cash += 12345
		return nil, paper.Fail("TEST_FAILURE", "intentional rollback", 409)
	}); err == nil {
		t.Fatal("failed mutation succeeded")
	}
	if n := updates(); n != 0 || version(t, s) != before {
		t.Fatalf("after expiry: %d updates from reads and a rolled-back write", n)
	}
	// 5. Idempotency still holds: a reused key with another body is refused.
	if _, err = s.Execute(ctx, "read-test-retried", "other-body", func(*paper.State, time.Time) (any, error) { return nil, nil }); err == nil || !strings.Contains(err.Error(), "different request") {
		t.Fatalf("key reuse with another body: %v", err)
	}
}

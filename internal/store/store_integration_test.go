package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"tradestocker/internal/paper"
)

func TestPostgresAtomicityPersistenceAndConcurrency(t *testing.T) {
	urlString := os.Getenv("TEST_DATABASE_URL")
	if urlString == "" {
		t.Skip("TEST_DATABASE_URL absent; run scripts/test-integration.sh")
	}
	u, e := url.Parse(urlString)
	if e != nil || !strings.HasPrefix(u.Path, "/tradestocker_test_") {
		t.Fatal("only explicitly disposable tradestocker_test_* databases permitted")
	}
	ctx := context.Background()
	s, e := Open(ctx, urlString)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Migrate(ctx); e != nil {
		t.Fatal("migration not repeatable", e)
	}
	exec := func(key, hash string, fn func(*paper.State, time.Time) (any, error)) []byte {
		t.Helper()
		b, e := s.Execute(ctx, key, hash, fn)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	create := func(st *paper.State, n time.Time) (any, error) { return st.CreateAccount("USD", 1_000_000, n) }
	original := exec("create-test-account", "create-usd", create)
	again := exec("create-test-account", "create-usd", create)
	if string(original) != string(again) {
		t.Fatal("retry response changed")
	}
	var a paper.Account
	json.Unmarshal(original, &a)
	if _, e = s.Execute(ctx, "create-test-account", "different-body", create); e == nil {
		t.Fatal("idempotency collision accepted")
	}
	exec("load-test-fixtures", "fixtures", func(st *paper.State, n time.Time) (any, error) { return st.LoadFixtures(n), nil })
	// A second connection represents process restart against the same durable database.
	reopened, e := Open(ctx, urlString)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	b, e := reopened.Execute(ctx, "create-test-account", "create-usd", create)
	if e != nil || string(b) != string(original) {
		t.Fatal("replay receipt did not survive reconnect", e)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	approved := []paper.Recommendation{}
	errs := []error{}
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b, e := s.Execute(ctx, fmt.Sprintf("candidate-%d", i), fmt.Sprintf("body-%d", i), func(st *paper.State, n time.Time) (any, error) {
				return st.Evaluate(paper.Candidate{AccountID: a.ID, Symbol: "AAPL", Lane: "US_SWING", Quantity: 1, Entry: 20000, Stop: 19500}, n)
			})
			mu.Lock()
			defer mu.Unlock()
			if e != nil {
				errs = append(errs, e)
				return
			}
			var r paper.Recommendation
			json.Unmarshal(b, &r)
			if r.Status == "PROPOSED" {
				approved = append(approved, r)
			}
		}(i)
	}
	wg.Wait()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(approved) != 10 {
		t.Fatalf("concurrent exposure gate: got %d accepted, want 10", len(approved))
	}
	r := approved[0]
	exec("ack-test-candidate", "ack", func(st *paper.State, n time.Time) (any, error) { return st.Acknowledge(r.ID, n) })
	success := 0
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Execute(ctx, fmt.Sprintf("fill-%d", i), fmt.Sprintf("fill-body-%d", i), func(st *paper.State, n time.Time) (any, error) { return st.Fill(r.ID, 20000, "PAPER_ONLY", n) })
			if e == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("got %d financial effects, want 1", success)
	}
	_, e = s.Execute(ctx, "rollback-test-key", "rollback", func(st *paper.State, n time.Time) (any, error) {
		st.Accounts[0].Cash += 12345
		return nil, paper.Fail("TEST_FAILURE", "intentional rollback", 409)
	})
	if e == nil {
		t.Fatal("failed mutation succeeded")
	}
	snapshot := exec("", "", func(st *paper.State, n time.Time) (any, error) {
		if e := st.Validate(); e != nil {
			return nil, e
		}
		return st, nil
	})
	var final paper.State
	json.Unmarshal(snapshot, &final)
	if final.Accounts[0].Cash != 980000 || final.Accounts[0].Reserved != 180000 || len(final.Positions) != 1 || final.Positions[0].Quantity != 1 {
		t.Fatalf("incorrect final balance %+v %+v", final.Accounts, final.Positions)
	}
	if strings.Contains(string(snapshot), "receipts") || strings.Contains(string(snapshot), "create-usd") {
		t.Fatal("private receipt exposed")
	}
	t.Log("Migration replay, persistent idempotency, reconnect, 24-way reservation race, 16-way fill race, rollback and journal reconciliation passed")
}

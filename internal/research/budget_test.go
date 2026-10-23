package research

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"tradestocker/internal/store"
)

func TestRetryBudgetDefaults(t *testing.T) {
	if got := (&Service{}).retryBudget(); got != DefaultRetryBudget {
		t.Fatalf("unset budget %+v", got)
	}
	if got := (&Service{Retries: RetryBudget{Capacity: 0, Window: time.Minute}}).retryBudget(); got != DefaultRetryBudget {
		t.Fatalf("zero capacity %+v", got)
	}
	custom := RetryBudget{Capacity: 2, Window: time.Minute}
	if got := (&Service{Retries: custom}).retryBudget(); got != custom {
		t.Fatalf("custom budget %+v", got)
	}
}

// Reviewer counterexample: retries planned apart but all overdue after a pause
// (or a restart) must still start no faster than the budget.
func TestRetryBudgetHoldsOverdueRetriesAcrossRestart(t *testing.T) {
	f := &scriptedFetcher{results: []error{retryable("SEC_HTTP_ERROR"), retryable("SEC_HTTP_ERROR"), retryable("SEC_HTTP_ERROR"), retryable("SEC_HTTP_ERROR")}}
	s := integrationService(t, f)
	budget := RetryBudget{Capacity: 2, Window: 3 * time.Second}
	s.Retries = budget
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		createJob(t, s, fmt.Sprintf("budget-retry-%d", i))
		processOnce(t, s) // the first attempt fails and schedules a retry
	}
	// Every retry is now long overdue, as after the worker was paused.
	if _, err := s.Pool.Exec(ctx, `UPDATE research_jobs SET not_before=now()-interval '20 minutes'`); err != nil {
		t.Fatal(err)
	}
	claimed := make(chan *Job, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := s.claim(ctx)
			if err != nil {
				t.Error(err)
			}
			claimed <- j
		}()
	}
	wg.Wait()
	close(claimed)
	started := 0
	for j := range claimed {
		if j != nil {
			started++
		}
	}
	if started != budget.Capacity {
		t.Fatalf("%d overdue retries started together, want %d", started, budget.Capacity)
	}

	// A restarted process (new pool, new service) keeps the count.
	db, err := store.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	restarted := &Service{Pool: db.Pool, Provider: f, Configured: true, Retries: budget}
	if j, err := restarted.claim(ctx); err != nil || j != nil {
		t.Fatalf("restart started a retry beyond the budget: %+v %v", j, err)
	}
	// First attempts are not held by the retry budget.
	fresh := createJob(t, s, "budget-first-attempt")
	if j, err := restarted.claim(ctx); err != nil || j == nil || j.ID != fresh.ID {
		t.Fatalf("first attempt held: %+v %v", j, err)
	}
	// Once the window has passed, the next retry starts.
	for deadline := time.Now().Add(10 * time.Second); ; {
		j, err := restarted.claim(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if j != nil {
			if j.Attempts != 2 {
				t.Fatalf("expected a retry, got %+v", j)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("retry never started after the window")
		}
		time.Sleep(100 * time.Millisecond)
	}
	// No window of the budget's length holds more than Capacity retry starts.
	rows, err := s.Pool.Query(ctx, `SELECT at FROM research_events WHERE action='ATTEMPT_STARTED' AND (detail->>'attempt')::int>1 ORDER BY at`)
	if err != nil {
		t.Fatal(err)
	}
	var starts []time.Time
	for rows.Next() {
		var at time.Time
		if err = rows.Scan(&at); err != nil {
			t.Fatal(err)
		}
		starts = append(starts, at)
	}
	rows.Close()
	if len(starts) != 3 {
		t.Fatalf("%d retry starts recorded, want 3", len(starts))
	}
	for i := budget.Capacity; i < len(starts); i++ {
		if starts[i].Sub(starts[i-budget.Capacity]) < budget.Window {
			t.Fatalf("retry starts %v exceed %d per %v", starts, budget.Capacity, budget.Window)
		}
	}
}

// Reviewer counterexample: a claim that waited for the claim lock longer than
// the budget window and the lease must date its retry start and its lease from
// admission, not from its transaction start.
func TestClaimAfterAWaitForTheLockUsesTheAdmissionTime(t *testing.T) {
	f := &scriptedFetcher{results: []error{retryable("SEC_HTTP_ERROR"), retryable("SEC_HTTP_ERROR")}}
	s := integrationService(t, f)
	s.Retries = RetryBudget{Capacity: 1, Window: time.Second}
	s.LeaseSeconds = 2
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		createJob(t, s, fmt.Sprintf("held-lock-%d", i))
		processOnce(t, s)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE research_jobs SET not_before=now()-interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	// Another session of this test database holds the claim lock.
	holder, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(context.Background())
	if _, err = holder.Exec(ctx, `SELECT pg_advisory_xact_lock(81342105)`); err != nil {
		t.Fatal(err)
	}
	claimed := make(chan *Job, 1)
	go func() {
		j, err := s.claim(ctx)
		if err != nil {
			t.Error(err)
		}
		claimed <- j
	}()
	for deadline := time.Now().Add(5 * time.Second); ; {
		var waiting int
		if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND objid=81342105 AND NOT granted AND database=(SELECT oid FROM pg_database WHERE datname=current_database())`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the claim never waited for the lock")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(2500 * time.Millisecond) // longer than the window and the lease
	var released time.Time
	if err = holder.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&released); err != nil {
		t.Fatal(err)
	}
	if err = holder.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	first := <-claimed
	if first == nil || first.Attempts != 2 {
		t.Fatalf("waiting claim %+v", first)
	}
	// The retry just admitted counts against the budget: the next claim waits.
	if j, err := s.claim(ctx); err != nil || j != nil {
		t.Fatalf("second retry admitted right after the first: %+v %v", j, err)
	}
	var startedAt, leaseUntil time.Time
	var leaseLive bool
	if err = s.Pool.QueryRow(ctx, `SELECT e.at,j.lease_until,j.lease_until>clock_timestamp() FROM research_events e JOIN research_jobs j ON j.id=e.job_id WHERE e.job_id=$1 AND e.action='ATTEMPT_STARTED' AND (e.detail->>'attempt')::int=2`, first.ID).Scan(&startedAt, &leaseUntil, &leaseLive); err != nil {
		t.Fatal(err)
	}
	if startedAt.Before(released) || leaseUntil.Sub(startedAt) != 2*time.Second || !leaseLive {
		t.Fatalf("start %v (lock released %v), lease until %v, live %v", startedAt, released, leaseUntil, leaseLive)
	}
}

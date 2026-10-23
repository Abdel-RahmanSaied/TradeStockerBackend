package research

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	maxAttempts   = 3
	maxActiveJobs = 20
	maxStoredJobs = 10000
	// The lease outlives one attempt, so a live worker never loses its job to recovery.
	leaseSeconds    = 150
	attemptDeadline = 90 * time.Second
)

var errLeaseLost = errors.New("research lease no longer owned")

// retryDelay is the wait after a retryable failure of the given attempt (1 or 2).
func retryDelay(code string, attempt int) time.Duration {
	if code == "SEC_ACCESS_LIMITED" {
		// SEC states access may resume once requests stay below its threshold for
		// 10 minutes. Pausing every job that long applies this conservatively; it
		// does not guarantee that SEC has lifted a block.
		return 10 * time.Minute
	}
	if attempt <= 1 {
		return 30 * time.Second
	}
	return 2 * time.Minute
}

// Run processes jobs while it holds a database session lock, so only one worker
// contacts SEC across API instances that share the database.
func (s *Service) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := s.runLeader(ctx); err != nil && ctx.Err() == nil {
			slog.Error("research worker database unavailable", "error", err)
		}
		sleep(ctx, time.Second)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func (s *Service) runLeader(ctx context.Context) error {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(81342104)`).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.Exec(c, `SELECT pg_advisory_unlock(81342104)`)
	}()
	for ctx.Err() == nil {
		// The lock lives with this session; stop leading if the session is gone.
		if _, err = conn.Exec(ctx, `SELECT 1`); err != nil {
			return err
		}
		processed, err := s.ProcessOne(ctx)
		if errors.Is(err, errLeaseLost) {
			slog.Warn("research attempt result discarded; a newer attempt owns the job")
		} else if err != nil {
			return err
		}
		if !processed {
			sleep(ctx, time.Second)
		}
	}
	return nil
}

// ProcessOne claims and runs at most one due job. It reports whether a job was claimed.
func (s *Service) ProcessOne(ctx context.Context) (bool, error) {
	j, err := s.claim(ctx)
	if err != nil || j == nil {
		return false, err
	}
	work, cancel := context.WithTimeout(ctx, attemptDeadline)
	defer cancel()
	sources, err := s.Provider.Fetch(work, j.Symbol, j.CIK)
	if err != nil {
		return true, s.afterFailure(ctx, *j, err)
	}
	// Originals are stored before analysis, so they survive a parser failure.
	if err = s.storeSources(work, *j, sources); err != nil {
		if ctx.Err() != nil {
			return true, s.release(*j)
		}
		return true, err
	}
	report, err := s.analyze(work, *j, sources)
	if err != nil {
		if ctx.Err() != nil {
			return true, s.release(*j)
		}
		return true, s.settle(ctx, *j, "FAILED", map[string]any{"attempt": j.Attempts, "code": "RESEARCH_ANALYSIS_FAILED"},
			`status='FAILED',error_code='RESEARCH_ANALYSIS_FAILED',last_error='RESEARCH_ANALYSIS_FAILED'`)
	}
	return true, s.settle(ctx, *j, "SUCCEEDED", map[string]any{"attempt": j.Attempts, "parser_version": ParserVersion},
		`status='SUCCEEDED',result=$3,last_error=NULL`, json.RawMessage(report))
}

func (s *Service) claim(ctx context.Context) (*Job, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	// A crashed attempt keeps its lease until expiry. A job with no attempts left,
	// crashed or queued, becomes a visible failure and is never claimed again.
	rows, err := tx.Query(ctx, `UPDATE research_jobs SET status='FAILED',error_code='WORKER_ATTEMPTS_EXHAUSTED',lease_until=NULL,updated_at=now() WHERE attempts>=$1 AND (status='QUEUED' OR (status='RUNNING' AND lease_until<now())) RETURNING id`, maxAttempts)
	if err != nil {
		return nil, err
	}
	exhausted, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	for _, id := range exhausted {
		if err = addEvent(ctx, tx, id, "WORKER_ATTEMPTS_EXHAUSTED", nil); err != nil {
			return nil, err
		}
	}
	// While SEC limits access, no job may contact it, whichever job caused the limit.
	var cooling bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM research_provider_state WHERE provider='SEC' AND not_before>now())`).Scan(&cooling); err != nil {
		return nil, err
	}
	if cooling {
		return nil, tx.Commit(ctx)
	}
	var id, previous string
	err = tx.QueryRow(ctx, `SELECT id,status FROM research_jobs WHERE attempts<$1 AND ((status='QUEUED' AND not_before<=now()) OR (status='RUNNING' AND lease_until<now())) ORDER BY not_before,created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, maxAttempts).Scan(&id, &previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	j, err := scanJob(tx.QueryRow(ctx, `UPDATE research_jobs SET status='RUNNING',attempts=attempts+1,lease_until=now()+$2*interval '1 second',updated_at=now() WHERE id=$1 RETURNING `+jobColumns, id, leaseSeconds))
	if err != nil {
		return nil, err
	}
	if previous == "RUNNING" {
		if err = addEvent(ctx, tx, id, "LEASE_RECOVERED", map[string]any{"previous_attempt": j.Attempts - 1}); err != nil {
			return nil, err
		}
	}
	if err = addEvent(ctx, tx, id, "ATTEMPT_STARTED", map[string]any{"attempt": j.Attempts}); err != nil {
		return nil, err
	}
	return &j, tx.Commit(ctx)
}

func (s *Service) afterFailure(ctx context.Context, j Job, err error) error {
	var pe *ProviderError
	if !errors.As(err, &pe) && ctx.Err() != nil {
		return s.release(j) // Shutdown interrupted the attempt before SEC answered.
	}
	// An answer SEC already gave (such as 403/429) is recorded even if shutdown
	// begins at the same moment, using a short context that outlives cancellation.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	code, retry := "RESEARCH_PROVIDER_FAILED", false
	switch {
	case errors.As(err, &pe):
		code, retry = pe.Code, pe.Retryable
	case errors.Is(err, context.DeadlineExceeded):
		code, retry = "SEC_REQUEST_TIMEOUT", true
	}
	var cooldown func(pgx.Tx) error
	if code == "SEC_ACCESS_LIMITED" {
		cooldown = func(tx pgx.Tx) error { return startCooldown(ctx, tx, j.ID, code) }
	}
	if retry && j.Attempts < maxAttempts {
		delay := retryDelay(code, j.Attempts)
		return s.settleWith(ctx, j, "ATTEMPT_FAILED_RETRY_SCHEDULED", map[string]any{"attempt": j.Attempts, "code": code, "retry_after_seconds": int(delay.Seconds())}, cooldown,
			`status='QUEUED',not_before=now()+$3*interval '1 second',last_error=$4`, int(delay.Seconds()), code)
	}
	return s.settleWith(ctx, j, "FAILED", map[string]any{"attempt": j.Attempts, "code": code}, cooldown, `status='FAILED',error_code=$3,last_error=$3`, code)
}

// startCooldown pauses every job's SEC access, never shortening an existing pause.
func startCooldown(ctx context.Context, tx pgx.Tx, jobID, code string) error {
	seconds := int(retryDelay(code, 1).Seconds())
	var until time.Time
	if err := tx.QueryRow(ctx, `INSERT INTO research_provider_state(provider,not_before,reason) VALUES('SEC',now()+$1*interval '1 second',$2)
ON CONFLICT (provider) DO UPDATE SET not_before=GREATEST(research_provider_state.not_before,EXCLUDED.not_before),reason=EXCLUDED.reason,updated_at=now()
RETURNING not_before`, seconds, code).Scan(&until); err != nil {
		return err
	}
	return addEvent(ctx, tx, jobID, "PROVIDER_COOLDOWN_STARTED", map[string]any{"provider": "SEC", "code": code, "until": until})
}

// release returns an interrupted attempt to the queue at once. A crash instead
// waits for lease expiry. Either way the attempt counts toward the limit, so an
// interrupted last attempt becomes a visible failure instead of a queued job.
func (s *Service) release(j Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if j.Attempts >= maxAttempts {
		return s.settle(ctx, j, "WORKER_ATTEMPTS_EXHAUSTED", map[string]any{"attempt": j.Attempts, "code": "WORKER_STOPPED"},
			`status='FAILED',error_code='WORKER_ATTEMPTS_EXHAUSTED',last_error='WORKER_STOPPED'`)
	}
	return s.settle(ctx, j, "ATTEMPT_INTERRUPTED", map[string]any{"attempt": j.Attempts}, `status='QUEUED',not_before=now(),last_error='WORKER_STOPPED'`)
}

// settle moves a RUNNING job on, only if this attempt still owns it.
func (s *Service) settle(ctx context.Context, j Job, action string, detail map[string]any, set string, args ...any) error {
	return s.settleWith(ctx, j, action, detail, nil, set, args...)
}

// settleWith also runs extra in the same transaction, after ownership is confirmed.
func (s *Service) settleWith(ctx context.Context, j Job, action string, detail map[string]any, extra func(pgx.Tx) error, set string, args ...any) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, `UPDATE research_jobs SET `+set+`,lease_until=NULL,updated_at=now() WHERE id=$1 AND status='RUNNING' AND attempts=$2`, append([]any{j.ID, j.Attempts}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errLeaseLost
	}
	if err = addEvent(ctx, tx, j.ID, action, detail); err != nil {
		return err
	}
	if extra != nil {
		if err = extra(tx); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) storeSources(ctx context.Context, j Job, sources []Source) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var owned bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM research_jobs WHERE id=$1 AND status='RUNNING' AND attempts=$2 FOR UPDATE)`, j.ID, j.Attempts).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return errLeaseLost
	}
	stored := make([]map[string]any, 0, len(sources))
	for _, src := range sources {
		if _, err = tx.Exec(ctx, `INSERT INTO research_sources(job_id,attempt,kind,sha256,url,received_at,body) VALUES($1,$2,$3,$4,$5,$6,$7)`, j.ID, j.Attempts, src.Kind, src.SHA256, src.URL, src.ReceivedAt, []byte(src.Body)); err != nil {
			return err
		}
		stored = append(stored, map[string]any{"kind": src.Kind, "sha256": src.SHA256, "size_bytes": len(src.Body)})
	}
	if err = addEvent(ctx, tx, j.ID, "SEC_SOURCES_STORED", map[string]any{"attempt": j.Attempts, "sources": stored}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

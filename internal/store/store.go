package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"tradestocker/db"
	"tradestocker/internal/paper"
)

type Store struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func Open(ctx context.Context, url string) (*Store, error) {
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &Store{Pool: p, Now: func() time.Time { return time.Now().UTC() }}, nil
}
func (s *Store) Close() { s.Pool.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Transaction-scoped lock also serializes simultaneous clean first starts.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(81342101)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, db.Migration+"\n"+db.ResearchMigration+"\n"+db.TraceMigration); err != nil {
		return err
	}
	b, err := json.Marshal(paper.Persisted{State: paper.NewState(), Receipts: map[string]paper.Receipt{}})
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO paper_workspace(id,document) VALUES(1,$1) ON CONFLICT DO NOTHING", b); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Read runs a read-only operation on the latest committed document, without the
// row lock and without rewriting it. If a reservation is due to expire, the
// expiry (refund, journal, audit, notice) is committed first through
// expireLocked, and the response shows it. Nothing is cached: every call reads
// PostgreSQL.
func (s *Store) Read(ctx context.Context, fn func(*paper.State, time.Time) (any, error)) ([]byte, error) {
	var raw []byte
	if err := s.Pool.QueryRow(ctx, "SELECT document FROM paper_workspace WHERE id=1").Scan(&raw); err != nil {
		return nil, err
	}
	state, err := decode(raw)
	if err != nil {
		return nil, err
	}
	now := s.Now()
	if state.ExpiryDue(now) {
		if state, now, err = s.expireLocked(ctx); err != nil {
			return nil, err
		}
	}
	out, err := fn(&state, now)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// expireLocked re-reads the document under the row lock and writes it only if
// that latest state still has expiry work. Readers that queued behind another
// request's expiry find nothing due and commit without an UPDATE, so each
// expiry is applied and written once.
func (s *Store) expireLocked(ctx context.Context) (paper.State, time.Time, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return paper.State{}, time.Time{}, err
	}
	defer tx.Rollback(context.Background())
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT document FROM paper_workspace WHERE id=1 FOR UPDATE").Scan(&raw); err != nil {
		return paper.State{}, time.Time{}, err
	}
	state, err := decode(raw)
	if err != nil {
		return paper.State{}, time.Time{}, err
	}
	now := s.Now()
	if state.ExpiryDue(now) {
		state.Expire(now)
		if err = state.Validate(); err != nil {
			return paper.State{}, time.Time{}, fmt.Errorf("expiry invariant: %w", err)
		}
		updated, err := json.Marshal(paper.Persisted{State: state, Receipts: state.Receipts})
		if err != nil {
			return paper.State{}, time.Time{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE paper_workspace SET document=$1,updated_at=now() WHERE id=1", updated); err != nil {
			return paper.State{}, time.Time{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return paper.State{}, time.Time{}, err
	}
	return state, now, nil
}

// decode loads a stored document and checks its invariants.
func decode(raw []byte) (paper.State, error) {
	var persisted paper.Persisted
	if err := json.Unmarshal(raw, &persisted); err != nil {
		return paper.State{}, err
	}
	state := persisted.State
	state.Receipts = persisted.Receipts
	if state.Receipts == nil {
		state.Receipts = map[string]paper.Receipt{}
	}
	if err := state.Validate(); err != nil {
		return paper.State{}, fmt.Errorf("stored invariant: %w", err)
	}
	return state, nil
}

// Execute commits the domain, journal, audit, dry-run outbox and replay receipt together.
// The locked singleton is intentionally a low-throughput private development aggregate.
func (s *Store) Execute(ctx context.Context, key, hash string, fn func(*paper.State, time.Time) (any, error)) ([]byte, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT document FROM paper_workspace WHERE id=1 FOR UPDATE").Scan(&raw); err != nil {
		return nil, err
	}
	var persisted paper.Persisted
	if err = json.Unmarshal(raw, &persisted); err != nil {
		return nil, err
	}
	state := persisted.State
	state.Receipts = persisted.Receipts
	if state.Receipts == nil {
		state.Receipts = map[string]paper.Receipt{}
	}
	if err = state.Validate(); err != nil {
		return nil, fmt.Errorf("stored invariant: %w", err)
	}
	now := s.Now()
	state.Expire(now)
	var body []byte
	if old, ok := state.Receipts[key]; key != "" && ok {
		if old.Hash != hash {
			return nil, paper.Fail("IDEMPOTENCY_CONFLICT", "Idempotency key was already used for a different request", 409)
		}
		body = old.Body
	} else {
		out, e := fn(&state, now)
		if e != nil {
			return nil, e
		}
		body, err = json.Marshal(out)
		if err != nil {
			return nil, err
		}
		if key != "" {
			state.Receipts[key] = paper.Receipt{Hash: hash, Body: body}
		}
	}
	if err = state.Validate(); err != nil {
		return nil, fmt.Errorf("mutation invariant: %w", err)
	}
	updated, err := json.Marshal(paper.Persisted{State: state, Receipts: state.Receipts})
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "UPDATE paper_workspace SET document=$1,updated_at=now() WHERE id=1", updated); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return body, nil
}

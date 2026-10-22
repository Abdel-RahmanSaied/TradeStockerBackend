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
	if _, err = tx.Exec(ctx, db.Migration); err != nil {
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

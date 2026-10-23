// Command s00t05 checks pgx and nats.go JetStream on the pinned versions: JetStream
// message-id de-duplication and an idempotent inbox consumer under simulated
// duplicates and redelivery, then measures local rates. It does not prove that a
// producer commits business rows and outbox rows atomically, nor that a durable
// relay recovers after a restart. All data is synthetic spike data.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type report struct {
	Versions          map[string]string `json:"versions"`
	Messages          int               `json:"messages"`
	OutboxInsertSecs  float64           `json:"outbox_insert_seconds"`
	PublishPerSecond  float64           `json:"publish_per_second"`
	PublishP50Millis  float64           `json:"publish_p50_ms"`
	PublishP95Millis  float64           `json:"publish_p95_ms"`
	Republished       int               `json:"republished"`
	DuplicatesFlagged int               `json:"duplicates_flagged_by_jetstream"`
	StreamMessages    uint64            `json:"stream_messages"`
	StreamBytes       uint64            `json:"stream_bytes"`
	UnackedOnPurpose  int               `json:"unacked_on_purpose"`
	Redeliveries      int               `json:"redeliveries"`
	InboxRows         int               `json:"inbox_rows"`
	EffectTotal       int64             `json:"effect_total"`
	ExpectedTotal     int64             `json:"expected_total"`
	ConsumePerSecond  float64           `json:"consume_per_second"`
	SampleStorage     map[string]int64  `json:"sample_storage_bytes,omitempty"`
	Failures          []string          `json:"failures"`
	Passed            bool              `json:"passed"`
}

func main() {
	r, err := run(context.Background())
	if err != nil {
		r.Failures = append(r.Failures, err.Error())
	}
	r.Passed = len(r.Failures) == 0
	out, _ := json.MarshalIndent(r, "", " ")
	fmt.Println(string(out))
	if !r.Passed {
		os.Exit(1)
	}
}

func run(ctx context.Context) (report, error) {
	r := report{Versions: map[string]string{}, Failures: []string{}}
	if info, ok := debug.ReadBuildInfo(); ok {
		r.Versions["go"] = info.GoVersion
		for _, m := range info.Deps {
			if m.Path == "github.com/jackc/pgx/v5" || m.Path == "github.com/nats-io/nats.go" {
				r.Versions[m.Path] = m.Version
			}
		}
	}
	n, err := strconv.Atoi(env("SPIKE_MESSAGES", "5000"))
	if err != nil || n < 100 {
		return r, errors.New("SPIKE_MESSAGES must be at least 100")
	}
	r.Messages = n
	pool, err := pgxpool.New(ctx, os.Getenv("SPIKE_PG_URL"))
	if err != nil {
		return r, err
	}
	defer pool.Close()
	var pg string
	if err = pool.QueryRow(ctx, `SHOW server_version`).Scan(&pg); err != nil {
		return r, err
	}
	r.Versions["postgresql"] = pg
	if _, err = pool.Exec(ctx, `DROP TABLE IF EXISTS spike_outbox, spike_inbox, spike_effect, spike_sample;
CREATE TABLE spike_outbox(id text PRIMARY KEY, amount bigint NOT NULL, published_at timestamptz);
CREATE TABLE spike_inbox(message_id text PRIMARY KEY, received_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE spike_effect(id int PRIMARY KEY CHECK (id = 1), total bigint NOT NULL);
INSERT INTO spike_effect VALUES (1, 0);`); err != nil {
		return r, err
	}

	// Outbox rows only (one COPY); there is no business table in this spike.
	start := time.Now()
	ids := make([]string, n)
	rows := make([][]any, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("evt-%06d", i)
		amount := int64(i%7 + 1)
		r.ExpectedTotal += amount
		rows[i] = []any{ids[i], amount}
	}
	if _, err = pool.CopyFrom(ctx, pgx.Identifier{"spike_outbox"}, []string{"id", "amount"}, pgx.CopyFromRows(rows)); err != nil {
		return r, err
	}
	r.OutboxInsertSecs = time.Since(start).Seconds()

	nc, err := nats.Connect(os.Getenv("SPIKE_NATS_URL"), nats.Name("tradestocker-spike-s00t05"))
	if err != nil {
		return r, err
	}
	defer nc.Close()
	r.Versions["nats-server"] = nc.ConnectedServerVersion()
	js, err := jetstream.New(nc)
	if err != nil {
		return r, err
	}
	_ = js.DeleteStream(ctx, "SPIKE_EVENTS")
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: "SPIKE_EVENTS", Subjects: []string{"spike.events"}, Storage: jetstream.FileStorage,
		Duplicates: 2 * time.Minute, MaxAge: time.Hour, MaxBytes: 256 << 20, Discard: jetstream.DiscardOld,
	})
	if err != nil {
		return r, err
	}

	// Publish each event with its id as the JetStream message id. Events come from the
	// in-memory list, not from a relay reading the outbox table.
	latencies := make([]time.Duration, 0, n)
	start = time.Now()
	for i, id := range ids {
		body, _ := json.Marshal(map[string]any{"id": id, "amount": i%7 + 1, "data_label": "SPIKE_SYNTHETIC"})
		t := time.Now()
		if _, err = js.Publish(ctx, "spike.events", body, jetstream.WithMsgID(id)); err != nil {
			return r, err
		}
		latencies = append(latencies, time.Since(t))
	}
	elapsed := time.Since(start)
	r.PublishPerSecond = float64(n) / elapsed.Seconds()
	sort.Slice(latencies, func(a, b int) bool { return latencies[a] < latencies[b] })
	r.PublishP50Millis = ms(latencies[n/2])
	r.PublishP95Millis = ms(latencies[n*95/100])
	if _, err = pool.Exec(ctx, `UPDATE spike_outbox SET published_at=now()`); err != nil {
		return r, err
	}
	// Simulate a relay that publishes again after a crash: repeat every tenth id.
	for i := 0; i < n; i += 10 {
		body, _ := json.Marshal(map[string]any{"id": ids[i], "amount": i%7 + 1, "data_label": "SPIKE_SYNTHETIC"})
		ack, err := js.Publish(ctx, "spike.events", body, jetstream.WithMsgID(ids[i]))
		if err != nil {
			return r, err
		}
		r.Republished++
		if ack.Duplicate {
			r.DuplicatesFlagged++
		}
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return r, err
	}
	r.StreamMessages, r.StreamBytes = info.State.Msgs, info.State.Bytes

	// The consumer commits the effect through the inbox, then acks. Some acks are
	// skipped on purpose (a crash after commit), so those messages come again.
	cons, err := js.CreateOrUpdateConsumer(ctx, "SPIKE_EVENTS", jetstream.ConsumerConfig{
		Durable: "spike-inbox", AckPolicy: jetstream.AckExplicitPolicy, AckWait: time.Second, MaxDeliver: 5,
	})
	if err != nil {
		return r, err
	}
	start = time.Now()
	deadline := time.Now().Add(2 * time.Minute)
	for r.InboxRows < n || r.Redeliveries < r.UnackedOnPurpose {
		if time.Now().After(deadline) {
			r.Failures = append(r.Failures, "consumer did not finish within two minutes")
			break
		}
		batch, err := cons.Fetch(500, jetstream.FetchMaxWait(500*time.Millisecond))
		if err != nil {
			return r, err
		}
		for msg := range batch.Messages() {
			meta, err := msg.Metadata()
			if err != nil {
				return r, err
			}
			var event struct {
				ID     string `json:"id"`
				Amount int64  `json:"amount"`
			}
			if err = json.Unmarshal(msg.Data(), &event); err != nil || event.ID != msg.Headers().Get("Nats-Msg-Id") {
				return r, fmt.Errorf("bad event %q", msg.Data())
			}
			if err = applyOnce(ctx, pool, event.ID, event.Amount); err != nil {
				return r, err
			}
			if meta.NumDelivered > 1 {
				r.Redeliveries++
			}
			seq := meta.Sequence.Stream
			if meta.NumDelivered == 1 && seq%13 == 0 {
				r.UnackedOnPurpose++
				continue
			}
			if err = msg.Ack(); err != nil {
				return r, err
			}
		}
		if err = batch.Error(); err != nil && !errors.Is(err, jetstream.ErrNoMessages) {
			return r, err
		}
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM spike_inbox`).Scan(&r.InboxRows); err != nil {
			return r, err
		}
	}
	r.ConsumePerSecond = float64(n) / time.Since(start).Seconds()
	if err = pool.QueryRow(ctx, `SELECT total FROM spike_effect`).Scan(&r.EffectTotal); err != nil {
		return r, err
	}
	check := func(ok bool, what string) {
		if !ok {
			r.Failures = append(r.Failures, what)
		}
	}
	check(r.StreamMessages == uint64(n), fmt.Sprintf("stream holds %d messages, want %d (duplicates must be dropped)", r.StreamMessages, n))
	check(r.DuplicatesFlagged == r.Republished, fmt.Sprintf("JetStream flagged %d of %d republished messages", r.DuplicatesFlagged, r.Republished))
	check(r.InboxRows == n, fmt.Sprintf("inbox has %d rows, want %d", r.InboxRows, n))
	check(r.EffectTotal == r.ExpectedTotal, fmt.Sprintf("effect total %d, want %d (no double effect)", r.EffectTotal, r.ExpectedTotal))
	check(r.UnackedOnPurpose > 0 && r.Redeliveries >= r.UnackedOnPurpose, fmt.Sprintf("%d unacked, %d redelivered", r.UnackedOnPurpose, r.Redeliveries))
	if dir := os.Getenv("SPIKE_SAMPLE_DIR"); dir != "" {
		if r.SampleStorage, err = sampleStorage(ctx, pool, dir); err != nil {
			return r, err
		}
	}
	_ = js.DeleteStream(ctx, "SPIKE_EVENTS")
	return r, nil
}

// applyOnce records the message id and applies its effect in one transaction.
func applyOnce(ctx context.Context, pool *pgxpool.Pool, id string, amount int64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, `INSERT INTO spike_inbox(message_id) VALUES($1) ON CONFLICT DO NOTHING`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		if _, err = tx.Exec(ctx, `UPDATE spike_effect SET total=total+$1`, amount); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// sampleStorage measures how much PostgreSQL stores for one research job's
// original sources (bytea, TOAST-compressed), using real downloaded SEC files.
func sampleStorage(ctx context.Context, pool *pgxpool.Pool, dir string) (map[string]int64, error) {
	out := map[string]int64{}
	if _, err := pool.Exec(ctx, `CREATE TABLE spike_sample(kind text PRIMARY KEY, body bytea NOT NULL)`); err != nil {
		return nil, err
	}
	for _, kind := range []string{"submissions", "companyfacts"} {
		body, err := os.ReadFile(filepath.Join(dir, kind+".json"))
		if err != nil {
			return nil, err
		}
		out[kind+"_raw"] = int64(len(body))
		if _, err = pool.Exec(ctx, `INSERT INTO spike_sample VALUES($1,$2)`, kind, body); err != nil {
			return nil, err
		}
	}
	var total int64
	if err := pool.QueryRow(ctx, `SELECT pg_total_relation_size('spike_sample')`).Scan(&total); err != nil {
		return nil, err
	}
	out["table_on_disk"] = total
	return out, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// Package natsbus sends research job notifications over core NATS. A
// notification is only a hint that wakes an idle worker sooner: every job is
// committed to PostgreSQL first and workers also poll PostgreSQL, so a lost,
// late or repeated message changes nothing but latency. It is not an outbox and
// carries no job or financial state, only a job ID and the trace header.
package natsbus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/nats-io/nats.go"

	"tradestocker/internal/telemetry"
)

// Subject carries one research job ID per message.
const Subject = "tradestocker.research.job.created"

// Bus is one NATS connection. It reconnects on its own; while it is
// disconnected, publishing fails at once (nothing is buffered for later).
type Bus struct {
	conn   *nats.Conn
	closed chan struct{}
}

var errURL = errors.New("TS_NATS_URL must be nats://host:port, with no credentials, path or query")

// CheckURL accepts only nats://host:port. Its error never repeats the value,
// which could hold a credential.
func CheckURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "nats" || u.User != nil || u.Hostname() == "" || u.Port() == "" ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return errURL
	}
	return nil
}

// Connect sets up the connection. If NATS is not reachable yet it keeps
// trying in the background, so platform-core starts without it; Ready
// reports whether it is connected.
func Connect(raw, name string) (*Bus, error) {
	if err := CheckURL(raw); err != nil {
		return nil, err
	}
	b := &Bus{closed: make(chan struct{})}
	conn, err := nats.Connect(raw,
		nats.Name(name),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second),
		nats.ReconnectBufSize(-1),
		nats.Timeout(2*time.Second),
		nats.DrainTimeout(5*time.Second),
		nats.ConnectHandler(func(*nats.Conn) { slog.Info("nats connected") }),
		nats.ReconnectHandler(func(*nats.Conn) { slog.Info("nats reconnected") }),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) { slog.Warn("nats disconnected", "error", err) }),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) { slog.Warn("nats error", "error", err) }),
		nats.ClosedHandler(func(*nats.Conn) { close(b.closed) }),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connection could not be set up: %w", err)
	}
	b.conn = conn
	return b, nil
}

// Ready reports whether the connection is up now.
func (b *Bus) Ready() bool { return b.conn.IsConnected() }

// JobCreated publishes jobID in ctx's trace (as a traceparent header) and waits
// briefly until the server has it. An error means the hint may not arrive; the
// caller logs it and keeps the committed job.
func (b *Bus) JobCreated(ctx context.Context, jobID string) error {
	msg := nats.NewMsg(Subject)
	msg.Data = []byte(jobID)
	telemetry.Inject(ctx, msg.Header.Set)
	if err := b.conn.PublishMsg(msg); err != nil {
		return err
	}
	flush, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	return b.conn.FlushWithContext(flush)
}

// Subscribe calls wake for every notification, in the publisher's trace (a new
// span), or in a new trace if the header is missing or malformed. Every
// platform-core instance gets every message; only the instance whose workers
// lead acts on it, and the receiver checks the job ID again.
func (b *Bus) Subscribe(wake func(ctx context.Context, jobID string)) error {
	_, err := b.conn.Subscribe(Subject, func(m *nats.Msg) {
		ctx, _ := telemetry.Continue(context.Background(), m.Header.Get("traceparent"))
		wake(ctx, string(m.Data))
	})
	return err
}

// Close drains the connection: notifications already received are handled and
// the subscription ends, within the drain timeout; then the connection closes.
func (b *Bus) Close() {
	if err := b.conn.Drain(); err != nil {
		b.conn.Close()
	}
	timer := time.NewTimer(7 * time.Second)
	defer timer.Stop()
	select {
	case <-b.closed:
	case <-timer.C:
		b.conn.Close()
	}
}

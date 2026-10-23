// Package telemetry carries W3C trace context through HTTP requests, NATS
// messages and jobs, and writes structured JSON logs that never contain
// registered secrets. It uses only the standard library; spans are written as
// log events, and no trace exporter is configured.
package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
)

// Trace identifies one unit of work: TraceID is shared by everything caused by
// the same request; SpanID names this step.
type Trace struct {
	TraceID string
	SpanID  string
}

var traceparent = regexp.MustCompile(`^00-([0-9a-f]{32})-([0-9a-f]{16})-[0-9a-f]{2}$`)

func randomHex(n int) string {
	for {
		b := make([]byte, n)
		if _, err := rand.Read(b); err != nil {
			panic(err) // crypto/rand does not fail on supported systems
		}
		if s := hex.EncodeToString(b); !allZero(s) {
			return s
		}
	}
}

func allZero(s string) bool {
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}

// NewTrace starts a new trace.
func NewTrace() Trace { return Trace{TraceID: randomHex(16), SpanID: randomHex(8)} }

// Child is the next step of the same trace.
func (t Trace) Child() Trace { return Trace{TraceID: t.TraceID, SpanID: randomHex(8)} }

// Traceparent is the W3C header value for this step (always sampled).
func (t Trace) Traceparent() string { return "00-" + t.TraceID + "-" + t.SpanID + "-01" }

// ParseTraceparent accepts only a well-formed version-00 header with non-zero IDs.
func ParseTraceparent(s string) (Trace, bool) {
	m := traceparent.FindStringSubmatch(s)
	if m == nil || allZero(m[1]) || allZero(m[2]) {
		return Trace{}, false
	}
	return Trace{TraceID: m[1], SpanID: m[2]}, true
}

// ValidTraceID reports whether s is a usable 32-hex-digit trace ID.
func ValidTraceID(s string) bool {
	_, ok := ParseTraceparent("00-" + s + "-0000000000000001-01")
	return ok
}

type traceKey struct{}

// WithTrace stores t in ctx.
func WithTrace(ctx context.Context, t Trace) context.Context {
	return context.WithValue(ctx, traceKey{}, t)
}

// FromContext returns the trace stored in ctx.
func FromContext(ctx context.Context) (Trace, bool) {
	t, ok := ctx.Value(traceKey{}).(Trace)
	return t, ok
}

// Continue starts the next step after a traceparent received from elsewhere
// (an HTTP header or a NATS message header). An absent or invalid header
// starts a new trace.
func Continue(ctx context.Context, header string) (context.Context, Trace) {
	t, ok := ParseTraceparent(header)
	if ok {
		t = t.Child()
	} else {
		t = NewTrace()
	}
	return WithTrace(ctx, t), t
}

// Inject writes ctx's trace, if any, as a traceparent header with set.
func Inject(ctx context.Context, set func(key, value string)) {
	if t, ok := FromContext(ctx); ok {
		set("traceparent", t.Traceparent())
	}
}

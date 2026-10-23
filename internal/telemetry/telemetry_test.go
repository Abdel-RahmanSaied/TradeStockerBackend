package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/slogtest"
)

// Credential-shaped values are assembled at run time from separate pieces, so
// no test file holds a literal that a secret scanner would rightly flag.
func randomFrom(alphabet string, n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

const alnum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func syntheticCredentials() map[string]string {
	return map[string]string{
		"telegram bot token": "1" + randomFrom("0123456789", 9) + ":" + randomFrom(alnum+"_-", 35),
		"github token":       "gh" + "p" + "_" + randomFrom(alnum, 36),
		"aws access key id":  "AK" + "IA" + randomFrom("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", 16),
		"bearer header":      "Bea" + "rer " + randomFrom(alnum, 40),
	}
}

func logTo(r *Redactor) (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return NewLogger(&buf, r, slog.LevelDebug), &buf
}

func TestTraceparentRoundTripAndStrictParsing(t *testing.T) {
	tr := NewTrace()
	got, ok := ParseTraceparent(tr.Traceparent())
	if !ok || got != tr || len(tr.TraceID) != 32 || len(tr.SpanID) != 16 {
		t.Fatalf("round trip: %+v %v", got, ok)
	}
	child := tr.Child()
	if child.TraceID != tr.TraceID || child.SpanID == tr.SpanID {
		t.Fatalf("child %+v of %+v", child, tr)
	}
	zeroTrace := "00-" + strings.Repeat("0", 32) + "-" + tr.SpanID + "-01"
	zeroSpan := "00-" + tr.TraceID + "-" + strings.Repeat("0", 16) + "-01"
	for _, bad := range []string{"", "garbage", zeroTrace, zeroSpan, "01-" + tr.TraceID + "-" + tr.SpanID + "-01",
		"00-" + strings.ToUpper(tr.TraceID) + "-" + tr.SpanID + "-01", tr.Traceparent() + "-x", tr.Traceparent() + "\n"} {
		if _, ok := ParseTraceparent(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
	if !ValidTraceID(tr.TraceID) || ValidTraceID("xyz") || ValidTraceID(strings.Repeat("0", 32)) {
		t.Fatal("ValidTraceID")
	}
}

func TestContinueKeepsTheTraceOrStartsANewOne(t *testing.T) {
	parent := NewTrace()
	ctx, tr := Continue(context.Background(), parent.Traceparent())
	if tr.TraceID != parent.TraceID || tr.SpanID == parent.SpanID {
		t.Fatalf("continued %+v from %+v", tr, parent)
	}
	if got, _ := FromContext(ctx); got != tr {
		t.Fatal("trace not in context")
	}
	_, fresh := Continue(context.Background(), "not a header")
	if fresh.TraceID == parent.TraceID || !ValidTraceID(fresh.TraceID) {
		t.Fatal("invalid header did not start a new trace")
	}
	headers := map[string]string{}
	Inject(ctx, func(k, v string) { headers[k] = v })
	if headers["traceparent"] != tr.Traceparent() {
		t.Fatalf("inject: %v", headers)
	}
}

func TestRegisteredSecretsAndCredentialShapesNeverReachTheLog(t *testing.T) {
	r := &Redactor{}
	apiToken := randomFrom(alnum, 48)
	dbPassword := randomFrom(alnum, 32)
	r.Add(apiToken, dbPassword, "short")
	logger, buf := logTo(r)
	creds := syntheticCredentials()
	dsn := "postgres://app:" + dbPassword + "@db:5432/x"
	other := "nats://u:" + randomFrom(alnum, 20) + "@nats:4222"
	logger.Info("startup "+apiToken, "note", "token is "+apiToken, "dsn_text", "url "+dsn, "other", other,
		"err", errors.New("connect failed for "+dsn), "word", "short")
	for _, c := range creds {
		logger.Warn("seen", "value", "x "+c+" y", "group", slog.GroupValue(slog.String("inner", c)))
	}
	out := buf.String()
	for _, secret := range append([]string{apiToken, dbPassword, other[strings.Index(other, ":")+3 : strings.Index(other, "@")]}, valuesOf(creds)...) {
		if strings.Contains(out, secret) {
			t.Fatalf("secret reached the log:\n%s", out)
		}
	}
	if !strings.Contains(out, "short") {
		t.Fatal("values under 8 bytes must not be blanked everywhere")
	}
	if !strings.Contains(out, `postgres://app:[REDACTED]@db:5432/x`) {
		t.Fatalf("URL password not replaced in place:\n%s", out)
	}
}

func valuesOf(m map[string]string) []string {
	var out []string
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func TestSecretAndContentKeysAreCleanedAtAnyDepth(t *testing.T) {
	logger, buf := logTo(&Redactor{})
	document := `{"facts":{"Revenue":[1,2,3]}}` // short on purpose: content is omitted whatever its length
	logger.Info("event",
		"Authorization", "anything", "bot_token", "anything", "db-password", "anything",
		"body", document, "source_document", document, "companyfacts", document, "licensed", Licensed(document),
		"detail", map[string]any{"user_agent": "TradeStocker contact@example.invalid", "nested": map[string]any{"api_key": "k", "payload": document, "ok": "kept"},
			"list": []any{map[string]any{"secret": "s"}, "plain"}},
		"group", slog.GroupValue(slog.String("password", "p"), slog.String("content", document), slog.String("fine", "visible")),
	)
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	for _, leaked := range []string{"anything", "Revenue", "contact@example.invalid", `"k"`, `"s"`, `"p"`} {
		if strings.Contains(text, leaked) {
			t.Fatalf("%s reached the log:\n%s", leaked, text)
		}
	}
	for _, kept := range []string{"kept", "plain", "visible", fmt.Sprintf("[omitted: %d bytes]", len(document))} {
		if !strings.Contains(text, kept) {
			t.Fatalf("%q missing from:\n%s", kept, text)
		}
	}
}

// Regression for the review of the first draft: typed maps, typed slices,
// structs and content-keyed groups must not bypass the key rules, and short
// content must be omitted too. Plain owned fixture strings only.
func TestTypedValuesAndContentGroupsAreCleaned(t *testing.T) {
	type nested struct {
		Name     string            `json:"name"`
		Password string            `json:"password"`
		Body     string            `json:"body"`
		Extra    map[string]string `json:"extra"`
	}
	logger, buf := logTo(&Redactor{})
	short := "fixture-filing-text"
	logger.Info("event",
		"typed_list", []map[string]string{{"secret": "fixture-secret-a", "body": short, "label": "kept-1"}},
		"typed_map", map[string]map[string]string{"outer": {"api_key": "fixture-secret-b", "document": short, "label": "kept-2"}},
		"struct", nested{Name: "kept-3", Password: "fixture-secret-c", Body: short, Extra: map[string]string{"token": "fixture-secret-d", "label": "kept-4"}},
		"mixed", map[string]any{"rows": []any{map[string]string{"content": short}, []string{"kept-5"}}},
		"payload", slog.GroupValue(slog.String("label", "fixture-hidden-group"), slog.String("x", short)),
		"strings", []string{"kept-6"},
	)
	text := buf.String()
	for _, leaked := range []string{short, "fixture-secret-a", "fixture-secret-b", "fixture-secret-c", "fixture-secret-d", "fixture-hidden-group"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("%q reached the log:\n%s", leaked, text)
		}
	}
	for _, kept := range []string{"kept-1", "kept-2", "kept-3", "kept-4", "kept-5", "kept-6", "[omitted: group of 2 fields]"} {
		if !strings.Contains(text, kept) {
			t.Fatalf("%q missing from:\n%s", kept, text)
		}
	}
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	logger.Info("odd", "channel", make(chan int))
	if !strings.Contains(buf.String(), "[unloggable chan int]") {
		t.Fatalf("unencodable value: %s", buf.String())
	}
}

// Regression for the group reviews: a group opened with WithGroup whose name is
// a secret or content key hides everything inside it, whether the values come
// from the record, from WithAttrs, from a nested group or from a group value.
func TestGroupScopesFollowTheKeyRules(t *testing.T) {
	short := "fixture-filing-text"
	cases := map[string]func(*slog.Logger){
		"content group, record attr":    func(l *slog.Logger) { l.WithGroup("payload").Info("m", "text", short) },
		"content group, WithAttrs":      func(l *slog.Logger) { l.WithGroup("payload").With("text", short).Info("m") },
		"content group inside a group":  func(l *slog.Logger) { l.WithGroup("outer").WithGroup("source_document").Info("m", "text", short) },
		"content group, then a group":   func(l *slog.Logger) { l.WithGroup("document").WithGroup("inner").With("x", short).Info("m") },
		"content group, group value":    func(l *slog.Logger) { l.WithGroup("payload").Info("m", slog.Group("", slog.String("x", short))) },
		"secret group, record attr":     func(l *slog.Logger) { l.WithGroup("credentials").Info("m", "value", short) },
		"secret group, WithAttrs":       func(l *slog.Logger) { l.WithGroup("authorization").With("value", short).Info("m") },
		"ordinary group, protected key": func(l *slog.Logger) { l.WithGroup("ordinary").With("body", short).Info("m") },
		"ordinary group, typed value": func(l *slog.Logger) {
			l.WithGroup("ordinary").Info("m", "rows", []map[string]string{{"content": short}})
		},
	}
	for name, emit := range cases {
		t.Run(name, func(t *testing.T) {
			logger, buf := logTo(&Redactor{})
			emit(logger)
			if strings.Contains(buf.String(), short) {
				t.Fatalf("the fixture reached the log:\n%s", buf)
			}
		})
	}

	// Ordinary groups keep their shape and the trace stays at the top level,
	// also next to a content group.
	logger, buf := logTo(&Redactor{})
	ctx := WithTrace(context.Background(), NewTrace())
	logger.WithGroup("payload").InfoContext(ctx, "content scope", "text", short)
	logger.WithGroup("plain").With("k", "kept-a").WithGroup("deeper").InfoContext(ctx, "ordinary", "k2", "kept-b")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var first, second map[string]any
	if json.Unmarshal([]byte(lines[0]), &first) != nil || json.Unmarshal([]byte(lines[1]), &second) != nil {
		t.Fatalf("not JSON lines:\n%s", buf)
	}
	if first["trace_id"] == nil || first["payload"].(map[string]any)["text"] != "[omitted: inside a content group]" {
		t.Fatalf("content scope or top-level trace: %v", first)
	}
	plain := second["plain"].(map[string]any)
	if second["trace_id"] == nil || plain["k"] != "kept-a" || plain["deeper"].(map[string]any)["k2"] != "kept-b" {
		t.Fatalf("ordinary nesting changed: %v", second)
	}
}

// Regression for the Licensed review: the marker stays a marker inside lists,
// structs and maps that are turned into JSON, and in fmt output.
func TestLicensedStaysOmittedInsideValuesAndText(t *testing.T) {
	owned := Licensed("fixture-filing-text")
	values := map[string]any{
		"list":                []Licensed{owned},
		"struct":              struct{ Rows []Licensed }{[]Licensed{owned}},
		"map":                 map[string]Licensed{"k": owned},
		"under a content key": owned,
	}
	base64Form := "Zml4dHVyZS1maWxpbmctdGV4dA" // fixture-filing-text, base64 without padding
	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			logger, buf := logTo(&Redactor{})
			key := "items"
			if name == "under a content key" {
				key = "body"
			}
			logger.Info("m", key, value)
			if strings.Contains(buf.String(), string(owned)) || strings.Contains(buf.String(), base64Form) {
				t.Fatalf("licensed content reached the log:\n%s", buf)
			}
			if !strings.Contains(buf.String(), "[omitted: 19 bytes]") {
				t.Fatalf("marker missing or wrong size:\n%s", buf)
			}
		})
	}
	for _, verb := range []string{"%s", "%v", "%x", "%d", "%q", "%#v"} {
		if got := fmt.Sprintf(verb, owned); got != "[omitted: 19 bytes]" {
			t.Fatalf("fmt %s printed %q", verb, got)
		}
	}
	if err := fmt.Errorf("parse: %v", owned); strings.Contains(err.Error(), string(owned)) {
		t.Fatalf("wrapped error kept the content: %v", err)
	}
}

// The handler keeps its own group nesting, so check it still follows the
// standard slog.Handler rules (empty groups, inline groups, WithAttrs order).
func TestHandlerFollowsTheSlogHandlerContract(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(slog.NewJSONHandler(&buf, nil), &Redactor{})
	results := func() []map[string]any {
		var out []map[string]any
		for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatalf("not JSON: %s", line)
			}
			out = append(out, m)
		}
		return out
	}
	if err := slogtest.TestHandler(h, results); err != nil {
		t.Fatal(err)
	}
}

func TestLongTextIsCut(t *testing.T) {
	logger, buf := logTo(&Redactor{})
	logger.Info("event", "stderr", strings.Repeat("x", 5000))
	if strings.Count(buf.String(), "x") > MaxText+10 || !strings.Contains(buf.String(), "[cut: 5000 bytes]") {
		t.Fatalf("not cut: %d bytes", buf.Len())
	}
}

func TestLogLinesCarryTheTraceFromTheContext(t *testing.T) {
	logger, buf := logTo(&Redactor{})
	tr := NewTrace()
	logger.InfoContext(WithTrace(context.Background(), tr), "step")
	logger.With("component", "x").InfoContext(context.Background(), "no trace")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var first, second map[string]any
	_ = json.Unmarshal([]byte(lines[0]), &first)
	_ = json.Unmarshal([]byte(lines[1]), &second)
	if first["trace_id"] != tr.TraceID || first["span_id"] != tr.SpanID {
		t.Fatalf("trace missing: %v", first)
	}
	if _, ok := second["trace_id"]; ok || second["component"] != "x" {
		t.Fatalf("unexpected: %v", second)
	}
}

func TestHTTPMiddlewarePropagatesTheTraceAndLogsNoQueryOrHeaders(t *testing.T) {
	logger, buf := logTo(&Redactor{})
	var seen Trace
	handler := HTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = FromContext(r.Context())
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hello"))
	}), logger)
	parent := NewTrace()
	secret := randomFrom(alnum, 40)
	r := httptest.NewRequest("GET", "/api/v1/dashboard?token="+secret, nil)
	r.Header.Set("traceparent", parent.Traceparent())
	r.Header.Set("Authorization", "Bearer "+secret)
	r.Header.Set("X-Request-Id", "req-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if seen.TraceID != parent.TraceID || seen.SpanID == parent.SpanID {
		t.Fatalf("handler trace %+v from %+v", seen, parent)
	}
	if w.Header().Get("X-Trace-Id") != parent.TraceID || w.Header().Get("traceparent") != seen.Traceparent() || w.Header().Get("X-Request-Id") != "req-1" {
		t.Fatalf("response headers %v", w.Header())
	}
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["path"] != "/api/v1/dashboard" || rec["status"].(float64) != 418 || rec["bytes"].(float64) != 5 || rec["trace_id"] != parent.TraceID {
		t.Fatalf("log %v", rec)
	}
	if strings.Contains(buf.String(), secret) || strings.Contains(buf.String(), "Authorization") {
		t.Fatalf("query or header reached the log: %s", buf.String())
	}
	bad := httptest.NewRequest("GET", "/x", nil)
	bad.Header.Set("X-Request-Id", "not ok\nnewline")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, bad)
	if w.Header().Get("X-Request-Id") != "" || w.Header().Get("X-Trace-Id") == parent.TraceID {
		t.Fatal("invalid request id echoed or trace reused")
	}
}

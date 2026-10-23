package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tradestocker/internal/paper"
	"tradestocker/internal/telemetry"
)

type fakeStore struct {
	state           paper.State
	reads, executes int
	readMutated     bool
}

func (f *fakeStore) Execute(ctx context.Context, k, h string, fn func(*paper.State, time.Time) (any, error)) ([]byte, error) {
	f.executes++
	out, e := fn(&f.state, time.Now().UTC())
	if e != nil {
		return nil, e
	}
	return json.Marshal(out)
}

// Read also records whether a read handler changed the state it was given.
func (f *fakeStore) Read(ctx context.Context, fn func(*paper.State, time.Time) (any, error)) ([]byte, error) {
	f.reads++
	before, _ := json.Marshal(f.state)
	out, e := fn(&f.state, time.Now().UTC())
	if after, _ := json.Marshal(f.state); string(after) != string(before) {
		f.readMutated = true
	}
	if e != nil {
		return nil, e
	}
	return json.Marshal(out)
}

func TestOtherMethodsNeverReachAHandler(t *testing.T) {
	token := strings.Repeat("a", 32)
	store := &fakeStore{state: paper.NewState()}
	handler := (&API{Store: store, Token: token}).Handler()
	for _, method := range []string{"PUT", "DELETE", "PATCH", "OPTIONS"} {
		r := httptest.NewRequest(method, "/api/v1/dashboard", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: got %d", method, w.Code)
		}
	}
	if store.reads != 0 || store.executes != 0 {
		t.Fatalf("unsupported methods reached the store: reads=%d executes=%d", store.reads, store.executes)
	}
}

func TestConfiguredDashboardOriginReplacesTheDefault(t *testing.T) {
	token := strings.Repeat("a", 32)
	handler := (&API{Store: &fakeStore{state: paper.NewState()}, Token: token, Origins: []string{"http://127.0.0.1:15173"}}).Handler()
	for origin, want := range map[string]int{
		"http://127.0.0.1:15173": http.StatusOK,
		"http://127.0.0.1:5173":  http.StatusForbidden,
		"http://localhost:15173": http.StatusForbidden,
		"https://evil.invalid":   http.StatusForbidden,
		"":                       http.StatusOK,
	} {
		r := httptest.NewRequest("GET", "/api/v1/dashboard", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("origin %q: got %d want %d", origin, w.Code, want)
		}
	}
}

func TestReadsNeverTakeTheWritePath(t *testing.T) {
	token := strings.Repeat("a", 32)
	for _, tc := range []struct {
		method, path, body string
		read               bool
	}{
		{"GET", "/api/v1/health", "", true},
		{"GET", "/api/v1/dashboard", "", true},
		{"GET", "/api/v1/risk/policy", "", true},
		// Go's router serves HEAD on GET routes; HEAD must never write.
		{"HEAD", "/api/v1/health", "", true},
		{"HEAD", "/api/v1/dashboard", "", true},
		{"HEAD", "/api/v1/risk/policy", "", true},
		{"POST", "/api/v1/fixtures", "{}", false},
		{"POST", "/api/v1/paper/entries", `{"paused":true}`, false},
		{"POST", "/api/v1/research/assessments", `{"symbol":"AAPL"}`, false},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			store := &fakeStore{state: paper.NewState()}
			handler := (&API{Store: store, Token: token}).Handler()
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Idempotency-Key", "path-check-key")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("got %d: %s", w.Code, w.Body)
			}
			if tc.read && (store.reads != 1 || store.executes != 0) || !tc.read && (store.reads != 0 || store.executes != 1) {
				t.Fatalf("reads=%d executes=%d", store.reads, store.executes)
			}
			if store.readMutated {
				t.Fatal("a read handler changed paper state")
			}
		})
	}
}
func TestHTTPBoundary(t *testing.T) {
	token := strings.Repeat("a", 32)
	research := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := (&API{Store: &fakeStore{state: paper.NewState()}, Token: token, Research: research}).Handler()
	for _, tc := range []struct {
		name, method, path, body, auth, key, origin string
		want                                        int
	}{
		{"health", "GET", "/api/v1/health", "", "", "", "", 200},
		{"no token", "GET", "/api/v1/dashboard", "", "", "", "", 401},
		{"wrong token", "GET", "/api/v1/dashboard", "", "wrong", "", "", 401},
		{"dashboard", "GET", "/api/v1/dashboard", "", token, "", "", 200},
		{"cross origin", "GET", "/api/v1/dashboard", "", token, "", "https://evil.invalid", 403},
		{"missing idempotency", "POST", "/api/v1/fixtures", "{}", token, "", "", 422},
		{"unknown actual field", "POST", "/api/v1/fixtures", `{"mode":"ACTUAL"}`, token, "test-key-01", "", 422},
		{"trailing json", "POST", "/api/v1/fixtures", `{} {}`, token, "test-key-02", "", 422},
		{"bounded body", "POST", "/api/v1/fixtures", strings.Repeat("x", 65537), token, "test-key-03", "", 413},
		{"fixture", "POST", "/api/v1/fixtures", "{}", token, "test-key-04", "", 200},
		{"no broker endpoint", "POST", "/api/v1/broker/orders", "{}", token, "test-key-05", "", 404},
		{"research needs token", "GET", "/api/v1/research/status", "", "", "", "", 401},
		{"research wrong token", "GET", "/api/v1/research/jobs", "", "wrong", "", "", 401},
		{"research cross origin", "GET", "/api/v1/research/status", "", token, "", "https://evil.invalid", 403},
		{"research authorized", "GET", "/api/v1/research/status", "", token, "", "", 200},
		{"research source needs token", "GET", "/api/v1/research/jobs/x/sources/companyfacts", "", "", "", "", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			if tc.auth != "" {
				r.Header.Set("Authorization", "Bearer "+tc.auth)
			}
			r.Header.Set("Idempotency-Key", tc.key)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d body %s", w.Code, tc.want, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable auth response")
			}
		})
	}
}

// Liveness checks nothing; readiness follows the dependencies, never shows
// error text, and turns 503 while draining. Both answer without the token.
func TestLivenessAndReadiness(t *testing.T) {
	secret := "fixture-dependency-detail"
	state := map[string]string{"postgres": DependencyOK, "nats": DependencyOK}
	var draining atomic.Bool
	api := &API{Store: &fakeStore{state: paper.NewState()}, Token: strings.Repeat("a", 32), Draining: &draining,
		Dependencies: []Dependency{
			{Name: "postgres", Required: true, Check: func(context.Context) string { return state["postgres"] }},
			{Name: "nats", Check: func(context.Context) string { return state["nats"] }},
		}}
	h := api.Handler()
	call := func(path string) (int, map[string]any) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("%s leaked dependency detail: %s", path, w.Body)
		}
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}
	for _, tc := range []struct {
		postgres, nats string
		draining       bool
		code           int
		status         string
	}{
		{DependencyOK, DependencyOK, false, 200, "ready"},
		{DependencyOK, DependencyNotConfigured, false, 200, "ready"},
		{DependencyOK, DependencyUnavailable, false, 200, "degraded"},
		{DependencyUnavailable, DependencyOK, false, 503, "not_ready"},
		{DependencyNotConfigured, DependencyOK, false, 503, "not_ready"},
		{secret, DependencyOK, false, 503, "not_ready"}, // unknown values count as unavailable
		{DependencyOK, DependencyOK, true, 503, "draining"},
	} {
		state["postgres"], state["nats"] = tc.postgres, tc.nats
		draining.Store(tc.draining)
		code, body := call("/api/v1/ready")
		if code != tc.code || body["status"] != tc.status {
			t.Fatalf("%+v: got %d %v", tc, code, body)
		}
		if live, _ := call("/api/v1/live"); live != 200 {
			t.Fatalf("liveness followed a dependency: %d in %+v", live, tc)
		}
	}
	state["postgres"] = secret
	draining.Store(false)
	if _, body := call("/api/v1/ready"); body["checks"].(map[string]any)["postgres"] != DependencyUnavailable {
		t.Fatalf("checks %v", body["checks"])
	}
}

func TestEveryResponseCarriesTheTrace(t *testing.T) {
	h := (&API{Store: &fakeStore{state: paper.NewState()}, Token: strings.Repeat("a", 32)}).Handler()
	parent := telemetry.NewTrace()
	for _, path := range []string{"/api/v1/live", "/api/v1/health", "/api/v1/dashboard"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("traceparent", parent.Traceparent())
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got, ok := telemetry.ParseTraceparent(w.Header().Get("traceparent")); !ok || got.TraceID != parent.TraceID || w.Header().Get("X-Trace-Id") != parent.TraceID {
			t.Fatalf("%s: traceparent %q", path, w.Header().Get("traceparent"))
		}
	}
}

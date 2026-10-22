package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"tradestocker/internal/paper"
)

type fakeStore struct{ state paper.State }

func (f *fakeStore) Execute(ctx context.Context, k, h string, fn func(*paper.State, time.Time) (any, error)) ([]byte, error) {
	out, e := fn(&f.state, time.Now().UTC())
	if e != nil {
		return nil, e
	}
	return json.Marshal(out)
}
func TestHTTPBoundary(t *testing.T) {
	token := strings.Repeat("a", 32)
	handler := (&API{Store: &fakeStore{paper.NewState()}, Token: token}).Handler()
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

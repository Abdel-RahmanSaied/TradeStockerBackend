package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func setup(t *testing.T, api, token, provider string) {
	t.Helper()
	t.Setenv("TS_API_URL", api)
	t.Setenv("TS_LOCAL_DEV_TOKEN", token)
	t.Setenv("TS_MARKET_PROVIDER", provider)
	old := os.Args
	os.Args = []string{"market-ingestion"}
	t.Cleanup(func() { os.Args = old })
}

func TestRealFeedsAndRemoteTargetsRefused(t *testing.T) {
	token := strings.Repeat("t", 32)
	for _, tc := range []struct{ name, api, token, provider, expected string }{
		{"alpaca", "", token, "alpaca", "not qualified"},
		{"massive", "", token, "massive", "not qualified"},
		{"alpha vantage", "", token, "alphavantage", "not qualified"},
		{"case variant", "", token, "Fixture", "not qualified"},
		{"remote api", "http://evil.invalid:8080", token, "", "loopback"},
		{"https", "https://127.0.0.1:8080", token, "", "loopback"},
		{"credentials in url", "http://user:pw@127.0.0.1:8080", token, "", "loopback"},
		{"path", "http://127.0.0.1:8080/api", token, "", "loopback"},
		{"query", "http://127.0.0.1:8080?x=1", token, "", "loopback"},
		{"wildcard host", "http://0.0.0.0:8080", token, "", "loopback"},
		{"short token", "", "short", "", "token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setup(t, tc.api, tc.token, tc.provider)
			if e := run(); e == nil || !strings.Contains(e.Error(), tc.expected) {
				t.Fatalf("got %v, want an error containing %q", e, tc.expected)
			}
		})
	}
	setup(t, "", token, "")
	os.Args = []string{"market-ingestion", "--provider=alpaca"}
	if e := run(); e == nil || !strings.Contains(e.Error(), "no arguments") {
		t.Fatalf("arguments accepted: %v", e)
	}
}

func TestFixtureLoadUsesOnlyTheLocalAPI(t *testing.T) {
	token := strings.Repeat("t", 32)
	var calls int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/api/v1/fixtures" || string(body) != "{}" || r.Header.Get("Authorization") != "Bearer "+token ||
			r.Header.Get("Content-Type") != "application/json" || !strings.HasPrefix(r.Header.Get("Idempotency-Key"), "fixture-adapter-") {
			t.Errorf("unexpected request %s %s %q", r.Method, r.URL, body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()
	for _, provider := range []string{"", "fixture"} {
		setup(t, api.URL, token, provider)
		if e := run(); e != nil {
			t.Fatalf("provider %q: %v", provider, e)
		}
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestFailedOrRedirectedLoadFails(t *testing.T) {
	token := strings.Repeat("t", 32)
	for name, handler := range map[string]http.HandlerFunc{
		"rejected": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://evil.invalid/", http.StatusTemporaryRedirect)
		},
	} {
		t.Run(name, func(t *testing.T) {
			api := httptest.NewServer(handler)
			defer api.Close()
			setup(t, api.URL, token, "")
			if e := run(); e == nil {
				t.Fatal("failed load reported success")
			}
		})
	}
}

func TestTokenFromFile(t *testing.T) {
	token := strings.Repeat("f", 32)
	path := t.TempDir() + "/token"
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()
	setup(t, api.URL, "", "")
	t.Setenv("TS_LOCAL_DEV_TOKEN_FILE", path)
	if e := run(); e != nil || got != "Bearer "+token {
		t.Fatalf("token file not used: %v, header %q", e, got)
	}
	t.Setenv("TS_LOCAL_DEV_TOKEN", token)
	if e := run(); e == nil || !strings.Contains(e.Error(), "not both") || strings.Contains(e.Error(), token) {
		t.Fatalf("both token forms: %v", e)
	}
}

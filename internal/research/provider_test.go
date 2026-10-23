package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Synthetic SEC-shaped payloads for tests. The CIK encodings match the real
// endpoints: submissions uses a zero-padded string, companyfacts a number.
const testSubmissions = `{"cik":"0000320193","name":"Test Issuer","tickers":["AAPL"],"filings":{"recent":{}}}`
const testFacts = `{"cik":320193,"entityName":"Test Issuer","facts":{}}`

type fakeTransport struct {
	requests []*http.Request
	reply    func(*http.Request) (*http.Response, error)
}

func (f *fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, r)
	return f.reply(r)
}

func respond200(body string) func(*http.Request) (*http.Response, error) {
	return func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/submissions/") {
			return reply(200, testSubmissions), nil
		}
		return reply(200, body), nil
	}
}

func reply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

func testSEC(t *testing.T, transport http.RoundTripper) *SEC {
	t.Helper()
	s := newSEC("TradeStocker Research test@example.com", transport)
	s.minInterval = 0
	return s
}

func TestSECCIKEncodings(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want uint64
		ok   bool
	}{
		{`"0000320193"`, 320193, true}, // submissions endpoint
		{`320193`, 320193, true},       // companyfacts endpoint
		{`"320193"`, 320193, true},
		{`"00003201930"`, 0, false},
		{`-320193`, 0, false},
		{`3.2e5`, 0, false},
		{`320193.0`, 0, false},
		{`null`, 0, false},
		{`"abc"`, 0, false},
		{`""`, 0, false},
		{`true`, 0, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var c secCIK
			err := json.Unmarshal([]byte(tc.raw), &c)
			if tc.ok != (err == nil) || (tc.ok && uint64(c) != tc.want) {
				t.Fatalf("got %d, %v", c, err)
			}
		})
	}
}

func TestSECFetchUsesOnlyOfficialEndpoints(t *testing.T) {
	transport := &fakeTransport{reply: respond200(testFacts)}
	sources, err := testSEC(t, transport).Fetch(context.Background(), "AAPL", "0000320193")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://data.sec.gov/submissions/CIK0000320193.json", "https://data.sec.gov/api/xbrl/companyfacts/CIK0000320193.json"}
	if len(transport.requests) != 2 || len(sources) != 2 {
		t.Fatalf("got %d requests and %d sources", len(transport.requests), len(sources))
	}
	for i, r := range transport.requests {
		if r.Method != http.MethodGet || r.URL.String() != want[i] {
			t.Fatalf("request %d: %s %s", i, r.Method, r.URL)
		}
		if r.Header.Get("User-Agent") != "TradeStocker Research test@example.com" || r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected headers %v", r.Header)
		}
	}
	for i, kind := range []string{"submissions", "companyfacts"} {
		sum := sha256.Sum256(sources[i].Body)
		if sources[i].Kind != kind || sources[i].URL != want[i] || sources[i].SHA256 != hex.EncodeToString(sum[:]) || sources[i].Size != int64(len(sources[i].Body)) || sources[i].ReceivedAt.IsZero() {
			t.Fatalf("source %d: %+v", i, sources[i])
		}
	}
}

func TestSECProviderFailuresAreClassified(t *testing.T) {
	networkDown := func(*http.Request) (*http.Response, error) { return nil, errors.New("connection reset") }
	status := func(code int) func(*http.Request) (*http.Response, error) {
		return func(*http.Request) (*http.Response, error) { return reply(code, `{}`), nil }
	}
	redirect := func(*http.Request) (*http.Response, error) {
		r := reply(301, "")
		r.Header.Set("Location", "https://example.invalid/elsewhere.json")
		return r, nil
	}
	for _, tc := range []struct {
		name, symbol, cik string
		reply             func(*http.Request) (*http.Response, error)
		code              string
		retryable         bool
	}{
		{"not found", "AAPL", "0000320193", status(404), "SEC_NOT_FOUND", false},
		{"rate limited", "AAPL", "0000320193", status(429), "SEC_ACCESS_LIMITED", true},
		{"access refused", "AAPL", "0000320193", status(403), "SEC_ACCESS_LIMITED", true},
		{"server error", "AAPL", "0000320193", status(503), "SEC_HTTP_ERROR", true},
		{"unexpected status", "AAPL", "0000320193", status(400), "SEC_HTTP_ERROR", false},
		{"redirect refused", "AAPL", "0000320193", redirect, "SEC_REDIRECT_REFUSED", false},
		{"network failure", "AAPL", "0000320193", networkDown, "SEC_REQUEST_FAILED", true},
		{"invalid json", "AAPL", "0000320193", respond200(`{"cik":`), "SEC_INVALID_JSON", false},
		{"oversized", "AAPL", "0000320193", respond200(`{"cik":320193,"pad":"` + strings.Repeat("x", 4096) + `"}`), "SEC_RESPONSE_TOO_LARGE", false},
		{"identity mismatch", "AAPL", "0000320193", respond200(`{"cik":789019,"facts":{}}`), "SEC_IDENTITY_MISMATCH", false},
		{"unexpected shape", "AAPL", "0000320193", respond200(`{"cik":"not-a-number","facts":{}}`), "SEC_UNEXPECTED_SHAPE", false},
		{"ticker not issued to cik", "MSFT", "0000320193", respond200(testFacts), "SEC_SYMBOL_CIK_MISMATCH", false},
		{"malformed cik", "AAPL", "320193", respond200(testFacts), "INVALID_ISSUER", false},
		{"zero cik", "AAPL", "0000000000", respond200(testFacts), "INVALID_ISSUER", false},
		{"lowercase symbol", "aapl", "0000320193", respond200(testFacts), "INVALID_ISSUER", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testSEC(t, &fakeTransport{reply: tc.reply})
			s.sourceLimit = 1024
			_, err := s.Fetch(context.Background(), tc.symbol, tc.cik)
			var pe *ProviderError
			if !errors.As(err, &pe) || pe.Code != tc.code || pe.Retryable != tc.retryable {
				t.Fatalf("got %#v", err)
			}
		})
	}
}

func TestSECContactRequired(t *testing.T) {
	for agent, ok := range map[string]bool{
		"TradeStocker Research abdo.example@gmail.com": true,
		"":                                    false,
		"TradeStocker Research":               false,
		"abdo.example@gmail.com":              false,
		"TradeStocker\r\nX: y a@example.com":  false,
		strings.Repeat("a", 190) + " a@b.com": false,
	} {
		s := newSEC(agent, &fakeTransport{reply: respond200(testFacts)})
		if s.Configured() != ok {
			t.Fatalf("%q configured=%v", agent, s.Configured())
		}
		if !ok {
			_, err := s.Fetch(context.Background(), "AAPL", "0000320193")
			var pe *ProviderError
			if !errors.As(err, &pe) || pe.Code != "SEC_CONTACT_NOT_CONFIGURED" {
				t.Fatalf("unconfigured fetch: %v", err)
			}
		}
	}
}

func TestSECRequestsAreSpaced(t *testing.T) {
	s := testSEC(t, &fakeTransport{reply: respond200(testFacts)})
	s.minInterval = 40 * time.Millisecond
	start := time.Now()
	if _, err := s.Fetch(context.Background(), "AAPL", "0000320193"); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("two requests took %v; want at least the minimum interval", elapsed)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Fetch(ctx, "AAPL", "0000320193"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait: %v", err)
	}
}

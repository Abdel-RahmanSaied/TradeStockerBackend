package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SourceLimit bounds each decompressed SEC response kept as an original source.
const SourceLimit = 16 << 20

// The only origin the provider contacts. URLs are built from it and a validated CIK.
const secDataOrigin = "https://data.sec.gov"

var cikPattern = regexp.MustCompile(`^[0-9]{10}$`)
var cikDigits = regexp.MustCompile(`^[0-9]{1,10}$`)
var symbolPattern = regexp.MustCompile(`^[A-Z][A-Z0-9.-]{0,15}$`)
var contactPattern = regexp.MustCompile(`^[^\r\n]{1,100} [A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
var errRedirectRefused = errors.New("redirect refused")

// ProviderError carries a stable code that is safe to show to the owner.
// Retryable errors may succeed on a later attempt; others fail the job.
type ProviderError struct {
	Code      string
	Retryable bool
}

func (e *ProviderError) Error() string { return e.Code }

func permanent(code string) error { return &ProviderError{Code: code} }
func retryable(code string) error { return &ProviderError{Code: code, Retryable: true} }

type Source struct {
	Attempt    int             `json:"attempt"`
	Kind       string          `json:"kind"`
	SHA256     string          `json:"sha256"`
	URL        string          `json:"url"`
	ReceivedAt time.Time       `json:"received_at"`
	Size       int64           `json:"size_bytes"`
	Body       json.RawMessage `json:"-"`
}
type Fetcher interface {
	Fetch(ctx context.Context, symbol, cik string) ([]Source, error)
}

// secCIK accepts both SEC encodings: a zero-padded JSON string in submissions
// ("0000320193") and a JSON number in companyfacts (320193).
type secCIK uint64

func (c *secCIK) UnmarshalJSON(b []byte) error {
	text := string(b)
	if strings.HasPrefix(text, `"`) {
		if err := json.Unmarshal(b, &text); err != nil {
			return err
		}
	}
	if !cikDigits.MatchString(text) {
		return errors.New("CIK must be 1 to 10 digits")
	}
	n, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return err
	}
	*c = secCIK(n)
	return nil
}

type SEC struct {
	UserAgent   string
	client      *http.Client
	minInterval time.Duration
	sourceLimit int64
	mu          sync.Mutex
	next        time.Time
}

func NewSEC(agent string) *SEC { return newSEC(agent, nil) }

func newSEC(agent string, transport http.RoundTripper) *SEC {
	client := &http.Client{Timeout: 25 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errRedirectRefused }}
	// SEC fair access allows 10 requests per second; one per second stays far below it.
	return &SEC{UserAgent: agent, client: client, minInterval: time.Second, sourceLimit: SourceLimit}
}

func (s *SEC) Configured() bool {
	return len(s.UserAgent) <= 200 && contactPattern.MatchString(s.UserAgent)
}

func (s *SEC) Fetch(ctx context.Context, symbol, cik string) ([]Source, error) {
	if !s.Configured() {
		return nil, permanent("SEC_CONTACT_NOT_CONFIGURED")
	}
	if !cikPattern.MatchString(cik) || !symbolPattern.MatchString(symbol) || cik == "0000000000" {
		return nil, permanent("INVALID_ISSUER")
	}
	urls := []string{secDataOrigin + "/submissions/CIK" + cik + ".json", secDataOrigin + "/api/xbrl/companyfacts/CIK" + cik + ".json"}
	sources := make([]Source, 0, 2)
	for i, u := range urls {
		source, err := s.fetch(ctx, u)
		if err != nil {
			return nil, err
		}
		source.Kind = []string{"submissions", "companyfacts"}[i]
		sources = append(sources, source)
	}
	var sub struct {
		CIK     secCIK   `json:"cik"`
		Tickers []string `json:"tickers"`
	}
	var facts struct {
		CIK secCIK `json:"cik"`
	}
	if json.Unmarshal(sources[0].Body, &sub) != nil || json.Unmarshal(sources[1].Body, &facts) != nil {
		return nil, permanent("SEC_UNEXPECTED_SHAPE")
	}
	requested, _ := strconv.ParseUint(cik, 10, 64)
	if uint64(sub.CIK) != requested || uint64(facts.CIK) != requested {
		return nil, permanent("SEC_IDENTITY_MISMATCH")
	}
	for _, t := range sub.Tickers {
		if t == symbol {
			return sources, nil
		}
	}
	return nil, permanent("SEC_SYMBOL_CIK_MISMATCH")
}

func (s *SEC) fetch(ctx context.Context, u string) (Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if wait := time.Until(s.next); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return Source{}, ctx.Err()
		}
	} else if ctx.Err() != nil {
		return Source{}, ctx.Err()
	}
	s.next = time.Now().Add(s.minInterval)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Source{}, permanent("SEC_REQUEST_FAILED")
	}
	req.Header.Set("User-Agent", s.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Source{}, ctx.Err()
		}
		if errors.Is(err, errRedirectRefused) {
			return Source{}, permanent("SEC_REDIRECT_REFUSED")
		}
		return Source{}, retryable("SEC_REQUEST_FAILED")
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusNotFound:
		return Source{}, permanent("SEC_NOT_FOUND")
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden:
		// SEC does not document its status codes; we treat 403/429 as an access limit.
		return Source{}, retryable("SEC_ACCESS_LIMITED")
	case resp.StatusCode >= 500:
		return Source{}, retryable("SEC_HTTP_ERROR")
	default:
		return Source{}, permanent("SEC_HTTP_ERROR")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, s.sourceLimit+1))
	if err != nil {
		if ctx.Err() != nil {
			return Source{}, ctx.Err()
		}
		return Source{}, retryable("SEC_BODY_FAILED")
	}
	if int64(len(body)) > s.sourceLimit {
		return Source{}, permanent("SEC_RESPONSE_TOO_LARGE")
	}
	if !json.Valid(body) {
		return Source{}, permanent("SEC_INVALID_JSON")
	}
	hash := sha256.Sum256(body)
	return Source{URL: u, Body: body, Size: int64(len(body)), SHA256: hex.EncodeToString(hash[:]), ReceivedAt: time.Now().UTC()}, nil
}

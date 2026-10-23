// market-ingestion is a fixture-only adapter. It cannot request a real feed:
// no provider is qualified (S00-T03), so every real feed stays disabled until
// its adapter task (S03-T02/T03) and data decision (D-01/D-02) are complete.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"tradestocker/internal/paper"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// qualifiedProviders lists the sources this binary may use. Only the synthetic
// fixture source exists; adding a provider needs its qualification evidence.
var qualifiedProviders = map[string]bool{"fixture": true}

func run() error {
	if len(os.Args) > 1 {
		return errors.New("usage: market-ingestion (loads synthetic paper fixtures; takes no arguments)")
	}
	if provider := os.Getenv("TS_MARKET_PROVIDER"); provider != "" && !qualifiedProviders[provider] {
		return fmt.Errorf("market data provider %q is not qualified; real feeds stay disabled", provider)
	}
	base := os.Getenv("TS_API_URL")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	u, e := url.Parse(base)
	if e != nil {
		return e
	}
	if u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return errors.New("fixture adapter permits only a local loopback API origin")
	}
	token := os.Getenv("TS_LOCAL_DEV_TOKEN")
	if len(token) < 24 {
		return errors.New("local API token required")
	}
	req, e := http.NewRequest("POST", base+"/api/v1/fixtures", bytes.NewBufferString("{}"))
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "fixture-adapter-"+paper.ID())
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirects disabled") }}
	response, e := client.Do(req)
	if e != nil {
		return e
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fixture ingestion rejected: HTTP %d", response.StatusCode)
	}
	fmt.Println("Synthetic paper fixtures loaded. No live provider used.")
	return nil
}

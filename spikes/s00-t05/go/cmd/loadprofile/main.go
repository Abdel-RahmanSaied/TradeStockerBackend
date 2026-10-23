// Command loadprofile measures the local paper API on a disposable database:
// latency and throughput for reads and writes, and how they change as the paper
// aggregate grows. Synthetic paper data only; never point it at the normal API.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type phase struct {
	Name          string  `json:"name"`
	Seconds       float64 `json:"seconds"`
	Concurrency   int     `json:"concurrency"`
	Requests      int     `json:"requests"`
	Errors        int     `json:"errors"`
	PerSecond     float64 `json:"per_second"`
	P50Millis     float64 `json:"p50_ms"`
	P95Millis     float64 `json:"p95_ms"`
	P99Millis     float64 `json:"p99_ms"`
	ResponseBytes int     `json:"last_response_bytes"`
}

type profile struct {
	API             string  `json:"api"`
	Phases          []phase `json:"phases"`
	Recommendations int     `json:"recommendations_after"`
	AuditEvents     int     `json:"audit_events_after"`
	Note            string  `json:"note"`
}

var (
	base   = env("SPIKE_API", "http://127.0.0.1:18180/api/v1")
	token  = os.Getenv("TS_LOCAL_DEV_TOKEN")
	client = &http.Client{Timeout: 30 * time.Second}
	keySeq atomic.Int64
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "loadprofile:", err)
		os.Exit(1)
	}
}

func run() error {
	if base == "http://127.0.0.1:8080/api/v1" {
		return errors.New("refusing to load the normal API on 8080")
	}
	seconds, _ := strconv.Atoi(env("SPIKE_LOAD_SECONDS", "5"))
	workers, _ := strconv.Atoi(env("SPIKE_LOAD_CONCURRENCY", "8"))
	var account struct {
		ID string `json:"id"`
	}
	if err := call("POST", "/paper/accounts", map[string]any{"currency": "USD", "amount_minor": 100_000_000}, &account); err != nil {
		return fmt.Errorf("create paper account: %w", err)
	}
	if err := call("POST", "/fixtures", map[string]any{}, nil); err != nil {
		return fmt.Errorf("load fixtures: %w", err)
	}
	read := func() (int, error) { return request("GET", "/dashboard", nil) }
	write := func() (int, error) {
		return request("POST", "/paper/recommendations", map[string]any{"account_id": account.ID, "symbol": "AAPL", "lane": "US_SWING", "quantity": 1, "entry_minor": 20000, "stop_minor": 19500})
	}
	p := profile{API: base, Note: "Every request, including reads, locks and rewrites the single paper JSON aggregate (ADR-017)."}
	d := time.Duration(seconds) * time.Second
	p.Phases = append(p.Phases, measure("dashboard_read_small_state", d, workers, read))
	p.Phases = append(p.Phases, measure("evaluate_write", d, workers, write))
	p.Phases = append(p.Phases, measure("dashboard_read_after_writes", d, workers, read))
	var dash struct {
		Recommendations []json.RawMessage `json:"recommendations"`
		Audit           []json.RawMessage `json:"audit"`
	}
	if err := call("GET", "/dashboard", nil, &dash); err != nil {
		return err
	}
	p.Recommendations, p.AuditEvents = len(dash.Recommendations), len(dash.Audit)
	out, _ := json.MarshalIndent(p, "", " ")
	fmt.Println(string(out))
	return nil
}

func measure(name string, d time.Duration, workers int, fn func() (int, error)) phase {
	var mu sync.Mutex
	var latencies []time.Duration
	var errs, last int
	stop := time.Now().Add(d)
	start := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(stop) {
				t := time.Now()
				size, err := fn()
				elapsed := time.Since(t)
				mu.Lock()
				latencies = append(latencies, elapsed)
				if err != nil {
					errs++
				} else {
					last = size
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	total := time.Since(start)
	sort.Slice(latencies, func(a, b int) bool { return latencies[a] < latencies[b] })
	pick := func(q float64) float64 {
		if len(latencies) == 0 {
			return 0
		}
		return float64(latencies[int(q*float64(len(latencies)-1))].Microseconds()) / 1000
	}
	return phase{Name: name, Seconds: total.Seconds(), Concurrency: workers, Requests: len(latencies), Errors: errs,
		PerSecond: float64(len(latencies)) / total.Seconds(), P50Millis: pick(0.50), P95Millis: pick(0.95), P99Millis: pick(0.99), ResponseBytes: last}
}

func request(method, path string, body any) (int, error) {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		// Every write is a new operation, so each gets its own key.
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", fmt.Sprintf("spike-load-%d-%d", time.Now().UnixNano(), keySeq.Add(1)))
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, data)
	}
	return len(data), nil
}

func call(method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", fmt.Sprintf("spike-setup-%d", keySeq.Add(1)))
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, data)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

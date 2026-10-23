// Command paperbench measures paper API reads at a fixed synthetic history size
// and counts the SQL writes they cause. Run it only through
// scripts/paper-read-bench.sh, against a disposable database and API.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type latency struct {
	Requests  int     `json:"requests"`
	Errors    int     `json:"errors"`
	PerSecond float64 `json:"per_second"`
	P50       float64 `json:"p50_ms"`
	P95       float64 `json:"p95_ms"`
	P99       float64 `json:"p99_ms"`
}

type result struct {
	Label                string  `json:"label"`
	HistoryEvaluations   int     `json:"history_evaluations"`
	Recommendations      int     `json:"recommendations"`
	AuditEvents          int     `json:"audit_events"`
	DocumentBytes        int64   `json:"document_bytes"`
	DashboardBytes       int     `json:"dashboard_response_bytes"`
	ReadSeconds          float64 `json:"read_seconds"`
	Dashboard            latency `json:"dashboard_reads"`
	Health               latency `json:"health_checks"`
	UpdatesDuringReads   int64   `json:"sql_updates_during_reads"`
	RowVersionChanged    bool    `json:"row_version_changed"`
	UpdatedAtChanged     bool    `json:"updated_at_changed"`
	TableBytesBeforeRead int64   `json:"table_bytes_before_reads"`
	TableBytesAfterRead  int64   `json:"table_bytes_after_reads"`
}

var (
	api    = os.Getenv("BENCH_API")
	token  = os.Getenv("TS_LOCAL_DEV_TOKEN")
	client = &http.Client{Timeout: 30 * time.Second}
)

func main() {
	r, err := run(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "paperbench:", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(r, "", " ")
	fmt.Println(string(out))
}

func run(ctx context.Context) (result, error) {
	r := result{Label: os.Getenv("BENCH_LABEL")}
	if api == "" || strings.Contains(api, ":8080") {
		return r, errors.New("BENCH_API must name a disposable API, never the normal one on 8080")
	}
	r.HistoryEvaluations = envInt("BENCH_HISTORY", 400)
	seconds := envInt("BENCH_SECONDS", 10)
	workers := envInt("BENCH_CONCURRENCY", 8)
	pool, err := pgxpool.New(ctx, os.Getenv("BENCH_DB_URL"))
	if err != nil {
		return r, err
	}
	defer pool.Close()

	// The same synthetic history for every binary: one account, fixtures, N evaluations.
	var account struct {
		ID string `json:"id"`
	}
	if err = post("/paper/accounts", "bench-account", map[string]any{"currency": "USD", "amount_minor": 100_000_000}, &account); err != nil {
		return r, err
	}
	if err = post("/fixtures", "bench-fixtures", map[string]any{}, nil); err != nil {
		return r, err
	}
	for i := 0; i < r.HistoryEvaluations; i++ {
		body := map[string]any{"account_id": account.ID, "symbol": "AAPL", "lane": "US_SWING", "quantity": 1, "entry_minor": 20000, "stop_minor": 19500}
		if err = post("/paper/recommendations", fmt.Sprintf("bench-eval-%05d", i), body, nil); err != nil {
			return r, err
		}
	}

	// Count every UPDATE of the paper row exactly while the reads run.
	if _, err = pool.Exec(ctx, `CREATE TABLE bench_paper_writes(id int PRIMARY KEY, updates bigint NOT NULL);
INSERT INTO bench_paper_writes VALUES (1, 0);
CREATE FUNCTION bench_count_paper_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN UPDATE bench_paper_writes SET updates = updates + 1 WHERE id = 1; RETURN NEW; END $$;
CREATE TRIGGER bench_count_paper_write AFTER UPDATE ON paper_workspace FOR EACH ROW EXECUTE FUNCTION bench_count_paper_write();`); err != nil {
		return r, err
	}
	var xmin1, updated1 string
	if err = pool.QueryRow(ctx, `SELECT xmin::text, updated_at::text, octet_length(document::text), pg_total_relation_size('paper_workspace') FROM paper_workspace WHERE id=1`).Scan(&xmin1, &updated1, &r.DocumentBytes, &r.TableBytesBeforeRead); err != nil {
		return r, err
	}

	var wg sync.WaitGroup
	stop := time.Now().Add(time.Duration(seconds) * time.Second)
	start := time.Now()
	dash := make([][]time.Duration, workers)
	dashErrs := make([]int, workers)
	sizes := make([]int, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for time.Now().Before(stop) {
				t := time.Now()
				n, err := get("/dashboard", true)
				dash[w] = append(dash[w], time.Since(t))
				if err != nil {
					dashErrs[w]++
				} else {
					sizes[w] = n
				}
			}
		}(w)
	}
	var health []time.Duration
	healthErrs := 0
	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Now().Before(stop) {
			t := time.Now()
			if _, err := get("/health", false); err != nil {
				healthErrs++
			}
			health = append(health, time.Since(t))
		}
	}()
	wg.Wait()
	r.ReadSeconds = time.Since(start).Seconds()
	var all []time.Duration
	errs := 0
	for w := 0; w < workers; w++ {
		all = append(all, dash[w]...)
		errs += dashErrs[w]
		if sizes[w] > r.DashboardBytes {
			r.DashboardBytes = sizes[w]
		}
	}
	r.Dashboard = summarize(all, errs, r.ReadSeconds)
	r.Health = summarize(health, healthErrs, r.ReadSeconds)

	var xmin2, updated2 string
	if err = pool.QueryRow(ctx, `SELECT xmin::text, updated_at::text, pg_total_relation_size('paper_workspace') FROM paper_workspace WHERE id=1`).Scan(&xmin2, &updated2, &r.TableBytesAfterRead); err != nil {
		return r, err
	}
	if err = pool.QueryRow(ctx, `SELECT updates FROM bench_paper_writes WHERE id=1`).Scan(&r.UpdatesDuringReads); err != nil {
		return r, err
	}
	r.RowVersionChanged, r.UpdatedAtChanged = xmin1 != xmin2, updated1 != updated2
	var dashboard struct {
		Recommendations []json.RawMessage `json:"recommendations"`
		Audit           []json.RawMessage `json:"audit"`
	}
	if err = getJSON("/dashboard", &dashboard); err != nil {
		return r, err
	}
	r.Recommendations, r.AuditEvents = len(dashboard.Recommendations), len(dashboard.Audit)
	return r, nil
}

func summarize(d []time.Duration, errs int, seconds float64) latency {
	sort.Slice(d, func(a, b int) bool { return d[a] < d[b] })
	pick := func(q float64) float64 {
		if len(d) == 0 {
			return 0
		}
		return float64(d[int(q*float64(len(d)-1))].Microseconds()) / 1000
	}
	return latency{Requests: len(d), Errors: errs, PerSecond: float64(len(d)) / seconds, P50: pick(0.5), P95: pick(0.95), P99: pick(0.99)}
}

func get(path string, auth bool) (int, error) {
	req, err := http.NewRequest(http.MethodGet, api+path, nil)
	if err != nil {
		return 0, err
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err == nil && resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("GET %s: %d", path, resp.StatusCode)
	}
	return len(body), err
}

func getJSON(path string, out any) error {
	req, _ := http.NewRequest(http.MethodGet, api+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func post(path, key string, body, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, api+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s: %d %s", path, resp.StatusCode, data)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}

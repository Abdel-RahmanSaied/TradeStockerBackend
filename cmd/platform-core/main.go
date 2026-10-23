package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"tradestocker/internal/httpapi"
	"tradestocker/internal/natsbus"
	"tradestocker/internal/research"
	"tradestocker/internal/store"
	"tradestocker/internal/telemetry"
)

var loopbackOrigin = regexp.MustCompile(`^http://(127\.0\.0\.1|localhost):[1-9][0-9]{1,4}$`)

func main() {
	if err := run(); err != nil {
		slog.Error("platform-core stopped", "error", err)
		os.Exit(1)
	}
}

// secret returns the variable name, or the contents of the file named by
// name_FILE (for example a Compose secret mount). Setting both is refused, and
// errors never include the secret itself.
func secret(name string) (string, error) {
	path := os.Getenv(name + "_FILE")
	if path == "" {
		return os.Getenv(name), nil
	}
	if os.Getenv(name) != "" {
		return "", fmt.Errorf("set %s or %s_FILE, not both", name, name)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%s_FILE cannot be read: %w", name, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return "", fmt.Errorf("%s_FILE cannot be read: %w", name, err)
	}
	if len(b) > 4096 {
		return "", fmt.Errorf("%s_FILE is larger than 4096 bytes", name)
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

// config is everything serve needs, checked by run.
type config struct {
	token, databaseURL, bind string
	origins                  []string
	python, pythonVersion    string
	fetcher                  research.Fetcher // the SEC provider; tests pass a fake
	researchConfigured       bool
	natsURL                  string // empty: no NATS, workers poll only
	workers                  int
}

// newLogger is the process logger: JSON lines (stderr in production), with
// the given secrets removed wherever they appear.
func newLogger(w io.Writer, secrets ...string) *slog.Logger {
	r := &telemetry.Redactor{}
	r.Add(secrets...)
	return telemetry.NewLogger(w, r, slog.LevelInfo)
}

// databasePassword returns the password inside a PostgreSQL URL or DSN, if any.
func databasePassword(url string) string {
	if c, err := pgconn.ParseConfig(url); err == nil {
		return c.Password
	}
	return ""
}

func run() error {
	token, err := secret("TS_LOCAL_DEV_TOKEN")
	if err != nil {
		return err
	}
	if len(token) < 24 {
		return errors.New("TS_LOCAL_DEV_TOKEN must have at least 24 characters; generate locally")
	}
	if mode := os.Getenv("TS_MODE"); mode != "" && mode != "PAPER" {
		return errors.New("only PAPER mode is implemented; live and actual are disabled")
	}
	url, err := secret("DATABASE_URL")
	if err != nil {
		return err
	}
	if url == "" {
		return errors.New("DATABASE_URL required")
	}
	agent := os.Getenv("TS_SEC_USER_AGENT")
	// From here on every log line is JSON with the secrets removed.
	slog.SetDefault(newLogger(os.Stderr, token, url, databasePassword(url), agent))
	cfg := config{token: token, databaseURL: url, bind: os.Getenv("TS_BIND")}
	if cfg.bind == "" {
		cfg.bind = "127.0.0.1:8080"
	}
	host, _, err := net.SplitHostPort(cfg.bind)
	if err != nil {
		return err
	}
	if host != "127.0.0.1" && host != "::1" && !(host == "0.0.0.0" && os.Getenv("TS_CONTAINER_LOCAL") == "1") {
		return errors.New("bind must be loopback; container mode requires explicit TS_CONTAINER_LOCAL=1")
	}
	// An isolated dashboard (for example a test on another port) may name its own
	// origin, but only a loopback http origin with an explicit port.
	if origin := os.Getenv("TS_DASHBOARD_ORIGIN"); origin != "" {
		if !loopbackOrigin.MatchString(origin) {
			return errors.New("TS_DASHBOARD_ORIGIN must be http://127.0.0.1:<port> or http://localhost:<port>")
		}
		cfg.origins = []string{origin}
	}
	cfg.workers = 1
	if text := os.Getenv("TS_RESEARCH_WORKERS"); text != "" {
		if cfg.workers, err = strconv.Atoi(text); err != nil || cfg.workers < 1 || cfg.workers > 4 {
			return errors.New("TS_RESEARCH_WORKERS must be a whole number from 1 to 4")
		}
	}
	// NATS is optional: without it, workers find new jobs by polling PostgreSQL.
	if cfg.natsURL = os.Getenv("TS_NATS_URL"); cfg.natsURL != "" {
		if err = natsbus.CheckURL(cfg.natsURL); err != nil {
			return err
		}
	}
	// The research parser runs only on the exact CPython it is tested on.
	cfg.python = os.Getenv("TS_PYTHON")
	if cfg.python == "" {
		cfg.python = "python3"
	}
	check, cancelCheck := context.WithTimeout(context.Background(), 10*time.Second)
	cfg.pythonVersion, err = research.CheckPython(check, cfg.python)
	cancelCheck()
	if err != nil {
		return err
	}
	provider := research.NewSEC(agent)
	cfg.fetcher, cfg.researchConfigured = provider, provider.Configured()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, cfg, nil)
}

// serve runs platform-core until ctx ends, then stops in order. listening, if
// set, is told the address once the API accepts connections.
func serve(ctx context.Context, cfg config, listening func(addr string)) error {
	start, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	db, err := store.Open(start, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = db.Migrate(start); err != nil {
		return err
	}
	researchService := &research.Service{Pool: db.Pool, Provider: cfg.fetcher, Configured: cfg.researchConfigured, Python: cfg.python, Workers: cfg.workers}
	natsState := func(context.Context) string { return httpapi.DependencyNotConfigured }
	if cfg.natsURL != "" {
		bus, err := natsbus.Connect(cfg.natsURL, "platform-core")
		if err != nil {
			return err
		}
		defer bus.Close()
		if err = bus.Subscribe(researchService.Wake); err != nil {
			return err
		}
		researchService.Notifier = bus
		natsState = func(context.Context) string {
			if bus.Ready() {
				return httpapi.DependencyOK
			}
			return httpapi.DependencyUnavailable
		}
	}
	var draining atomic.Bool
	api := &httpapi.API{Store: db, Token: cfg.token, Research: researchService.Handler(), Origins: cfg.origins, Draining: &draining,
		Dependencies: []httpapi.Dependency{
			{Name: "postgres", Required: true, Check: func(ctx context.Context) string {
				if db.Pool.Ping(ctx) != nil {
					return httpapi.DependencyUnavailable
				}
				return httpapi.DependencyOK
			}},
			{Name: "nats", Check: natsState},
		}}
	listener, err := net.Listen("tcp", cfg.bind)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}

	// The worker gets its own context, so it is stopped at a chosen point below.
	work, stopWorker := context.WithCancel(context.WithoutCancel(ctx))
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); researchService.Run(work) }()
	defer func() { stopWorker(); <-workerDone }()

	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	slog.InfoContext(ctx, "local paper API ready", "address", listener.Addr().String(), "mode", "PAPER", "live_advisory_enabled", false,
		"research_python", cfg.pythonVersion, "research_configured", cfg.researchConfigured, "research_workers", researchService.Workers, "nats_configured", cfg.natsURL != "")
	if listening != nil {
		listening(listener.Addr().String())
	}
	select {
	case err = <-result:
		return err
	case <-ctx.Done():
	}
	// Graceful stop, in order: readiness reports draining; HTTP stops taking
	// requests and finishes those in flight; the research workers settle or
	// release their attempts; then (deferred) NATS drains and the database
	// closes last.
	slog.Info("platform-core stopping")
	draining.Store(true)
	stopWorker()
	end, cancelEnd := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelEnd()
	err = server.Shutdown(end)
	<-workerDone
	slog.Info("platform-core stopped", "http_shutdown_error", err)
	return err
}

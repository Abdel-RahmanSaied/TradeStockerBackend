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
	"strings"
	"syscall"
	"time"

	"tradestocker/internal/httpapi"
	"tradestocker/internal/research"
	"tradestocker/internal/store"
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
	bind := os.Getenv("TS_BIND")
	if bind == "" {
		bind = "127.0.0.1:8080"
	}
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		return err
	}
	if host != "127.0.0.1" && host != "::1" && !(host == "0.0.0.0" && os.Getenv("TS_CONTAINER_LOCAL") == "1") {
		return errors.New("bind must be loopback; container mode requires explicit TS_CONTAINER_LOCAL=1")
	}
	// An isolated dashboard (for example a test on another port) may name its own
	// origin, but only a loopback http origin with an explicit port.
	var origins []string
	if origin := os.Getenv("TS_DASHBOARD_ORIGIN"); origin != "" {
		if !loopbackOrigin.MatchString(origin) {
			return errors.New("TS_DASHBOARD_ORIGIN must be http://127.0.0.1:<port> or http://localhost:<port>")
		}
		origins = []string{origin}
	}
	// The research parser runs only on the exact CPython it is tested on.
	python := os.Getenv("TS_PYTHON")
	if python == "" {
		python = "python3"
	}
	check, cancelCheck := context.WithTimeout(context.Background(), 10*time.Second)
	pythonVersion, err := research.CheckPython(check, python)
	cancelCheck()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	db, err := store.Open(start, url)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = db.Migrate(start); err != nil {
		return err
	}
	provider := research.NewSEC(os.Getenv("TS_SEC_USER_AGENT"))
	researchService := &research.Service{Pool: db.Pool, Provider: provider, Configured: provider.Configured(), Python: python}
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); researchService.Run(ctx) }()
	defer func() { stop(); <-workerDone }()
	server := &http.Server{Addr: bind, Handler: (&httpapi.API{Store: db, Token: token, Research: researchService.Handler(), Origins: origins}).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	result := make(chan error, 1)
	go func() {
		slog.Info("local paper API ready", "address", bind, "mode", "PAPER", "live_advisory_enabled", false, "research_python", pythonVersion)
		result <- server.ListenAndServe()
	}()
	select {
	case err = <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		end, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		return server.Shutdown(end)
	}
}

package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tradestocker/internal/httpapi"
	"tradestocker/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("platform-core stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	token := os.Getenv("TS_LOCAL_DEV_TOKEN")
	if len(token) < 24 {
		return errors.New("TS_LOCAL_DEV_TOKEN must have at least 24 characters; generate locally")
	}
	if mode := os.Getenv("TS_MODE"); mode != "" && mode != "PAPER" {
		return errors.New("only PAPER mode is implemented; live and actual are disabled")
	}
	url := os.Getenv("DATABASE_URL")
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
	server := &http.Server{Addr: bind, Handler: (&httpapi.API{Store: db, Token: token}).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	result := make(chan error, 1)
	go func() {
		slog.Info("local paper API ready", "address", bind, "mode", "PAPER", "live_advisory_enabled", false)
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

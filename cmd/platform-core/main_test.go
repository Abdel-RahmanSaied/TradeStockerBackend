package main

import (
	"os"
	"strings"
	"testing"
)

func TestUnsafeStartupRefused(t *testing.T) {
	for _, tc := range []struct{ name, token, mode, url, bind, expected string }{
		{"short token", "short", "PAPER", "unused", "127.0.0.1:8080", "TS_LOCAL_DEV_TOKEN"},
		{"actual mode", strings.Repeat("t", 32), "ACTUAL", "unused", "127.0.0.1:8080", "only PAPER"},
		{"missing database", strings.Repeat("t", 32), "PAPER", "", "127.0.0.1:8080", "DATABASE_URL"},
		{"public bind", strings.Repeat("t", 32), "PAPER", "unused", "0.0.0.0:8080", "loopback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TS_LOCAL_DEV_TOKEN", tc.token)
			t.Setenv("TS_MODE", tc.mode)
			t.Setenv("DATABASE_URL", tc.url)
			t.Setenv("TS_BIND", tc.bind)
			t.Setenv("TS_CONTAINER_LOCAL", "")
			t.Setenv("TS_DASHBOARD_ORIGIN", "")
			if e := run(); e == nil || !strings.Contains(e.Error(), tc.expected) {
				t.Fatalf("unsafe startup: %v", e)
			}
		})
	}
}

func TestDashboardOriginMustBeLoopback(t *testing.T) {
	for _, origin := range []string{"https://evil.invalid", "http://evil.invalid:5173", "https://127.0.0.1:5173", "http://127.0.0.1",
		"http://0.0.0.0:5173", "http://127.0.0.1:5173/", "http://127.0.0.1:05173", "http://user@127.0.0.1:5173", "*"} {
		t.Run(origin, func(t *testing.T) {
			t.Setenv("TS_LOCAL_DEV_TOKEN", strings.Repeat("t", 32))
			t.Setenv("TS_MODE", "PAPER")
			t.Setenv("DATABASE_URL", "unused")
			t.Setenv("TS_BIND", "127.0.0.1:8080")
			t.Setenv("TS_DASHBOARD_ORIGIN", origin)
			if e := run(); e == nil || !strings.Contains(e.Error(), "TS_DASHBOARD_ORIGIN") {
				t.Fatalf("non-loopback dashboard origin accepted: %v", e)
			}
		})
	}
}

// The startup check runs before any database connection.
func TestUnsupportedResearchPythonRefused(t *testing.T) {
	fake := t.TempDir() + "/python"
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'CPython 3.9.6'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, python := range []string{fake, t.TempDir() + "/missing"} {
		t.Setenv("TS_LOCAL_DEV_TOKEN", strings.Repeat("t", 32))
		t.Setenv("TS_MODE", "PAPER")
		t.Setenv("DATABASE_URL", "postgres://127.0.0.1:1/unused")
		t.Setenv("TS_BIND", "127.0.0.1:8080")
		t.Setenv("TS_DASHBOARD_ORIGIN", "")
		t.Setenv("TS_PYTHON", python)
		if e := run(); e == nil || !strings.Contains(e.Error(), "research Python") {
			t.Fatalf("unsupported research Python %s accepted: %v", python, e)
		}
	}
}

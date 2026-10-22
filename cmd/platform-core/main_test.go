package main

import (
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
			if e := run(); e == nil || !strings.Contains(e.Error(), tc.expected) {
				t.Fatalf("unsafe startup: %v", e)
			}
		})
	}
}

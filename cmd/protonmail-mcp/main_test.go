package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLogLevelFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want slog.Level
	}{
		{"debug-set", []string{"PROTONMAIL_MCP_LOG_LEVEL=debug"}, slog.LevelDebug},
		{"info-default", nil, slog.LevelInfo},
		{"other-value-defaults", []string{"PROTONMAIL_MCP_LOG_LEVEL=trace"}, slog.LevelInfo},
		{"empty-value", []string{"PROTONMAIL_MCP_LOG_LEVEL="}, slog.LevelInfo},
		{"surrounding-vars", []string{"PATH=/usr/bin", "PROTONMAIL_MCP_LOG_LEVEL=debug", "HOME=/h"}, slog.LevelDebug},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := logLevelFromEnv(tc.env); got != tc.want {
				t.Fatalf("logLevelFromEnv = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnvLookup(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		key  string
		want string
	}{
		{"found", []string{"FOO=bar"}, "FOO", "bar"},
		{"missing", []string{"FOO=bar"}, "BAZ", ""},
		{"empty-env", nil, "FOO", ""},
		{"empty-value", []string{"FOO="}, "FOO", ""},
		{"prefix-collision", []string{"FOOBAR=x", "FOO=y"}, "FOO", "y"},
		{"value-with-equals", []string{"FOO=a=b=c"}, "FOO", "a=b=c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := envLookup(tc.env, tc.key); got != tc.want {
				t.Fatalf("envLookup = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"oops"}, nil, strings.NewReader(""), stdout, stderr, nil)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown subcommand oops") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

// TestRunNoArgsServerError covers the no-arg branch's error mapping
// (main.go:78-81): a server startup failure surfaces as exit 1 with a
// "server:" stderr prefix. The serverRun seam returns synchronously because
// the real StdioTransport only returns nil on context cancel.
func TestRunNoArgsServerError(t *testing.T) {
	orig := serverRun
	t.Cleanup(func() { serverRun = orig })
	serverRun = func(context.Context, string, http.RoundTripper) error {
		return errors.New("startup boom")
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(), nil, nil, strings.NewReader(""), stdout, stderr, nil)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "server: startup boom") {
		t.Fatalf("stderr = %q, want 'server: startup boom'", stderr.String())
	}
}

// TestRunNoArgsCleanExit covers the no-arg success path (return 0): a server
// that returns nil (host disconnected cleanly) maps to exit 0. The real
// StdioTransport never returns nil under a canceled context, so this path is
// only reachable deterministically through the serverRun seam.
func TestRunNoArgsCleanExit(t *testing.T) {
	orig := serverRun
	t.Cleanup(func() { serverRun = orig })
	serverRun = func(context.Context, string, http.RoundTripper) error { return nil }
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(), nil, nil, strings.NewReader(""), stdout, stderr, nil)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
}

// TestRunNoArgsStartsServer covers the no-arg branch against the real
// server.RunWithOptions (distinct from the serverRun-seam tests, which stub
// it out): a pre-canceled context makes the real StdioTransport return an
// error, which the branch maps to exit 1 with a "server:" stderr prefix.
func TestRunNoArgsStartsServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, nil, []string{"PROTONMAIL_MCP_API_URL=https://example.test/api"},
			strings.NewReader(""), stdout, stderr, nil)
	}()
	select {
	case code := <-done:
		if code != 1 {
			t.Fatalf("exit = %d, want 1; stderr=%s", code, stderr.String())
		}
		if !strings.Contains(stderr.String(), "server: ") {
			t.Fatalf("stderr = %q, want 'server: ' prefix", stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return within 5s")
	}
}

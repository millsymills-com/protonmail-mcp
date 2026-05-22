package main

import (
	"bytes"
	"context"
	"log/slog"
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

// TestRunNoArgsStartsServer covers the no-arg branch that delegates to
// server.RunWithOptions. Context cancel returns from server.Run; an error
// from the server bubbles up as exit=1.
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
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return within 5s")
	}
}

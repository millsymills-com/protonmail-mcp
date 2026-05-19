package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"log/slog"

	mcplog "github.com/millsmillsymills/protonmail-mcp/internal/log"
	"github.com/millsmillsymills/protonmail-mcp/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	// AfterFunc fires when ctx is canceled (SIGINT/SIGTERM). The 50ms
	// grace window lets run() return cleanly; if it hangs past that, we
	// force-exit with the SIGINT convention (128+2). Capture the stop
	// handle and cancel it on the happy path so the callback can't fire
	// during a normal-exit ctx cancellation triggered by future code.
	stopAfterFunc := context.AfterFunc(ctx, func() {
		time.Sleep(50 * time.Millisecond)
		os.Exit(130)
	})
	code := run(ctx, os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr, nil)
	stopAfterFunc()
	stop()
	os.Exit(code)
}

// run is the testable entrypoint. transport is normally nil; tests pass a
// cassette-backed RoundTripper so subcommands hit the cassette instead of
// the real Proton API. env follows os.Environ() shape (KEY=value entries).
func run(
	ctx context.Context,
	args []string,
	env []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	transport http.RoundTripper,
) int {
	logger := mcplog.New(logLevelFromEnv(env), stderr)
	slog.SetDefault(logger)

	apiURL := envLookup(env, "PROTONMAIL_MCP_API_URL")

	if len(args) > 0 {
		switch args[0] {
		case "login":
			if err := runLogin(ctx, apiURL, transport, stdin, stdout, stderr); err != nil {
				_, _ = stderr.Write([]byte("login: " + err.Error() + "\n"))
				return 1
			}
			return 0
		case "logout":
			if err := runLogout(ctx, apiURL, transport, stderr); err != nil {
				_, _ = stderr.Write([]byte("logout: " + err.Error() + "\n"))
				return 1
			}
			return 0
		case "status":
			if err := runStatus(ctx, apiURL, transport, stdout); err != nil {
				_, _ = stderr.Write([]byte("status: " + err.Error() + "\n"))
				return 1
			}
			return 0
		default:
			_, _ = stderr.Write([]byte("unknown subcommand " + args[0] + "\n"))
			return 2
		}
	}

	if err := server.RunWithOptions(ctx, apiURL, transport); err != nil {
		_, _ = stderr.Write([]byte("server: " + err.Error() + "\n"))
		return 1
	}
	return 0
}

func envLookup(env []string, key string) string {
	prefix := key + "="
	for _, kv := range env {
		if len(kv) > len(prefix) && kv[:len(prefix)] == prefix {
			return kv[len(prefix):]
		}
	}
	return ""
}

func logLevelFromEnv(env []string) slog.Level {
	if envLookup(env, "PROTONMAIL_MCP_LOG_LEVEL") == "debug" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

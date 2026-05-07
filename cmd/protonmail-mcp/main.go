package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	mcplog "github.com/millsmillsymills/protonmail-mcp/internal/log"
	"github.com/millsmillsymills/protonmail-mcp/internal/server"
)

func main() {
	os.Exit(run())
}

func run() int {
	level := slog.LevelInfo
	if v := os.Getenv("PROTONMAIL_MCP_LOG_LEVEL"); v == "debug" {
		level = slog.LevelDebug
	}
	logger := mcplog.New(level, os.Stderr)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// errgroup intentionally not used here (per GO-013): this is a single
	// fire-and-forget signal-watcher whose only job is to call os.Exit on
	// SIGINT/SIGTERM when the foreground subcommand is blocked in a
	// syscall (term.ReadPassword). It is not fan-out work that needs error
	// aggregation. For the long-running MCP server path the goroutine still
	// fires on shutdown but server.Run returns before the timer elapses,
	// so this doesn't affect normal graceful exit.
	go func() {
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)
		os.Exit(130)
	}()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "login":
			if err := runLogin(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "login:", err)
				return 1
			}
			return 0
		case "logout":
			if err := runLogout(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "logout:", err)
				return 1
			}
			return 0
		case "status":
			if err := runStatus(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "status:", err)
				return 1
			}
			return 0
		default:
			fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
			return 2
		}
	}

	if err := server.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		return 1
	}
	return 0
}

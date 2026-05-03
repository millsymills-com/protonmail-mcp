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
	level := slog.LevelInfo
	if v := os.Getenv("PROTONMAIL_MCP_LOG_LEVEL"); v == "debug" {
		level = slog.LevelDebug
	}
	logger := mcplog.New(level, os.Stderr)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Make Ctrl-C actually terminate the interactive subcommands even when
	// blocked in a syscall (term.ReadPassword). For the long-running MCP
	// server path, the goroutine still fires on shutdown but server.Run
	// returns before the timer elapses, so this doesn't affect normal
	// graceful exit.
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
				os.Exit(1)
			}
			return
		case "logout":
			if err := runLogout(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "logout:", err)
				os.Exit(1)
			}
			return
		case "status":
			if err := runStatus(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "status:", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
			os.Exit(2)
		}
	}

	if err := server.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}

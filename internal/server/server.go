// Package server glues the MCP transport, tool registry, and session manager
// together. Run starts the stdio transport and blocks until the host
// disconnects.
package server

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/tools"
	"github.com/millsymills-com/protonmail-mcp/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultAPIURL = "https://mail.proton.me/api"
	serverName    = "protonmail-mcp"
)

// RegisterAll attaches every v1 tool to srv against sess. Exposed so tests
// can construct a server without owning the stdio transport.
func RegisterAll(srv *mcp.Server, sess *session.Session) {
	tools.Register(srv, tools.Deps{Session: sess})
}

// Run starts the stdio MCP server. Blocks until the host disconnects.
func Run(ctx context.Context) error {
	return RunWithOptions(ctx, defaultAPIURL, nil)
}

// RunWithOptions starts the stdio MCP server with an explicit API URL and transport.
func RunWithOptions(ctx context.Context, apiURL string, transport http.RoundTripper) error {
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	// Honour PROTONMAIL_MCP_API_URL only when no explicit override was passed,
	// so callers (CLI tests) can pin a URL without env leakage.
	if v := os.Getenv("PROTONMAIL_MCP_API_URL"); v != "" && apiURL == defaultAPIURL {
		apiURL = v
	}
	store, err := session.SelectStore(os.Getenv)
	if err != nil {
		return fmt.Errorf("credential backend: %w", err)
	}
	sess := session.New(apiURL, store, session.WithTransport(transport))
	srv := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: version.MCP}, nil)
	tools.Register(srv, tools.Deps{Session: sess})

	cfg, err := transportConfigFromEnv(os.Getenv)
	if err != nil {
		return fmt.Errorf("transport config: %w", err)
	}
	if cfg.kind == "sse" {
		return serveSSE(ctx, cfg, srv)
	}
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp server: %w", err)
	}
	return nil
}

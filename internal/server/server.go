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

const instructions = `protonmail-mcp wraps the Proton account, mail, and custom-domain APIs.

Discover before you fetch: resource IDs are not guessable. Call a list_*/search_* tool ` +
	`(proton_list_addresses, proton_search_messages, proton_list_custom_domains) to obtain an ID ` +
	`before any get_*/update_*/delete_* tool that takes one.

Reading encrypted content (mail bodies via proton_get_message with include_body, address keys) ` +
	`requires an unlocked keyring. A read that fails with a permission/keyring-unlock error means the ` +
	`session token lacks the keyring-unlock scope; the session must be re-established, not retried.

Write tools (create_*/update_*/delete_*/set_*/add_*/remove_*/disable_*/verify_*/label_*/mark_*) are registered only when ` +
	`PROTONMAIL_MCP_ENABLE_WRITES=1; permanent-delete tools additionally require PROTONMAIL_MCP_ENABLE_DANGEROUS=1. ` +
	`If a write tool is absent, writes are disabled by configuration.`

// RegisterAll attaches every v1 tool to srv against sess. Exposed so tests
// can construct a server without owning the stdio transport.
func RegisterAll(srv *mcp.Server, sess *session.Session) {
	tools.Register(srv, tools.Deps{Session: sess})
}

func newServer() *mcp.Server {
	return mcp.NewServer(
		&mcp.Implementation{Name: serverName, Version: version.MCP},
		&mcp.ServerOptions{Instructions: instructions},
	)
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
	srv := newServer()
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

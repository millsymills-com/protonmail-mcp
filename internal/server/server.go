// Package server glues the MCP transport, tool registry, and session manager
// together. Run starts the stdio transport and blocks until the host
// disconnects.
package server

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/tools"
)

const (
	defaultAPIURL = "https://mail.proton.me/api"
	serverName    = "protonmail-mcp"
	serverVersion = "v0.1.0"
)

// Run starts the stdio MCP server. Blocks until the host disconnects.
func Run(ctx context.Context) error {
	apiURL := os.Getenv("PROTONMAIL_MCP_API_URL")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	sess := session.New(apiURL, keychain.New())
	srv := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)
	tools.Register(srv, tools.Deps{Session: sess})
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp server: %w", err)
	}
	return nil
}

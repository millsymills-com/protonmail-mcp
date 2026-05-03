// Package tools registers MCP tools against an mcp.Server. Reads are always
// registered; writes are registered only when PROTONMAIL_MCP_ENABLE_WRITES=1.
package tools

import (
	"context"
	"os"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

// Deps is what handlers need. Kept tiny on purpose.
type Deps struct {
	Session *session.Session
}

// Register attaches every v1 tool to server. WritesEnabled is read once at
// registration time; tools added when it is false are simply absent from the
// MCP tool list.
func Register(server *mcp.Server, d Deps) {
	registerIdentity(server, d)
	registerAddresses(server, d)
	registerDomains(server, d)
	registerSettings(server, d)
	registerKeys(server, d)
}

// WritesEnabled returns true when PROTONMAIL_MCP_ENABLE_WRITES is set to a
// truthy value ("1", "true", "yes", case insensitive).
func WritesEnabled() bool {
	v := os.Getenv("PROTONMAIL_MCP_ENABLE_WRITES")
	switch v {
	case "1", "true", "True", "TRUE", "yes", "Yes", "YES":
		return true
	}
	return false
}

// failure converts a *proterr.Error into the MCP CallToolResult shape with
// IsError=true so the host shows structured error text without surfacing a
// transport-level failure.
func failure(perr *proterr.Error) *mcp.CallToolResult {
	if perr == nil {
		return nil
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: perr.Error()}},
	}
}

// clientOrFail centralizes the "get session.Client or return MCP error" pattern.
func clientOrFail(ctx context.Context, d Deps) (*proton.Client, *mcp.CallToolResult) {
	c, err := d.Session.Client(ctx)
	if err != nil {
		return nil, failure(proterr.Map(err))
	}
	return c, nil
}

// requireField returns a structured validation failure when value is empty.
// Used at tool entry to give callers a clear "missing X" error before any
// API call, instead of letting the raw layer reject the request with a
// less specific "domain_id is required" generic error.
func requireField(name, value string) *mcp.CallToolResult {
	if value != "" {
		return nil
	}
	return failure(&proterr.Error{
		Code:    "proton/validation",
		Message: name + " is required",
	})
}

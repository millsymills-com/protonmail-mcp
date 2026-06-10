// Package tools registers MCP tools against an mcp.Server. Reads are always
// registered; writes are registered only when PROTONMAIL_MCP_ENABLE_WRITES=1.
package tools

import (
	"context"
	"os"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Deps is what handlers need. Kept tiny on purpose. Session is an interface so
// tests can wrap a real session to drive handler error paths (e.g. a keyring
// unlock failure) without a live backend.
type Deps struct {
	Session session.Service
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
	registerMessages(server, d)
	registerLabels(server, d)
	registerDrafts(server, d)
	registerCalendar(server, d)
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

// DangerousEnabled returns true when PROTONMAIL_MCP_ENABLE_DANGEROUS is set to a
// truthy value. It gates irreversible operations (permanent delete) that sit
// above the ENABLE_WRITES tier; a dangerous tool registers only when both this
// and WritesEnabled() are true.
func DangerousEnabled() bool {
	switch os.Getenv("PROTONMAIL_MCP_ENABLE_DANGEROUS") {
	case "1", "true", "True", "TRUE", "yes", "Yes", "YES":
		return true
	}
	return false
}

// addTool registers a handler that returns the honest (Out, *proterr.Error)
// contract, adapting it to the MCP SDK's (*CallToolResult, Out, error) shape.
// A non-nil *proterr.Error becomes an IsError result; the transport-level
// error is always nil so the host renders structured error text rather than a
// protocol failure.
func addTool[In, Out any](server *mcp.Server, d Deps, t *mcp.Tool,
	fn func(context.Context, Deps, In) (Out, *proterr.Error)) {
	mcp.AddTool(server, t, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		out, perr := fn(ctx, d, in)
		if perr != nil {
			var zero Out
			return failure(perr), zero, nil
		}
		return nil, out, nil
	})
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

// client centralizes the "get session.Client or map the error" pattern.
func client(ctx context.Context, d Deps) (*proton.Client, *proterr.Error) {
	c, err := d.Session.Client(ctx)
	if err != nil {
		return nil, proterr.Map(err)
	}
	return c, nil
}

// required returns a structured validation error when value is empty. Used at
// tool entry to give callers a clear "missing X" error before any API call,
// instead of letting the raw layer reject the request with a less specific
// generic error.
func required(name, value string) *proterr.Error {
	if value != "" {
		return nil
	}
	return &proterr.Error{
		Code:    "proton/validation",
		Message: name + " is required",
	}
}

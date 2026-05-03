// Package testharness boots a go-proton-api dev server, instantiates the
// session manager + tool registry against it, and exposes a Call helper that
// invokes tools via an in-memory MCP client. Used by tools/* tests and by the
// integration suite.
//
// The harness is deliberately placed at internal/testharness (not under
// internal/tools/internal/) so that the integration suite under
// /integration/... can import it. Go's internal-package rule would otherwise
// block consumers outside internal/tools/.
package testharness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ProtonMail/go-proton-api/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/tools"
)

// Harness is a live test rig: dev server + session + MCP server/client over
// an in-memory transport. Construct with Boot.
type Harness struct {
	t       *testing.T
	srv     *server.Server
	mcp     *mcp.ClientSession
	mcpSrv  *mcp.Server
	closed  bool
	cleanup []func()
}

// Boot creates a user with the supplied email + password on a freshly-spun
// dev server, logs the session manager in against it, registers tools, and
// connects an in-memory MCP client to the server.
//
// email must be of the form "<local>@<domain>". The dev server is configured
// with the matching domain so the resulting primary address is exactly the
// supplied email — this lets tests assert against the email directly.
//
// keyring.MockInit() is called to switch go-keyring into in-memory mode so we
// never touch the user's real OS keychain.
func Boot(t *testing.T, email, password string) *Harness {
	t.Helper()
	keyring.MockInit()

	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" {
		t.Fatalf("Boot: email must be local@domain, got %q", email)
	}

	// Plain HTTP — avoids needing to inject a custom transport into
	// session.New just to skip TLS verification on the dev server's self-
	// signed cert. The dev server happily serves over HTTP when WithTLS(false)
	// is set, and the default proton.Manager transport handles plain http://.
	devsrv := server.New(server.WithTLS(false), server.WithDomain(domain))
	if _, _, err := devsrv.CreateUser(local, []byte(password)); err != nil {
		devsrv.Close()
		t.Fatalf("dev server CreateUser: %v", err)
	}

	apiURL := devsrv.GetHostURL()
	kc := keychain.New()
	sess := session.New(apiURL, kc)
	// The dev server stores accounts by the bare username (no @domain). The
	// resulting primary address is <username>@<domain>, but auth lookups go
	// through the username only, mirroring proton.local's real-server SRP
	// behavior.
	if err := sess.Login(context.Background(), session.LoginInput{Username: local, Password: password}); err != nil {
		devsrv.Close()
		t.Fatalf("session.Login: %v", err)
	}

	mcpSrv := mcp.NewServer(&mcp.Implementation{Name: "protonmail-mcp-test", Version: "0.0.0"}, nil)
	tools.Register(mcpSrv, tools.Deps{Session: sess})

	clientT, serverT := mcp.NewInMemoryTransports()

	// Connect the server side BEFORE the client — the in-memory transports
	// require this ordering because Client.Connect runs the MCP initialization
	// handshake immediately.
	srvSession, err := mcpSrv.Connect(context.Background(), serverT, nil)
	if err != nil {
		devsrv.Close()
		t.Fatalf("mcp server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "protonmail-mcp-test-client", Version: "0.0.0"}, nil)
	csess, err := client.Connect(context.Background(), clientT, nil)
	if err != nil {
		_ = srvSession.Close()
		devsrv.Close()
		t.Fatalf("mcp client connect: %v", err)
	}

	h := &Harness{
		t:      t,
		srv:    devsrv,
		mcp:    csess,
		mcpSrv: mcpSrv,
		cleanup: []func(){
			func() { _ = csess.Close() },
			func() { _ = srvSession.Close() },
			func() { devsrv.Close() },
		},
	}
	t.Cleanup(h.Close)
	return h
}

// Close releases the MCP sessions and dev server. Safe to call multiple times.
func (h *Harness) Close() {
	if h.closed {
		return
	}
	h.closed = true
	for _, fn := range h.cleanup {
		fn()
	}
}

// Call invokes a tool by name with the given arguments and unmarshals its
// structured output into a map[string]any. Returns an error if the tool
// reported IsError or the call itself failed at the transport layer.
func (h *Harness) Call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	res, err := h.mcp.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, toolErr(res)
	}

	// The SDK populates StructuredContent with the typed Out value (any). For
	// our tools that's the structured output struct. Round-trip through JSON
	// to land on a map[string]any consumers can poke at without importing
	// internal types.
	if res.StructuredContent == nil {
		// Fall back to text content: tools that returned an empty struct will
		// still have a JSON text content block.
		return mapFromContent(res), nil
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return nil, fmt.Errorf("marshal structured content: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("unmarshal structured content: %w", err)
	}
	return out, nil
}

func mapFromContent(res *mcp.CallToolResult) map[string]any {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			var m map[string]any
			if err := json.Unmarshal([]byte(tc.Text), &m); err == nil {
				return m
			}
		}
	}
	return nil
}

func toolErr(res *mcp.CallToolResult) error {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			return errors.New(tc.Text)
		}
	}
	return errors.New("tool error (no detail)")
}

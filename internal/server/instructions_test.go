package server

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/millsymills-com/protonmail-mcp/internal/tools"
)

// The instructions string lands in the host system prompt at initialize time;
// assert the client actually receives the production-wired value and that it
// carries the cross-tool invariants it exists to convey.
func TestServerAdvertisesInstructions(t *testing.T) {
	srv := newServer()

	ctx := context.Background()
	clientT, serverT := mcp.NewInMemoryTransports()

	srvSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = srvSession.Close() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	got := cs.InitializeResult().Instructions
	if got != instructions {
		t.Fatalf("client instructions = %q, want %q", got, instructions)
	}
	for _, want := range []string{"proton_search_messages", "keyring", "PROTONMAIL_MCP_ENABLE_WRITES"} {
		if !strings.Contains(got, want) {
			t.Errorf("instructions missing %q", want)
		}
	}
}

// The instructions enumerate write-tool prefixes as a hint to models; a prefix
// present on a registered write tool but absent from the list reads to a model
// as "not a gated write". Derive the prefix set from the real tool registry
// (writes-on minus writes-off) so the enumeration cannot silently drift.
func TestInstructionsEnumerateEveryWritePrefix(t *testing.T) {
	prefixes := registeredWritePrefixes(t)
	if len(prefixes) == 0 {
		t.Fatal("no write tools registered; prefix derivation is broken")
	}
	for prefix := range prefixes {
		if !strings.Contains(instructions, prefix) {
			t.Errorf("instructions omit registered write-tool prefix %q", prefix)
		}
	}
}

// registeredWritePrefixes returns the set of "<verb>_*" prefixes carried by
// tools that exist only when writes are enabled.
func registeredWritePrefixes(t *testing.T) map[string]struct{} {
	t.Helper()
	on := listToolNames(t, "1")
	off := listToolNames(t, "")
	prefixes := make(map[string]struct{})
	for name := range on {
		if _, isRead := off[name]; isRead {
			continue
		}
		verb, _, _ := strings.Cut(strings.TrimPrefix(name, "proton_"), "_")
		prefixes[verb+"_*"] = struct{}{}
	}
	return prefixes
}

// listToolNames registers the tool set under the given PROTONMAIL_MCP_ENABLE_WRITES
// value and returns the advertised tool names. A nil session is safe: handlers
// touch it only when invoked, never at registration.
func listToolNames(t *testing.T, writes string) map[string]bool {
	t.Helper()
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", writes)

	srv := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "test"}, nil)
	tools.Register(srv, tools.Deps{})

	ctx := context.Background()
	clientT, serverT := mcp.NewInMemoryTransports()
	srvSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = srvSession.Close() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	listed, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make(map[string]bool, len(listed.Tools))
	for _, tl := range listed.Tools {
		names[tl.Name] = true
	}
	return names
}

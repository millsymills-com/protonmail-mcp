package server

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

package tools_test

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

func TestSearchMessagesHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "search_messages_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_search_messages", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["messages"]; !ok {
		t.Fatalf("envelope missing %q", "messages")
	}
}

func TestGetMessageHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "get_message_happy")
	defer h.Close()
	ctx := context.Background()

	searchOut, err := h.Call(ctx, "proton_search_messages", map[string]any{"limit": float64(1)})
	if err != nil {
		t.Fatalf("search_messages: %v", err)
	}
	msgs, ok := searchOut["messages"].([]any)
	if !ok {
		t.Fatalf("messages not a list: %#v", searchOut["messages"])
	}
	// The cassette was recorded against an account with at least one message.
	// Skip rather than fail if the cassette is present but empty (e.g. new test
	// account with no messages).
	if len(msgs) == 0 {
		t.Skip("cassette has no messages; re-record after sending a message to the test account")
	}
	first, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatalf("message[0] not an object: %#v", msgs[0])
	}
	id, ok := first["id"].(string)
	if !ok || id == "" {
		t.Fatalf("message[0].id missing or empty: %#v", first)
	}

	out, err := h.Call(ctx, "proton_get_message", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("get_message: %v", err)
	}
	if _, ok := out["message"]; !ok {
		t.Fatalf("envelope missing %q", "message")
	}
}

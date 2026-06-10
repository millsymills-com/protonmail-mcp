package tools_test

import (
	"context"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

// fetchMessageState returns the message stub's label_ids and unread flag via
// proton_get_message, so each organize operation is verified against server
// state rather than the handler's echoed inputs.
func fetchMessageState(ctx context.Context, t *testing.T, h *testharness.Harness, id string) ([]any, bool) {
	t.Helper()
	out, err := h.Call(ctx, "proton_get_message", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("get_message: %v", err)
	}
	msg, ok := out["message"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing message: %#v", out)
	}
	labelIDs, _ := msg["label_ids"].([]any)
	unread, _ := msg["unread"].(bool)
	return labelIDs, unread
}

func hasLabel(labelIDs []any, want string) bool {
	for _, l := range labelIDs {
		if s, _ := l.(string); s == want {
			return true
		}
	}
	return false
}

func TestOrganizeToolsDevServer(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootDevServer(t, "organizer@example.test", "hunter2")
	defer h.Close()
	ctx := context.Background()

	raw := []byte("From: organizer@example.test\r\n" +
		"To: organizer@example.test\r\n" +
		"Subject: organize test\r\n" +
		"\r\norganize test body\r\n")
	id := h.SeedMessage(t, raw)

	star, err := h.Call(ctx, "proton_label_messages", map[string]any{
		"message_ids": []any{id}, "label_id": "10", "action": "add",
	})
	if err != nil {
		t.Fatalf("label add: %v", err)
	}
	if ok, _ := star["ok"].(bool); !ok {
		t.Fatalf("want ok=true, got %#v", star)
	}
	if labels, _ := fetchMessageState(ctx, t, h, id); !hasLabel(labels, "10") {
		t.Fatalf("label add did not take: label_ids=%#v", labels)
	}

	unstar, err := h.Call(ctx, "proton_label_messages", map[string]any{
		"message_ids": []any{id}, "label_id": "10", "action": "remove",
	})
	if err != nil {
		t.Fatalf("label remove: %v", err)
	}
	if ok, _ := unstar["ok"].(bool); !ok {
		t.Fatalf("want ok=true, got %#v", unstar)
	}
	if labels, _ := fetchMessageState(ctx, t, h, id); hasLabel(labels, "10") {
		t.Fatalf("label remove did not take: label_ids=%#v", labels)
	}

	read, err := h.Call(ctx, "proton_mark_messages", map[string]any{
		"message_ids": []any{id}, "read": true,
	})
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if ok, _ := read["ok"].(bool); !ok {
		t.Fatalf("want ok=true, got %#v", read)
	}
	if _, unreadFlag := fetchMessageState(ctx, t, h, id); unreadFlag {
		t.Fatal("mark read did not take: message still unread")
	}

	unread, err := h.Call(ctx, "proton_mark_messages", map[string]any{
		"message_ids": []any{id}, "read": false,
	})
	if err != nil {
		t.Fatalf("mark unread: %v", err)
	}
	if ok, _ := unread["ok"].(bool); !ok {
		t.Fatalf("want ok=true, got %#v", unread)
	}
	if _, unreadFlag := fetchMessageState(ctx, t, h, id); !unreadFlag {
		t.Fatal("mark unread did not take: message still read")
	}
}

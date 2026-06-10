package tools_test

import (
	"context"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

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

	star, err := h.Call(ctx, "proton_label_message", map[string]any{
		"message_ids": []any{id}, "label_id": "10", "action": "add",
	})
	if err != nil {
		t.Fatalf("label add: %v", err)
	}
	if ok, _ := star["ok"].(bool); !ok {
		t.Fatalf("want ok=true, got %#v", star)
	}

	unstar, err := h.Call(ctx, "proton_label_message", map[string]any{
		"message_ids": []any{id}, "label_id": "10", "action": "remove",
	})
	if err != nil {
		t.Fatalf("label remove: %v", err)
	}
	if ok, _ := unstar["ok"].(bool); !ok {
		t.Fatalf("want ok=true, got %#v", unstar)
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

	unread, err := h.Call(ctx, "proton_mark_messages", map[string]any{
		"message_ids": []any{id}, "read": false,
	})
	if err != nil {
		t.Fatalf("mark unread: %v", err)
	}
	if ok, _ := unread["ok"].(bool); !ok {
		t.Fatalf("want ok=true, got %#v", unread)
	}
}

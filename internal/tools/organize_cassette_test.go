package tools_test

import (
	"context"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

func TestDeleteMessagesHappyCassette(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	t.Setenv("PROTONMAIL_MCP_ENABLE_DANGEROUS", "1")
	h := testharness.BootWithCassette(t, "delete_messages_happy")
	defer h.Close()
	ctx := context.Background()
	page, err := h.Call(ctx, "proton_search_messages", map[string]any{"label_id": "8", "limit": float64(1)})
	if err != nil {
		t.Fatalf("search drafts: %v", err)
	}
	msgs, _ := page["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatal("cassette has no draft to delete; re-record delete_messages_happy")
	}
	id, _ := msgs[0].(map[string]any)["id"].(string)
	out, err := h.Call(ctx, "proton_delete_messages", map[string]any{"message_ids": []any{id}})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("want ok=true, got %#v", out)
	}
}

func TestLabelMessageHappyCassette(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "organize_label_happy")
	defer h.Close()
	ctx := context.Background()
	page, err := h.Call(ctx, "proton_search_messages", map[string]any{"limit": float64(1)})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	msgs, _ := page["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatalf("cassette has no messages; re-record organize_label_happy")
	}
	id, _ := msgs[0].(map[string]any)["id"].(string)
	out, err := h.Call(ctx, "proton_label_messages", map[string]any{
		"message_ids": []any{id}, "label_id": "10", "action": "add",
	})
	if err != nil {
		t.Fatalf("label: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("want ok=true, got %#v", out)
	}
	out, err = h.Call(ctx, "proton_label_messages", map[string]any{
		"message_ids": []any{id}, "label_id": "10", "action": "remove",
	})
	if err != nil {
		t.Fatalf("unlabel: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("want ok=true, got %#v", out)
	}
}

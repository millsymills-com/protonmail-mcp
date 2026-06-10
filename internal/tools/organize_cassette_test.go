package tools_test

import (
	"context"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

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

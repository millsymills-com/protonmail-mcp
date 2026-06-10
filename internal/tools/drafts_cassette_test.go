package tools_test

import (
	"context"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

func TestCreateDraftHappyCassette(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "create_draft_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_create_draft", map[string]any{
		"to":      []any{"recipient@example.test"},
		"subject": "Hello from the agent",
		"body":    "This is a draft body.",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["message"]; !ok {
		t.Fatalf("envelope missing %q: %#v", "message", out)
	}
}

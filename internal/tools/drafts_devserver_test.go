package tools_test

import (
	"context"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

// TestCreateAndUpdateDraftDevServer drives both draft handlers end-to-end
// against the go-proton-api dev server: a live keyring unlock, real
// body encryption, and a create-then-update round trip.
func TestCreateAndUpdateDraftDevServer(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootDevServer(t, "drafter@example.test", "hunter2")
	defer h.Close()
	ctx := context.Background()

	created, err := h.Call(ctx, "proton_create_draft", map[string]any{
		"to":      []any{"recipient@example.test"},
		"subject": "Hello from the agent",
		"body":    "This is a draft body.",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	msg, ok := created["message"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing message: %#v", created)
	}
	id, _ := msg["id"].(string)
	if id == "" {
		t.Fatalf("created draft has no id: %#v", msg)
	}

	updated, err := h.Call(ctx, "proton_update_draft", map[string]any{
		"id":      id,
		"to":      []any{"recipient@example.test"},
		"subject": "Updated subject",
		"body":    "Updated body.",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	umsg, ok := updated["message"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing message: %#v", updated)
	}
	if got, _ := umsg["subject"].(string); got != "Updated subject" {
		t.Fatalf("subject not updated: %#v", umsg)
	}
}

package tools_test

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

// TestGetMessageIncludeBodyHappy seeds a real message on the dev server and
// asserts proton_get_message{include_body:true} decrypts and returns its body.
// This drives the handler's include_body branch end-to-end: Session.Keyrings
// runs a live cache-miss unlock (against the production fetcher fallback) and
// DecryptBody recovers the plaintext.
func TestGetMessageIncludeBodyHappy(t *testing.T) {
	h := testharness.BootDevServer(t, "user@example.test", "hunter2")
	defer h.Close()
	ctx := context.Background()

	const body = "delivery confirmed via seeded message"
	raw := []byte("From: user@example.test\r\n" +
		"To: user@example.test\r\n" +
		"Subject: seeded\r\n" +
		"\r\n" + body + "\r\n")
	id := h.SeedMessage(t, raw)

	out, err := h.Call(ctx, "proton_get_message", map[string]any{"id": id, "include_body": true})
	if err != nil {
		t.Fatalf("get_message include_body: %v", err)
	}
	got, ok := out["body"].(string)
	if !ok {
		t.Fatalf("body missing or not a string: %#v", out["body"])
	}
	if got == "" {
		t.Fatal("decrypted body is empty")
	}
}

package tools_test

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

func TestWhoamiRoundTrip(t *testing.T) {
	h := testharness.Boot(t, "user@example.test", "hunter2")
	defer h.Close()

	out, err := h.Call(context.Background(), "proton_whoami", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if out["email"] != "user@example.test" {
		t.Fatalf("unexpected email: %#v", out)
	}
}

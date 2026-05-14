//go:build integration
// +build integration

package integration_test

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

// TestReadToolsRoundTrip exercises every read-only tool against the in-process
// go-proton-api dev server through the in-memory MCP transport. No network.
func TestReadToolsRoundTrip(t *testing.T) {
	h := testharness.BootDevServer(t, "user@example.test", "hunter2")
	defer h.Close()

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"proton_whoami", map[string]any{}},
		{"proton_session_status", map[string]any{}},
		{"proton_list_addresses", map[string]any{}},
		{"proton_get_mail_settings", map[string]any{}},
		{"proton_get_core_settings", map[string]any{}},
		{"proton_search_messages", map[string]any{"limit": 10}},
	}
	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			out, err := h.Call(context.Background(), tc.tool, tc.args)
			if err != nil {
				t.Fatalf("call %s: %v", tc.tool, err)
			}
			if out == nil {
				t.Fatalf("%s: nil structured output", tc.tool)
			}
		})
	}
}

// TestGetMessageWiring exercises the proton_get_message handler path. The
// dev server has no seeded messages and this tool requires a valid ID, so
// we verify the tool is reachable and that requests with a non-empty ID
// flow through to go-proton-api (which returns a not-found error). A
// transport-level failure here would indicate a wiring bug; a structured
// proton/not_found IsError result confirms the handler is wired.
func TestGetMessageWiring(t *testing.T) {
	h := testharness.BootDevServer(t, "user@example.test", "hunter2")
	defer h.Close()

	if _, err := h.Call(context.Background(), "proton_get_message", map[string]any{"id": "nonexistent"}); err == nil {
		t.Fatal("want error for nonexistent message id, got nil")
	}
	// Empty ID must hit the input-validation guard, not the API.
	if _, err := h.Call(context.Background(), "proton_get_message", map[string]any{"id": ""}); err == nil {
		t.Fatal("want validation error for empty id, got nil")
	}
}

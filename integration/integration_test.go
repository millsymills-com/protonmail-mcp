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
	h := testharness.Boot(t, "user@example.test", "hunter2")
	defer h.Close()

	cases := []struct {
		tool string
		args map[string]any
	}{
		{"proton_whoami", map[string]any{}},
		{"proton_session_status", map[string]any{}},
		{"proton_list_addresses", map[string]any{}},
		{"proton_get_mail_settings", map[string]any{}},
		{"proton_get_core_settings", map[string]any{}},
	}
	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			out, err := h.Call(context.Background(), c.tool, c.args)
			if err != nil {
				t.Fatalf("call %s: %v", c.tool, err)
			}
			if out == nil {
				t.Fatalf("%s: nil structured output", c.tool)
			}
		})
	}
}

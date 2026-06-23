package tools_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// dangerousTools enumerates the tools gated behind BOTH ENABLE_WRITES and
// ENABLE_DANGEROUS, alongside a minimal valid handler request for each.
var dangerousTools = []struct {
	name string
	args map[string]any
}{
	{"proton_delete_messages", map[string]any{"message_ids": []any{"any-id"}}},
	{"proton_delete_address", map[string]any{"id": "any-id"}},
	{"proton_remove_custom_domain", map[string]any{"id": "any-id"}},
}

// Each dangerous tool must be absent unless BOTH ENABLE_WRITES and
// ENABLE_DANGEROUS are set. BootDevServer registers tools at boot reading the
// env set above.
func TestDangerousToolsGateMatrix(t *testing.T) {
	gates := []struct {
		writes, dangerous string
		present           bool
	}{
		{"", "", false},
		{"1", "", false},
		{"", "1", false},
		{"1", "1", true},
	}
	for _, tool := range dangerousTools {
		for _, g := range gates {
			t.Run(fmt.Sprintf("%s/writes=%s,dangerous=%s", tool.name, g.writes, g.dangerous), func(t *testing.T) {
				t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", g.writes)
				t.Setenv("PROTONMAIL_MCP_ENABLE_DANGEROUS", g.dangerous)
				h := testharness.BootDevServer(t, "gate@example.test", "password")
				defer h.Close()
				res, err := h.MCP().ListTools(context.Background(), &mcp.ListToolsParams{})
				if err != nil {
					t.Fatalf("ListTools: %v", err)
				}
				names := make([]string, len(res.Tools))
				for i, registered := range res.Tools {
					names[i] = registered.Name
				}
				got := slices.Contains(names, tool.name)
				if got != g.present {
					t.Fatalf("%s: writes=%q dangerous=%q: present=%v want %v", tool.name, g.writes, g.dangerous, got, g.present)
				}
			})
		}
	}
}

// Each handler re-checks the gates at call time (belt-and-suspenders above the
// registration gate). Clearing DANGEROUS after boot must yield the structured
// writes-disabled error rather than executing the destructive operation.
func TestDangerousToolsRuntimeRecheck(t *testing.T) {
	for _, tool := range dangerousTools {
		t.Run(tool.name, func(t *testing.T) {
			t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
			t.Setenv("PROTONMAIL_MCP_ENABLE_DANGEROUS", "1")
			h := testharness.BootDevServer(t, "recheck@example.test", "password")
			defer h.Close()
			t.Setenv("PROTONMAIL_MCP_ENABLE_DANGEROUS", "")
			_, err := h.Call(context.Background(), tool.name, tool.args)
			if err == nil || !strings.Contains(err.Error(), "proton/writes_disabled") {
				t.Fatalf("%s: want proton/writes_disabled from the runtime re-check, got %v", tool.name, err)
			}
		})
	}
}

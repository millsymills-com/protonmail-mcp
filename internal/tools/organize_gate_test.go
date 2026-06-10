package tools_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// delete_messages must be absent unless BOTH ENABLE_WRITES and ENABLE_DANGEROUS
// are set. BootDevServer registers tools at boot reading the env set above.
func TestDeleteMessagesGateMatrix(t *testing.T) {
	cases := []struct {
		writes, dangerous string
		present           bool
	}{
		{"", "", false},
		{"1", "", false},
		{"", "1", false},
		{"1", "1", true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("writes=%s,dangerous=%s", tc.writes, tc.dangerous), func(t *testing.T) {
			t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", tc.writes)
			t.Setenv("PROTONMAIL_MCP_ENABLE_DANGEROUS", tc.dangerous)
			h := testharness.BootDevServer(t, "gate@example.test", "password")
			defer h.Close()
			res, err := h.MCP().ListTools(context.Background(), &mcp.ListToolsParams{})
			if err != nil {
				t.Fatalf("ListTools: %v", err)
			}
			names := make([]string, len(res.Tools))
			for i, tool := range res.Tools {
				names[i] = tool.Name
			}
			got := slices.Contains(names, "proton_delete_messages")
			if got != tc.present {
				t.Fatalf("writes=%q dangerous=%q: present=%v want %v", tc.writes, tc.dangerous, got, tc.present)
			}
		})
	}
}

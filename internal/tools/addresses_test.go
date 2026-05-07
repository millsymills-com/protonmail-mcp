package tools_test

import (
	"os"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/tools"
)

func TestWritesEnabledRespectsEnv(t *testing.T) {
	_ = os.Unsetenv("PROTONMAIL_MCP_ENABLE_WRITES")
	if tools.WritesEnabled() {
		t.Fatalf("want false when env unset")
	}
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"enabled_one", "1", true},
		{"enabled_true", "true", true},
		{"enabled_yes", "yes", true},
		{"disabled_no", "no", false},
		{"disabled_zero", "0", false},
		{"disabled_empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", tc.value)
			if got := tools.WritesEnabled(); got != tc.want {
				t.Fatalf("env=%q: got %v want %v", tc.value, got, tc.want)
			}
		})
	}
}

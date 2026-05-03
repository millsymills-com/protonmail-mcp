package tools_test

import (
	"os"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/tools"
)

func TestWritesEnabledRespectsEnv(t *testing.T) {
	os.Unsetenv("PROTONMAIL_MCP_ENABLE_WRITES")
	if tools.WritesEnabled() {
		t.Fatalf("want false when env unset")
	}
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	if !tools.WritesEnabled() {
		t.Fatalf("want true when env=1")
	}
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "no")
	if tools.WritesEnabled() {
		t.Fatalf("want false when env=no")
	}
}

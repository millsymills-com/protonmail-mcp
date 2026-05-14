package tools_test

import (
	"context"
	"os"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
	"github.com/millsmillsymills/protonmail-mcp/internal/tools"
)

func TestListAddressesHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "list_addresses_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_list_addresses", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["addresses"]; !ok {
		t.Fatalf("envelope missing %q", "addresses")
	}
}

func TestGetAddressHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "get_address_happy")
	defer h.Close()
	ctx := context.Background()

	addrsOut, err := h.Call(ctx, "proton_list_addresses", map[string]any{})
	if err != nil {
		t.Fatalf("list_addresses: %v", err)
	}
	addrs, ok := addrsOut["addresses"].([]any)
	if !ok || len(addrs) == 0 {
		t.Fatalf("expected at least one address, got %#v", addrsOut)
	}
	first, ok := addrs[0].(map[string]any)
	if !ok {
		t.Fatalf("address[0] not an object: %#v", addrs[0])
	}
	id, ok := first["id"].(string)
	if !ok || id == "" {
		t.Fatalf("address[0].id missing or empty: %#v", first)
	}

	out, err := h.Call(ctx, "proton_get_address", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("get_address: %v", err)
	}
	if _, ok := out["address"]; !ok {
		t.Fatalf("envelope missing %q", "address")
	}
}

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

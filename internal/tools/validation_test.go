package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

// TestMissingRequiredFields exercises the requireField guard on every
// MCP tool that has one. Each call uses an empty argument map; the harness
// boots from a cassette only because BootWithCassette requires it, but no
// network interaction actually happens — the handler returns the validation
// failure before any session call.
func TestMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		field    string
		writable bool
	}{
		{"get_message_missing_id", "proton_get_message", map[string]any{}, "id", false},
		{"get_address_missing_id", "proton_get_address", map[string]any{}, "id", false},
		{"list_address_keys_missing_id", "proton_list_address_keys",
			map[string]any{}, "address_id", false},
		{"get_catchall_missing_domain_id", "proton_get_catchall",
			map[string]any{}, "domain_id", false},
		{"get_custom_domain_missing_id", "proton_get_custom_domain",
			map[string]any{}, "id", false},
		{"update_mail_settings_no_fields", "proton_update_mail_settings",
			map[string]any{}, "", true},
		{"update_core_settings_no_fields", "proton_update_core_settings",
			map[string]any{}, "", true},
		{"update_address_missing_id", "proton_update_address",
			map[string]any{"display_name": "x"}, "id", true},
		{"set_address_status_missing_id", "proton_set_address_status",
			map[string]any{"enabled": true}, "id", true},
		{"create_address_missing_domain_id", "proton_create_address",
			map[string]any{"local_part": "x"}, "domain_id", true},
		{"create_address_missing_local_part", "proton_create_address",
			map[string]any{"domain_id": "REDACTED_DOMAINID_1"}, "local_part", true},
		{"delete_address_missing_id", "proton_delete_address",
			map[string]any{}, "id", true},
		{"add_custom_domain_missing_name", "proton_add_custom_domain",
			map[string]any{}, "domain_name", true},
		{"verify_custom_domain_missing_id", "proton_verify_custom_domain",
			map[string]any{}, "id", true},
		{"remove_custom_domain_missing_id", "proton_remove_custom_domain",
			map[string]any{}, "id", true},
		{"set_catchall_missing_domain_id", "proton_set_catchall",
			map[string]any{"destination_address_id": "x"}, "domain_id", true},
		{"set_catchall_missing_dest", "proton_set_catchall",
			map[string]any{"domain_id": "x"}, "destination_address_id", true},
		{"disable_catchall_missing_domain_id", "proton_disable_catchall",
			map[string]any{}, "domain_id", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.writable {
				t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
			}
			h := testharness.BootWithCassette(t, "whoami_happy")
			defer h.Close()
			_, err := h.Call(context.Background(), tc.tool, tc.args)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tc.tool)
			}
			if tc.field != "" && !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("error for %s does not mention %q: %v", tc.tool, tc.field, err)
			}
		})
	}
}

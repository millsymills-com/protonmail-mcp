package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

// TestToolErrorPathsUpstream exercises each tool's "API call failed →
// failure(proterr.Map(err))" branch by reusing a cassette (whoami_happy)
// whose interaction set doesn't cover the requested endpoint. The replay
// returns "interaction not found" which proterr maps to proton/upstream.
func TestToolErrorPathsUpstream(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		writable bool
	}{
		{"list_addresses_upstream", "proton_list_addresses", map[string]any{}, false},
		{"get_address_upstream", "proton_get_address", map[string]any{"id": "REDACTED_ADDRESSID_1"}, false},
		{"list_address_keys_upstream", "proton_list_address_keys",
			map[string]any{"address_id": "REDACTED_ADDRESSID_1"}, false},
		{"get_mail_settings_upstream", "proton_get_mail_settings", map[string]any{}, false},
		{"get_core_settings_upstream", "proton_get_core_settings", map[string]any{}, false},
		{"search_messages_upstream", "proton_search_messages", map[string]any{"query": "x"}, false},
		{"get_message_upstream", "proton_get_message", map[string]any{"id": "REDACTED_MESSAGEID_1"}, false},
		{"list_custom_domains_upstream", "proton_list_custom_domains", map[string]any{}, false},
		{"get_custom_domain_upstream", "proton_get_custom_domain",
			map[string]any{"id": "REDACTED_DOMAINID_1"}, false},
		{"get_catchall_upstream", "proton_get_catchall",
			map[string]any{"domain_id": "REDACTED_DOMAINID_1"}, false},
		{"list_calendars_upstream", "proton_list_calendars", map[string]any{}, false},
		{"list_events_upstream", "proton_list_events",
			map[string]any{"start": "2026-06-01T00:00:00Z", "end": "2026-06-30T23:59:59Z"}, false},
		{"get_event_upstream", "proton_get_event",
			map[string]any{"calendar_id": "c1", "event_id": "e1"}, false},
		{"add_custom_domain_upstream", "proton_add_custom_domain",
			map[string]any{"domain_name": "x.test"}, true},
		{"verify_custom_domain_upstream", "proton_verify_custom_domain",
			map[string]any{"id": "REDACTED_DOMAINID_1"}, true},
		{"remove_custom_domain_upstream", "proton_remove_custom_domain",
			map[string]any{"id": "REDACTED_DOMAINID_1"}, true},
		{"set_catchall_upstream", "proton_set_catchall", map[string]any{
			"domain_id":              "REDACTED_DOMAINID_1",
			"destination_address_id": "REDACTED_ADDRESSID_1",
		}, true},
		{"disable_catchall_upstream", "proton_disable_catchall",
			map[string]any{"domain_id": "REDACTED_DOMAINID_1"}, true},
		{"create_address_upstream", "proton_create_address",
			map[string]any{"domain_id": "REDACTED_DOMAINID_1", "local_part": "x"}, true},
		{"delete_address_upstream", "proton_delete_address",
			map[string]any{"id": "REDACTED_ADDRESSID_1"}, true},
		{"update_address_display_name_upstream", "proton_update_address",
			map[string]any{"id": "REDACTED_ADDRESSID_1", "display_name": "X"}, true},
		{"update_address_signature_upstream", "proton_update_address",
			map[string]any{"id": "REDACTED_ADDRESSID_1", "signature": "X"}, true},
		{"set_address_status_off_upstream", "proton_set_address_status",
			map[string]any{"id": "REDACTED_ADDRESSID_1", "enabled": false}, true},
		{"set_address_status_on_upstream", "proton_set_address_status",
			map[string]any{"id": "REDACTED_ADDRESSID_1", "enabled": true}, true},
		{"update_mail_settings_upstream", "proton_update_mail_settings",
			map[string]any{"signature": "X"}, true},
		{"update_core_settings_upstream", "proton_update_core_settings",
			map[string]any{"telemetry": true}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.writable {
				t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
			}
			if tc.tool == "proton_delete_address" || tc.tool == "proton_remove_custom_domain" {
				t.Setenv("PROTONMAIL_MCP_ENABLE_DANGEROUS", "1")
			}
			h := testharness.BootWithCassette(t, "whoami_happy")
			defer h.Close()
			_, err := h.Call(context.Background(), tc.tool, tc.args)
			if err == nil {
				t.Fatalf("expected error from %s when interaction is unrecorded, got nil", tc.tool)
			}
			// proton/upstream is the canonical mapping for transport / "interaction
			// not found" errors. Other codes (e.g. proton/auth_required) are also
			// acceptable mappings the test wrapper might surface — we only insist
			// the error is a proton/* category, not a stray Go error.
			if !strings.Contains(err.Error(), "proton/") {
				t.Fatalf("error did not surface a proton/* code: %v", err)
			}
		})
	}
}

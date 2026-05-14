package tools_test

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

func TestCreateAddressHappy(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "create_delete_address")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_create_address", map[string]any{
		"domain_id":    "REDACTED_DOMAINID_1",
		"local_part":   "record-test",
		"display_name": "Record Test",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["id"]; !ok {
		t.Fatalf("envelope missing %q: %#v", "id", out)
	}
	if _, ok := out["email"]; !ok {
		t.Fatalf("envelope missing %q: %#v", "email", out)
	}
}

func TestDeleteAddressHappy(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "create_delete_address")
	defer h.Close()
	ctx := context.Background()
	createOut, err := h.Call(ctx, "proton_create_address", map[string]any{
		"domain_id":    "REDACTED_DOMAINID_1",
		"local_part":   "record-test",
		"display_name": "Record Test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id, _ := createOut["id"].(string)
	if id == "" {
		t.Fatal("create returned no id")
	}
	_, err = h.Call(ctx, "proton_delete_address", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestSetAddressStatusOffHappy(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "address_status_toggle")
	defer h.Close()
	ctx := context.Background()
	createOut, err := h.Call(ctx, "proton_create_address", map[string]any{
		"domain_id":  "REDACTED_DOMAINID_1",
		"local_part": "status-test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id, _ := createOut["id"].(string)
	if id == "" {
		t.Fatal("create returned no id")
	}
	out, err := h.Call(ctx, "proton_set_address_status", map[string]any{
		"id":      id,
		"enabled": false,
	})
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %#v", out)
	}
}

func TestSetAddressStatusOnHappy(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "address_status_toggle")
	defer h.Close()
	ctx := context.Background()
	createOut, err := h.Call(ctx, "proton_create_address", map[string]any{
		"domain_id":  "REDACTED_DOMAINID_1",
		"local_part": "status-test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id, _ := createOut["id"].(string)
	if id == "" {
		t.Fatal("create returned no id")
	}
	_, err = h.Call(ctx, "proton_set_address_status", map[string]any{
		"id":      id,
		"enabled": false,
	})
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	out, err := h.Call(ctx, "proton_set_address_status", map[string]any{
		"id":      id,
		"enabled": true,
	})
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %#v", out)
	}
}

func TestUpdateAddressDisplayNameHappy(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "update_address_display_name")
	defer h.Close()
	// display_name is a global mail setting, not per-address; the id parameter
	// is forwarded to the API but the value has no effect on which setting is
	// changed (Proton's SetDisplayName applies account-wide).
	out, err := h.Call(context.Background(), "proton_update_address", map[string]any{
		"id":           "REDACTED_ADDRESSID_1",
		"display_name": "Record Test Name",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %#v", out)
	}
}

package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

func TestGetCatchallEnabled(t *testing.T) {
	h := testharness.BootWithCassette(t, "get_catchall_enabled")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_get_catchall",
		map[string]any{"domain_id": "REDACTED_DOMAINID_1"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	enabled, _ := out["enabled"].(bool)
	if !enabled {
		t.Fatalf("expected enabled=true, got %#v", out)
	}
	dest, _ := out["destination_address_id"].(string)
	if dest == "" {
		t.Fatalf("expected destination_address_id, got %#v", out)
	}
}

func TestGetCatchallDisabled(t *testing.T) {
	h := testharness.BootWithCassette(t, "get_catchall_disabled")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_get_catchall",
		map[string]any{"domain_id": "REDACTED_DOMAINID_1"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	enabled, _ := out["enabled"].(bool)
	if enabled {
		t.Fatalf("expected enabled=false, got %#v", out)
	}
}

// TestGetCatchallMissingDomainID covers the input-validation branch.
func TestGetCatchallMissingDomainID(t *testing.T) {
	h := testharness.BootWithCassette(t, "get_catchall_enabled")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_get_catchall", map[string]any{})
	if err == nil {
		t.Fatal("expected validation error for missing domain_id")
	}
	if !strings.Contains(err.Error(), "domain_id") {
		t.Fatalf("error does not mention domain_id: %v", err)
	}
}

func TestSetCatchallHappy(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "set_catchall_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_set_catchall", map[string]any{
		"domain_id":              "REDACTED_DOMAINID_1",
		"destination_address_id": "REDACTED_ADDRESSID_1",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %#v", out)
	}
}

// TestSetCatchallAddressNotOnDomain covers the "address not in domain"
// validation branch.
func TestSetCatchallAddressNotOnDomain(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "set_catchall_happy")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_set_catchall", map[string]any{
		"domain_id":              "REDACTED_DOMAINID_1",
		"destination_address_id": "REDACTED_OTHERADDRESS_1",
	})
	if err == nil {
		t.Fatal("expected validation error when address is not on the domain")
	}
	if !strings.Contains(err.Error(), "proton/validation") {
		t.Fatalf("expected proton/validation error, got: %v", err)
	}
}

// TestSetCatchallMissingDomainID covers the missing-domain_id branch.
func TestSetCatchallMissingDomainID(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "set_catchall_happy")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_set_catchall", map[string]any{
		"destination_address_id": "REDACTED_ADDRESSID_1",
	})
	if err == nil {
		t.Fatal("expected validation error for missing domain_id")
	}
}

// TestSetCatchallMissingAddressID covers the missing-destination-address branch.
func TestSetCatchallMissingAddressID(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "set_catchall_happy")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_set_catchall", map[string]any{
		"domain_id": "REDACTED_DOMAINID_1",
	})
	if err == nil {
		t.Fatal("expected validation error for missing destination_address_id")
	}
}

func TestDisableCatchallHappy(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "disable_catchall_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_disable_catchall", map[string]any{
		"domain_id": "REDACTED_DOMAINID_1",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %#v", out)
	}
}

// TestDisableCatchallMissingDomainID covers the missing-domain_id branch.
func TestDisableCatchallMissingDomainID(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "disable_catchall_happy")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_disable_catchall", map[string]any{})
	if err == nil {
		t.Fatal("expected validation error for missing domain_id")
	}
}

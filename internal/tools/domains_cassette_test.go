package tools_test

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

func TestListCustomDomainsHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "list_custom_domains_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_list_custom_domains", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["domains"]; !ok {
		t.Fatalf("envelope missing %q", "domains")
	}
}

func TestGetCustomDomainHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "get_custom_domain_happy")
	defer h.Close()
	ctx := context.Background()

	listOut, err := h.Call(ctx, "proton_list_custom_domains", map[string]any{})
	if err != nil {
		t.Fatalf("list_custom_domains: %v", err)
	}
	domains, ok := listOut["domains"].([]any)
	if !ok {
		t.Fatalf("domains not a list: %#v", listOut["domains"])
	}
	if len(domains) == 0 {
		t.Skip("cassette has no custom domains; re-record after adding a domain to the test account")
	}
	first, ok := domains[0].(map[string]any)
	if !ok {
		t.Fatalf("domain[0] not an object: %#v", domains[0])
	}
	id, ok := first["id"].(string)
	if !ok || id == "" {
		t.Fatalf("domain[0].id missing or empty: %#v", first)
	}

	out, err := h.Call(ctx, "proton_get_custom_domain", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("get_custom_domain: %v", err)
	}
	if _, ok := out["domain"]; !ok {
		t.Fatalf("envelope missing %q", "domain")
	}
}

func TestAddCustomDomainHappy(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "add_remove_custom_domain")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_add_custom_domain",
		map[string]any{"domain_name": "example.test"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	domain, ok := out["domain"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing %q: %#v", "domain", out)
	}
	if _, ok := domain["id"]; !ok {
		t.Fatalf("domain.id missing: %#v", domain)
	}
}

func TestRemoveCustomDomainHappy(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "add_remove_custom_domain")
	defer h.Close()
	ctx := context.Background()
	addOut, err := h.Call(ctx, "proton_add_custom_domain",
		map[string]any{"domain_name": "example.test"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	domain, ok := addOut["domain"].(map[string]any)
	if !ok {
		t.Fatalf("add response missing domain: %#v", addOut)
	}
	id, _ := domain["id"].(string)
	if id == "" {
		t.Fatal("add returned no domain.id")
	}
	out, err := h.Call(ctx, "proton_remove_custom_domain", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %#v", out)
	}
}

func TestVerifyCustomDomainPending(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "verify_custom_domain_pending")
	defer h.Close()
	ctx := context.Background()

	listOut, err := h.Call(ctx, "proton_list_custom_domains", map[string]any{})
	if err != nil {
		t.Fatalf("list_custom_domains: %v", err)
	}
	domains, ok := listOut["domains"].([]any)
	if !ok {
		t.Fatalf("domains not a list: %#v", listOut["domains"])
	}
	if len(domains) == 0 {
		t.Skip("cassette has no custom domains; re-record with a pending domain on the account")
	}
	first, ok := domains[0].(map[string]any)
	if !ok {
		t.Fatalf("domain[0] not an object: %#v", domains[0])
	}
	id, _ := first["id"].(string)
	if id == "" {
		t.Fatal("domain[0].id missing")
	}

	out, err := h.Call(ctx, "proton_verify_custom_domain", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, ok := out["domain"]; !ok {
		t.Fatalf("envelope missing %q: %#v", "domain", out)
	}
}

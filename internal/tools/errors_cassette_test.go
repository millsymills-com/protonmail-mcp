package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

func TestErrorCaptcha(t *testing.T) {
	h := testharness.BootWithCassette(t, "error_captcha")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_whoami", map[string]any{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "proton/captcha") {
		t.Fatalf("want proton/captcha in error, got %q", err.Error())
	}
}

func TestErrorRateLimited(t *testing.T) {
	h := testharness.BootWithCassette(t, "error_rate_limited")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_whoami", map[string]any{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "proton/rate_limited") {
		t.Fatalf("want proton/rate_limited in error, got %q", err.Error())
	}
}

func TestErrorNotFoundMessage(t *testing.T) {
	h := testharness.BootWithCassette(t, "error_not_found_message")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_get_message", map[string]any{"id": "nonexistent"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "proton/not_found") {
		t.Fatalf("want proton/not_found in error, got %q", err.Error())
	}
}

func TestErrorNotFoundAddress(t *testing.T) {
	h := testharness.BootWithCassette(t, "error_not_found_address")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_get_address", map[string]any{"id": "nonexistent"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "proton/not_found") {
		t.Fatalf("want proton/not_found in error, got %q", err.Error())
	}
}

func TestErrorNotFoundDomain(t *testing.T) {
	h := testharness.BootWithCassette(t, "error_not_found_domain")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_get_custom_domain", map[string]any{
		"id": "nonexistent",
	})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "proton/not_found") {
		t.Fatalf("want proton/not_found in error, got %q", err.Error())
	}
}

func TestErrorPermissionDenied(t *testing.T) {
	h := testharness.BootWithCassette(t, "error_permission_denied")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_whoami", map[string]any{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "proton/permission_denied") {
		t.Fatalf("want proton/permission_denied in error, got %q", err.Error())
	}
}

func TestErrorConflictAddDomain(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "error_conflict_add_domain")
	defer h.Close()
	ctx := context.Background()
	_, err := h.Call(ctx, "proton_add_custom_domain",
		map[string]any{"domain_name": "example.test"})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	_, err = h.Call(ctx, "proton_add_custom_domain",
		map[string]any{"domain_name": "example.test"})
	if err == nil {
		t.Fatal("want error on second add, got nil")
	}
	if !strings.Contains(err.Error(), "proton/conflict") {
		t.Fatalf("want proton/conflict in error, got %q", err.Error())
	}
}

func TestErrorValidationCreateAddress(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "error_validation_create_address")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_create_address", map[string]any{
		"domain_id":  "REDACTED_DOMAINID_1",
		"local_part": "--bad",
	})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "proton/validation") {
		t.Fatalf("want proton/validation in error, got %q", err.Error())
	}
}

func TestErrorUpstream502(t *testing.T) {
	h := testharness.BootWithCassette(t, "error_upstream_502")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_whoami", map[string]any{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "proton/upstream") {
		t.Fatalf("want proton/upstream in error, got %q", err.Error())
	}
}

func TestErrorUpstream503(t *testing.T) {
	h := testharness.BootWithCassette(t, "error_upstream_503")
	defer h.Close()
	_, err := h.Call(context.Background(), "proton_whoami", map[string]any{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "proton/upstream") {
		t.Fatalf("want proton/upstream in error, got %q", err.Error())
	}
}

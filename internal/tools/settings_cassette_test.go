package tools_test

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

func TestGetMailSettingsHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "mail_settings_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_get_mail_settings", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["settings"]; !ok {
		t.Fatalf("envelope missing %q", "settings")
	}
}

func TestGetCoreSettingsHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "core_settings_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_get_core_settings", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["settings"]; !ok {
		t.Fatalf("envelope missing %q", "settings")
	}
}

func TestUpdateMailSettingsSignatureHappy(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "update_mail_settings_signature")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_update_mail_settings", map[string]any{
		"signature": "<p>Record test signature</p>",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["settings"]; !ok {
		t.Fatalf("envelope missing %q: %#v", "settings", out)
	}
}

func TestUpdateCoreSettingsTelemetryOff(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "update_core_settings_flags")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_update_core_settings", map[string]any{
		"telemetry": false,
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	settings, ok := out["settings"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing %q: %#v", "settings", out)
	}
	if telemetry, _ := settings["telemetry"].(float64); telemetry != 0 {
		t.Fatalf("expected telemetry=0, got %v", settings["telemetry"])
	}
}

func TestUpdateCoreSettingsFlagsOn(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "update_core_settings_flags")
	defer h.Close()
	ctx := context.Background()

	_, err := h.Call(ctx, "proton_update_core_settings", map[string]any{
		"telemetry": false,
	})
	if err != nil {
		t.Fatalf("disable telemetry: %v", err)
	}
	_, err = h.Call(ctx, "proton_update_core_settings", map[string]any{
		"crash_reports": false,
	})
	if err != nil {
		t.Fatalf("disable crash reports: %v", err)
	}

	out, err := h.Call(ctx, "proton_update_core_settings", map[string]any{
		"telemetry": true,
	})
	if err != nil {
		t.Fatalf("enable telemetry: %v", err)
	}
	settings, ok := out["settings"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing %q: %#v", "settings", out)
	}
	if telemetry, _ := settings["telemetry"].(float64); telemetry != 1 {
		t.Fatalf("expected telemetry=1, got %v", settings["telemetry"])
	}
}

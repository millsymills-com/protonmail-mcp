package tools_test

import (
	"context"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

func TestListCalendarsHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "list_calendars_happy")
	defer h.Close()

	out, err := h.Call(context.Background(), "proton_list_calendars", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	raw, ok := out["calendars"]
	if !ok {
		t.Fatalf("envelope missing 'calendars': %#v", out)
	}
	cals, ok := raw.([]any)
	if !ok {
		t.Fatalf("calendars is not a slice: %T", raw)
	}
	if len(cals) != 2 {
		t.Fatalf("expected 2 calendars, got %d", len(cals))
	}

	first, ok := cals[0].(map[string]any)
	if !ok {
		t.Fatalf("calendar[0] is not a map: %T", cals[0])
	}
	if first["id"] != "cal-personal-1" {
		t.Errorf("calendar[0] id: want %q, got %q", "cal-personal-1", first["id"])
	}
	if first["type"] != "normal" {
		t.Errorf("calendar[0] type: want %q, got %q", "normal", first["type"])
	}
	if active, _ := first["active"].(bool); !active {
		t.Errorf("calendar[0] active: want true, got %v", first["active"])
	}

	second, ok := cals[1].(map[string]any)
	if !ok {
		t.Fatalf("calendar[1] is not a map: %T", cals[1])
	}
	if second["type"] != "subscribed" {
		t.Errorf("calendar[1] type: want %q, got %q", "subscribed", second["type"])
	}
	if lostAccess, _ := second["lost_access"].(bool); !lostAccess {
		t.Errorf("calendar[1] lost_access: want true, got %v", second["lost_access"])
	}
}

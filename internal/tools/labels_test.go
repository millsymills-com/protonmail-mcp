package tools_test

import (
	"context"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

func TestListLabelsHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "list_labels_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_list_labels", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["labels"]; !ok {
		t.Fatalf("envelope missing %q: %#v", "labels", out)
	}
}

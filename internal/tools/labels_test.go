package tools_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

func TestListLabelsHappyCassette(t *testing.T) {
	if _, err := os.ReadFile("testdata/cassettes/list_labels_happy.yaml"); errors.Is(err, os.ErrNotExist) {
		t.Skipf("cassette not recorded yet; run: make record SCENARIO=list_labels_happy")
	}
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

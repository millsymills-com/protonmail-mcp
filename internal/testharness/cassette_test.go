package testharness_test

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

func TestBootWithCassetteSmokePings(t *testing.T) {
	h := testharness.BootWithCassette(t, "smoke")
	defer h.Close()
	if err := h.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

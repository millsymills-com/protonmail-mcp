package tools

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
)

func TestRequired_Empty(t *testing.T) {
	got := required("id", "")
	if got == nil {
		t.Fatal("want validation error for empty value")
	}
	if got.Code != "proton/validation" {
		t.Fatalf("want proton/validation code, got %q", got.Code)
	}
}

func TestRequired_Present(t *testing.T) {
	if got := required("id", "abc"); got != nil {
		t.Fatalf("want nil for non-empty value, got %+v", got)
	}
}

func TestFailure_NilInput(t *testing.T) {
	if got := failure(nil); got != nil {
		t.Fatalf("want nil for nil input, got %+v", got)
	}
}

func TestFailure_WrapsError(t *testing.T) {
	got := failure(&proterr.Error{Code: "proton/validation", Message: "boom"})
	if got == nil || !got.IsError {
		t.Fatalf("want IsError=true, got %+v", got)
	}
}

func TestClient_NilSession(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("want panic when session is nil")
		}
	}()
	_, _ = client(context.Background(), Deps{Session: nil})
}

func TestWritesEnabled(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"1", true},
		{"true", true},
		{"yes", true},
		{"YES", true},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", tc.value)
			if got := WritesEnabled(); got != tc.want {
				t.Errorf("v=%q want %v got %v", tc.value, tc.want, got)
			}
		})
	}
}

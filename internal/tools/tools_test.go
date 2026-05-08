package tools

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
)

func TestRequireField_Empty(t *testing.T) {
	got := requireField("id", "")
	if got == nil {
		t.Fatal("want validation failure for empty value")
	}
	if !got.IsError {
		t.Fatal("want IsError=true")
	}
	if len(got.Content) == 0 {
		t.Fatal("want non-empty Content")
	}
}

func TestRequireField_Present(t *testing.T) {
	if got := requireField("id", "abc"); got != nil {
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

func TestClientOrFail_NilSession(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("want panic when session is nil")
		}
	}()
	_, _ = clientOrFail(context.Background(), Deps{Session: nil})
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

package proterr_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"

	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
)

func TestScopeDenied(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"403-9100", &proton.APIError{Status: http.StatusForbidden, Code: 9100}, true},
		{"wrapped-403-9100", fmt.Errorf("get salts: %w", &proton.APIError{Status: http.StatusForbidden, Code: 9100}), true},
		{"value-403-9100", fmt.Errorf("get salts: %w", proton.APIError{Status: http.StatusForbidden, Code: 9100}), true},
		{"403-other-code", &proton.APIError{Status: http.StatusForbidden, Code: 2001}, false},
		{"401-9100", &proton.APIError{Status: http.StatusUnauthorized, Code: 9100}, false},
		{"not-api-error", fmt.Errorf("plain"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := proterr.ScopeDenied(tc.err); got != tc.want {
				t.Fatalf("ScopeDenied(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestMapKeyringUnlockScope proves a salts scope denial tagged with
// ErrKeyringUnlockScope maps to the actionable code — naming the scope cause
// and directing the operator to re-login with two-factor — and overrides the
// generic 403 mapping even though the 403 APIError is still in the chain.
func TestMapKeyringUnlockScope(t *testing.T) {
	err := fmt.Errorf("get salts: %w: %w",
		proterr.ErrKeyringUnlockScope,
		&proton.APIError{Status: http.StatusForbidden, Code: 9100})

	got := proterr.Map(err)
	if got == nil {
		t.Fatal("Map returned nil")
	}
	if got.Code != "proton/keyring_unlock_scope" {
		t.Fatalf("Code = %q, want proton/keyring_unlock_scope", got.Code)
	}
	if !strings.Contains(strings.ToLower(got.Message), "decryption") {
		t.Fatalf("message should name decryption impact, got %q", got.Message)
	}
	if !strings.Contains(got.Hint, "two-factor") || !strings.Contains(got.Hint, "login") {
		t.Fatalf("hint should direct re-login with two-factor, got %q", got.Hint)
	}
}

// TestMapPlain403StillPermissionDenied is the regression guard: a 403 NOT
// tagged as a keyring-unlock scope denial must still map to permission_denied.
func TestMapPlain403StillPermissionDenied(t *testing.T) {
	got := proterr.Map(&proton.APIError{Status: http.StatusForbidden, Code: 9100})
	if got == nil || got.Code != "proton/permission_denied" {
		t.Fatalf("untagged 403 = %+v, want proton/permission_denied", got)
	}
}

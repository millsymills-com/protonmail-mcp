package session

import (
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
)

func TestStatusKeyringUnlockFromScope(t *testing.T) {
	cases := []struct {
		name  string
		scope string
		want  string
	}{
		{"full", "full", "ok"},
		{"full-among-many", "self user full loggedin", "ok"},
		{"twofactor", "twofactor", "under_scoped"},
		{"empty-unknown", "", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keyring.MockInit()
			s, err := NewForTesting("http://invalid.test",
				keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r", Scope: tc.scope})
			if err != nil {
				t.Fatalf("NewForTesting: %v", err)
			}
			if got := s.Status().KeyringUnlock; got != tc.want {
				t.Fatalf("KeyringUnlock = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestOnAuthRotatedRecordsScope proves a rotation that carries a scope persists
// it and surfaces it through Status — the post-login full-scope signal.
func TestOnAuthRotatedRecordsScope(t *testing.T) {
	keyring.MockInit()
	s := newSessionWithStore(keychain.New())

	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r", Scope: "full"})

	if got := s.Status().KeyringUnlock; got != "ok" {
		t.Fatalf("KeyringUnlock = %q, want ok", got)
	}
	loaded, err := s.kc.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded.Scope != "full" {
		t.Fatalf("persisted Scope = %q, want full", loaded.Scope)
	}
}

// TestOnAuthRotatedPreservesScopeWhenOmitted proves a plain token refresh that
// omits scope (go-proton-api's auth handler does) does not regress an
// established full scope to unknown.
func TestOnAuthRotatedPreservesScopeWhenOmitted(t *testing.T) {
	keyring.MockInit()
	s, err := NewForTesting("http://invalid.test",
		keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r", Scope: "full"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}

	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "a2", RefreshToken: "r2"})

	if got := s.Status().KeyringUnlock; got != "ok" {
		t.Fatalf("KeyringUnlock = %q, want ok (scope must survive a scope-less rotation)", got)
	}
}

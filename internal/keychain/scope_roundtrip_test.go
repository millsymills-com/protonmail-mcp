package keychain_test

import (
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
)

// TestSessionScopeRoundTrips proves the token scope survives a SaveSession →
// LoadSession round-trip, so an under-scoped session stays detectable across
// process restarts rather than being dropped on persist.
func TestSessionScopeRoundTrips(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()

	want := keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r", Scope: "full"}
	if err := kc.SaveSession(want); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, err := kc.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got.Scope != "full" {
		t.Fatalf("Scope = %q, want full", got.Scope)
	}
}

// TestSessionScopeAbsentLoadsEmpty proves a session persisted without a scope
// (pre-scope-tracking, or a saved empty scope) loads as empty rather than
// erroring, keeping legacy sessions readable.
func TestSessionScopeAbsentLoadsEmpty(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()

	if err := kc.SaveSession(keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, err := kc.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got.Scope != "" {
		t.Fatalf("Scope = %q, want empty", got.Scope)
	}
}

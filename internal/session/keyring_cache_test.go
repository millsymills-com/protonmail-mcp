package session

import (
	"testing"

	gokeyring "github.com/zalando/go-keyring"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/keyring"
)

func TestKeyringCacheClearedByClearMethod(t *testing.T) {
	s := &Session{raw: newRawClient("http://invalid.test", nil)}
	s.keyrings = &keyring.Keyrings{} // pretend a prior unlock populated it
	s.clearKeyringCache()
	if s.keyrings != nil {
		t.Fatal("keyring cache must be nil after clearKeyringCache")
	}
}

func TestLogoutClearsKeyringCache(t *testing.T) {
	gokeyring.MockInit()
	s := &Session{kc: keychain.New(), raw: newRawClient("http://invalid.test", nil)}
	s.keyrings = &keyring.Keyrings{}
	if err := s.Logout(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if s.keyrings != nil {
		t.Fatal("Logout must clear the keyring cache")
	}
}

func TestKeyringsCacheHitSkipsClient(t *testing.T) {
	// When keyrings is already set, Keyrings(ctx) returns it without calling
	// Client (which would fail against "http://invalid.test").
	s := &Session{raw: newRawClient("http://invalid.test", nil)}
	want := &keyring.Keyrings{}
	s.keyrings = want

	got, err := s.Keyrings(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatal("Keyrings must return the cached value")
	}
}

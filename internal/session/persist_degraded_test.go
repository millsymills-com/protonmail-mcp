package session_test

import (
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/zalando/go-keyring"
)

func TestStatusZeroOnFreshSession(t *testing.T) {
	keyring.MockInit()
	s, err := session.NewForTesting("http://invalid.test",
		keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	defer func() { _ = s.Logout() }()

	got := s.Status()
	if got.PersistDegraded || got.PersistError != "" {
		t.Fatalf("fresh Status() = %+v, want zero", got)
	}
}

func TestSetPersistDegradedForTestRoundTrip(t *testing.T) {
	keyring.MockInit()
	s, err := session.NewForTesting("http://invalid.test",
		keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	defer func() { _ = s.Logout() }()

	s.SetPersistDegradedForTest("disk full")
	got := s.Status()
	if !got.PersistDegraded || got.PersistError != "disk full" {
		t.Fatalf("after set: Status() = %+v", got)
	}

	s.SetPersistDegradedForTest("")
	got = s.Status()
	if got.PersistDegraded || got.PersistError != "" {
		t.Fatalf("after clear: Status() = %+v", got)
	}
}

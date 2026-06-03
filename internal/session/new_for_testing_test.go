package session_test

import (
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
)

func TestNewForTestingSeedsKeychain(t *testing.T) {
	keyring.MockInit()
	seed := keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}
	s, err := session.NewForTesting("https://mail.proton.me/api", seed)
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	if s == nil {
		t.Fatal("NewForTesting returned nil session")
	}
	kc := keychain.New()
	got, err := kc.LoadSession()
	if err != nil {
		t.Fatalf("kc.LoadSession after seed: %v", err)
	}
	if got.UID != seed.UID || got.AccessToken != seed.AccessToken || got.RefreshToken != seed.RefreshToken {
		t.Fatalf("seeded session mismatch: %+v", got)
	}
}

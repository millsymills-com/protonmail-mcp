package keychain_test

import (
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/zalando/go-keyring"
)

func TestRoundTrip(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()

	creds := keychain.Creds{Username: "andy@example.com", Password: "hunter2", TOTPSecret: "JBSWY3DPEHPK3PXP"}
	if err := kc.SaveCreds(creds); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	got, err := kc.LoadCreds()
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if got != creds {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, creds)
	}

	sess := keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}
	if e := kc.SaveSession(sess); e != nil {
		t.Fatalf("SaveSession: %v", e)
	}
	gotS, err := kc.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if gotS != sess {
		t.Fatalf("session round trip mismatch: got %+v want %+v", gotS, sess)
	}

	if err := kc.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := kc.LoadCreds(); err == nil {
		t.Fatalf("LoadCreds after Clear should fail")
	}
}

func TestLoadCredsMissing(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	if _, err := kc.LoadCreds(); err == nil {
		t.Fatalf("expected error when keychain is empty")
	}
}

// TestSaveCredsTOTPLifecycle walks the same user through two sequential
// SaveCreds calls (with TOTP, then without) and asserts post-load state
// at each step. Verifies the one-shot-code path doesn't leave a stale
// TOTP behind.
func TestSaveCredsTOTPLifecycle(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()

	tests := []struct {
		name     string
		save     keychain.Creds
		wantTOTP string
	}{
		{"first login sets TOTP", keychain.Creds{Username: "u", Password: "p", TOTPSecret: "JBSWY3DPEHPK3PXP"}, "JBSWY3DPEHPK3PXP"},
		{"second login without TOTP clears it", keychain.Creds{Username: "u", Password: "p"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := kc.SaveCreds(tc.save); err != nil {
				t.Fatalf("SaveCreds: %v", err)
			}
			got, err := kc.LoadCreds()
			if err != nil {
				t.Fatalf("LoadCreds: %v", err)
			}
			if got.TOTPSecret != tc.wantTOTP {
				t.Fatalf("TOTPSecret: got %q want %q", got.TOTPSecret, tc.wantTOTP)
			}
		})
	}
}

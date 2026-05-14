package keychain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/zalando/go-keyring"
)

func TestRoundTrip(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()

	creds := keychain.Creds{
		Username: "andy@example.com", Password: "hunter2", TOTPSecret: "JBSWY3DPEHPK3PXP",
	}
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

func TestSaveCredsError(t *testing.T) {
	keyring.MockInitWithError(errors.New("backend unavailable"))
	kc := keychain.New()
	err := kc.SaveCreds(keychain.Creds{Username: "u", Password: "p"})
	if err == nil || !strings.Contains(err.Error(), "save username") {
		t.Fatalf("want save-username error, got %v", err)
	}
}

func TestSaveSessionError(t *testing.T) {
	keyring.MockInitWithError(errors.New("backend unavailable"))
	kc := keychain.New()
	err := kc.SaveSession(keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"})
	if err == nil || !strings.Contains(err.Error(), "save uid") {
		t.Fatalf("want save-uid error, got %v", err)
	}
}

func TestLoadSessionError(t *testing.T) {
	keyring.MockInitWithError(errors.New("backend unavailable"))
	kc := keychain.New()
	_, err := kc.LoadSession()
	if err == nil || !strings.Contains(err.Error(), "load uid") {
		t.Fatalf("want load-uid error, got %v", err)
	}
}

func TestClearError(t *testing.T) {
	keyring.MockInitWithError(errors.New("backend unavailable"))
	kc := keychain.New()
	err := kc.Clear()
	if err == nil || !strings.Contains(err.Error(), "delete") {
		t.Fatalf("want delete error, got %v", err)
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
		{
			"first login sets TOTP",
			keychain.Creds{Username: "u", Password: "p", TOTPSecret: "JBSWY3DPEHPK3PXP"},
			"JBSWY3DPEHPK3PXP",
		},
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

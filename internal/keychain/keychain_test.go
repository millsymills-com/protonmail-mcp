package keychain_test

import (
	"testing"

	"github.com/zalando/go-keyring"
	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
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

func TestSaveCredsClearsStaleTOTP(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()

	// First login: TOTP is set.
	if err := kc.SaveCreds(keychain.Creds{Username: "u", Password: "p", TOTPSecret: "JBSWY3DPEHPK3PXP"}); err != nil {
		t.Fatalf("first SaveCreds: %v", err)
	}
	got, err := kc.LoadCreds()
	if err != nil || got.TOTPSecret != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("first LoadCreds: got=%+v err=%v", got, err)
	}

	// Second login: same user, TOTP NOT supplied (one-shot code path).
	if e := kc.SaveCreds(keychain.Creds{Username: "u", Password: "p"}); e != nil {
		t.Fatalf("second SaveCreds: %v", e)
	}
	got, err = kc.LoadCreds()
	if err != nil {
		t.Fatalf("second LoadCreds: %v", err)
	}
	if got.TOTPSecret != "" {
		t.Fatalf("stale TOTP survived second login: got=%q", got.TOTPSecret)
	}
}

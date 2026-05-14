package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
	"github.com/zalando/go-keyring"
)

func TestStatusLoggedOut(t *testing.T) {
	keyring.MockInit()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"status"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		strings.NewReader(""),
		stdout,
		stderr,
		nil,
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "not logged in") {
		t.Fatalf("stdout = %q, want 'not logged in'", stdout.String())
	}
}

func TestLogoutClearsKeychain(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	seed := keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}
	if err := kc.SaveSession(seed); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"logout"},
		nil,
		strings.NewReader(""),
		stdout,
		stderr,
		nil,
	)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if _, err := kc.LoadSession(); err == nil {
		t.Fatal("session still present after logout")
	}
}

func TestStatusLoggedInUsesCassette(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	seed := keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}
	if err := kc.SaveSession(seed); err != nil {
		t.Fatal(err)
	}
	rt := testvcr.New(t, "status_logged_in")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"status"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		strings.NewReader(""),
		stdout,
		stderr,
		rt,
	)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "user@example.test") {
		t.Fatalf("stdout missing email: %q", stdout.String())
	}
}

func TestLoginNo2FA(t *testing.T) {
	keyring.MockInit()
	rt := testvcr.New(t, "login_no_2fa")
	stdin := strings.NewReader("user@example.test\nhunter2\n")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"login"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		stdin,
		stdout,
		stderr,
		rt,
	)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	kc := keychain.New()
	if _, err := kc.LoadSession(); err != nil {
		t.Fatalf("session not persisted: %v", err)
	}
}

func TestLoginWith2FA(t *testing.T) {
	keyring.MockInit()
	rt := testvcr.New(t, "login_with_2fa")
	// The actual TOTP value typed by the user doesn't matter — the matcher
	// ignores TwoFactorCode value differences.
	stdin := strings.NewReader("user@example.test\nhunter2\n123456\n")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"login"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		stdin,
		stdout,
		stderr,
		rt,
	)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	kc := keychain.New()
	if _, err := kc.LoadSession(); err != nil {
		t.Fatalf("session not persisted: %v", err)
	}
}

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testvcr"
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

func TestStatusReportsPersistDegraded(t *testing.T) {
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

	preboot := func(s *session.Session) {
		s.SetPersistDegradedForTest("save session: keychain locked")
	}

	code := runWithSessionHook(
		context.Background(),
		[]string{"status"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		strings.NewReader(""),
		stdout,
		stderr,
		rt,
		preboot,
	)
	// Degraded persistence exits 3 (output still printed) so headless monitors
	// can detect the fault from $? without parsing stdout.
	if code != 3 {
		t.Fatalf("exit = %d, want 3; stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "user@example.test") {
		t.Fatalf("stdout missing email: %q", out)
	}
	if !strings.Contains(out, "warning: token persistence degraded") {
		t.Fatalf("stdout missing warning: %q", out)
	}
	if !strings.Contains(out, "save session: keychain locked") {
		t.Fatalf("stdout missing persist error reason: %q", out)
	}
}

// TestLoginWith2FAOtpauthURI covers the runLogin branch that accepts an
// otpauth:// URI after the 2FA prompt (preferred path — enables silent
// refresh). The cassette is shared with TestLoginWith2FA; the matcher
// ignores TwoFactorCode so the URI-derived code matches the recorded body.
func TestLoginWith2FAOtpauthURI(t *testing.T) {
	keyring.MockInit()
	t.Setenv("PROTONMAIL_MCP_TEST_SKIP_PROOFS", "1")
	rt := testvcr.New(t, "login_with_2fa")
	// RFC 6238 reference secret "12345678901234567890".
	stdin := strings.NewReader("user@example.test\nhunter2\n" +
		"otpauth://totp/Proton:user?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ&issuer=Proton\n")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"login"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		stdin, stdout, stderr, rt)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
}

// TestLoginWith2FAInvalidInput covers the validation branch that rejects
// 2FA input that isn't an otpauth:// URI and isn't a 6-digit code.
func TestLoginWith2FAInvalidInput(t *testing.T) {
	keyring.MockInit()
	t.Setenv("PROTONMAIL_MCP_TEST_SKIP_PROOFS", "1")
	rt := testvcr.New(t, "login_with_2fa")
	stdin := strings.NewReader("user@example.test\nhunter2\nnotacode\n")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"login"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		stdin, stdout, stderr, rt)
	if code == 0 {
		t.Fatalf("expected non-zero exit for invalid 2FA input")
	}
	if !strings.Contains(stderr.String(), "2FA input invalid") {
		t.Fatalf("stderr should mention 2FA invalid: %s", stderr.String())
	}
}

// TestLoginWith2FAEOFOnPrompt covers the prompt-EOF branch after 2FA is
// required — runLogin returns the promptReader error verbatim.
func TestLoginWith2FAEOFOnPrompt(t *testing.T) {
	keyring.MockInit()
	t.Setenv("PROTONMAIL_MCP_TEST_SKIP_PROOFS", "1")
	rt := testvcr.New(t, "login_with_2fa")
	stdin := strings.NewReader("user@example.test\nhunter2\n") // no 2FA line; EOF
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"login"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		stdin, stdout, stderr, rt)
	if code == 0 {
		t.Fatalf("expected non-zero exit for EOF on 2FA prompt")
	}
}

// TestLoginWith2FA covers the runLogin 2FA-prompt branch by replaying a
// cassette captured against the fake-Proton fixture in TwoFA mode (see
// cmd/record-cassettes/scenarios/srp_fixture.go). Stdin lines: email,
// password, then the 6-digit fixture TOTP code.
func TestLoginWith2FA(t *testing.T) {
	keyring.MockInit()
	t.Setenv("PROTONMAIL_MCP_TEST_SKIP_PROOFS", "1")
	rt := testvcr.New(t, "login_with_2fa")
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

func TestLoginNo2FA(t *testing.T) {
	keyring.MockInit()
	// The cassette was recorded against a fake Proton SRP server (see
	// cmd/record-cassettes/scenarios/srp_fixture.go). Replay can't reproduce
	// the exact ServerProof because the client's ephemeral is fresh on every
	// run, so the test asks session.New to disable SRP ServerProof
	// verification via PROTONMAIL_MCP_TEST_SKIP_PROOFS. Recording is
	// deterministic; only replay needs this hook.
	t.Setenv("PROTONMAIL_MCP_TEST_SKIP_PROOFS", "1")
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

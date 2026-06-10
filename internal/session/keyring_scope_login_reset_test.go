package session

import (
	"context"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/testvcr"
)

// TestLoginResetsScopeReloginLatch proves a successful production login clears
// the scope self-heal latch (the reset in loginLocked's success tail): an
// operator whose session latched on an unfixable 9100 recovers a healable
// session after re-authenticating. Dropping that reset would strand the latch
// until process restart with no failing test. This drives loginLocked to
// success via the offline relogin cassette (no seam), so it guards the
// production reset, not a stubbed one.
func TestLoginResetsScopeReloginLatch(t *testing.T) {
	keyring.MockInit()
	t.Setenv("PROTONMAIL_MCP_TEST_SKIP_PROOFS", "1")
	kc := keychain.New()
	if err := kc.SaveCreds(keychain.Creds{
		Username: "user@example.test", Password: "hunter2", TOTPSecret: "JBSWY3DPEHPK3PXP",
	}); err != nil {
		t.Fatal(err)
	}
	// Scrubbed token placeholders so the replayed cold-start refresh body matches
	// the recorded /auth/v4/refresh interaction (see the relogin scenario).
	if err := kc.SaveSession(keychain.Session{
		UID: "REDACTED_UID_1", AccessToken: "REDACTED_ACCESSTOKEN_1", RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}); err != nil {
		t.Fatal(err)
	}

	s := New("https://mail.proton.me/api", kc, WithTransport(testvcr.New(t, "relogin_after_refresh_reject")))
	s.mu.Lock()
	s.scopeReloginExhausted = true
	s.mu.Unlock()

	if _, err := s.Client(context.Background()); err != nil {
		t.Fatalf("expected the self-heal relogin to succeed: %v", err)
	}
	if s.scopeReloginSpent() {
		t.Fatal("a successful login must reset the scope self-heal latch")
	}
}

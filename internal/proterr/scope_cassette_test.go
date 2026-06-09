package proterr_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/zalando/go-keyring"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testvcr"
)

// TestScopeDeniedFromCassette anchors scopeDeniedCode (403/9100) to a real wire
// response instead of the hand-written constant: it replays the under-scoped
// salts denial recorded by cmd/record-cassettes/scenarios/salts_underscoped_denied.go.
//
// The cassette captures the raw pre-2FA login (NewClientWithLogin completes SRP
// but stops before 2FA, leaving an under-scoped token) followed by GetSalts. We
// replay the same flow: skip-proofs because a replayed SRP exchange can never
// reproduce the recording client's ServerProof, and log in as user@example.test
// because the scrubber rewrites RECORD_EMAIL to that address in the recorded
// /auth/v4/info body (password is irrelevant — SRP proofs are wildcarded by the
// matcher and the ServerProof check is disabled).
//
// Skips until the cassette is recorded (see testvcr.New), per the repo
// convention of landing the consuming test before the live recording phase.
func TestScopeDeniedFromCassette(t *testing.T) {
	keyring.MockInit()
	t.Setenv("PROTONMAIL_MCP_TEST_SKIP_PROOFS", "1")

	rt := testvcr.New(t, "salts_underscoped_denied")
	sess := session.New("https://mail.proton.me/api", keychain.New(), session.WithTransport(rt))
	mgr := sess.ManagerForTest()

	c, auth, err := mgr.NewClientWithLogin(context.Background(), "user@example.test", []byte("hunter2"))
	if err != nil {
		t.Fatalf("pre-2fa login replay: %v", err)
	}
	defer c.Close()

	if auth.TwoFA.Enabled == 0 {
		t.Fatal("recorded auth shows no 2FA enabled; the login token is full-scope, so the cassette does not capture an under-scoped salts denial")
	}

	_, err = c.GetSalts(context.Background())
	if err == nil {
		t.Fatal("expected a scope denial from GET keys/salts, got success")
	}

	var apiErr *proton.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("salts error is not *proton.APIError: %v", err)
	}
	if apiErr.Status != http.StatusForbidden || apiErr.Code != 9100 {
		t.Fatalf("recorded salts denial = %d/%d, want 403/9100 — scopeDeniedCode is no longer anchored to the wire", apiErr.Status, apiErr.Code)
	}
	if !proterr.ScopeDenied(err) {
		t.Fatal("proterr.ScopeDenied should classify the recorded 403/9100 salts denial")
	}
}

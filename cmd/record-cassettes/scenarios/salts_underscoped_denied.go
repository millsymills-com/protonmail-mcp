//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	proton "github.com/ProtonMail/go-proton-api"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testvcr"
)

func registerSaltsUnderscopedDenied() {
	Register("salts_underscoped_denied", recordSaltsUnderscopedDenied)
}

// recordSaltsUnderscopedDenied captures a real GET /core/v4/keys/salts denial
// against an under-scoped (pre-2FA) session token, anchoring proterr's
// scopeDeniedCode (403/9100) to a recorded wire response instead of a comment.
//
// It drives the raw go-proton-api login flow directly rather than
// session.Login: NewClientWithLogin completes SRP but stops before 2FA, minting
// the under-scoped token whose scope lacks "full". Calling GetSalts on that
// client reproduces the exact 403/9100 the keyring-unlock path classifies. We
// never call Auth2FA, so no full-scope token is ever minted.
//
// Requires a real account with 2FA (TOTP) enabled — RECORD_EMAIL / RECORD_PASSWORD.
// RECORD_TOTP_SECRET is intentionally unused: completing 2FA would widen the
// scope and the salts call would succeed, defeating the recording.
func recordSaltsUnderscopedDenied(ctx context.Context) (retErr error) {
	target := filepath.Join("internal", "proterr", "testdata", "cassettes", "salts_underscoped_denied")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	email, password, err := testvcr.RecordCredentials()
	if err != nil {
		return err
	}

	rt, stop, err := testvcr.NewAtPath(target, testvcr.ModeRecord)
	if err != nil {
		return fmt.Errorf("open recorder: %w", err)
	}
	// Registered before the stop() defer so it runs after stop() flushes: if the
	// scenario fails its precondition (account has no 2FA) or salts unexpectedly
	// succeeds, the captured login is not a valid scope-denial fixture. Discard
	// it rather than leave a partial cassette that flips the replay test from a
	// clean skip to a failure.
	defer func() {
		if retErr != nil {
			_ = os.Remove(target + ".yaml")
			_ = os.Remove(target + ".yaml.meta.yaml")
		}
	}()
	defer func() {
		if closeErr := stop(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	// The session exists only to build a proton.Manager wired with the recording
	// transport and the correct app-version header; we use its raw Manager, not
	// session.Login, to keep the token pre-2FA.
	sess := session.New(defaultAPIURL(), keychain.New(), session.WithTransport(rt))
	mgr := sess.ManagerForTest()

	c, auth, err := mgr.NewClientWithLogin(ctx, email, []byte(password))
	if err != nil {
		return fmt.Errorf("pre-2fa login: %w", err)
	}
	defer c.Close()

	if auth.TwoFA.Enabled == 0 {
		return fmt.Errorf("RECORD_EMAIL account has no 2FA enabled; its login token is already full-scope, so salts would succeed and cannot capture a scope denial")
	}

	if _, err := c.GetSalts(ctx); err == nil {
		return fmt.Errorf("expected 403/%d scope denial from GET keys/salts, got success — token was not under-scoped", proton.Code(9100))
	}
	// A non-nil error is the recording target: the 403/9100 interaction is now
	// captured in the cassette. Return nil so the recorder commits it.
	return nil
}

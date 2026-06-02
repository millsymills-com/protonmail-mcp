//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func registerReloginAfterRefreshReject() {
	Register("relogin_after_refresh_reject", recordReloginAfterRefreshReject)
}

// recordReloginAfterRefreshReject records the unattended self-heal path: a
// rejected cold-start refresh followed by a successful SRP login + 2FA from the
// stored credentials. Requires real Proton access (RECORD_EMAIL/PASSWORD and a
// RECORD_TOTP_SECRET, since self-heal needs a TOTP secret, not a one-shot code)
// and runs through the same manual recorder gate as the other auth scenarios.
func recordReloginAfterRefreshReject(ctx context.Context) (retErr error) {
	target := filepath.Join("internal", "session", "testdata", "cassettes", "relogin_after_refresh_reject")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	// One-shot reject the cold-start /auth/v4/refresh so the self-heal relogin
	// fires; the synthetic 422 sits inside the recorder (via WithRealTransport)
	// so it lands in the cassette, and the subsequent SRP login + 2FA reach the
	// real backend and get recorded too. The target must be the full versioned
	// path: strings.Contains("/api/auth/v4/refresh", "/auth/refresh") is false,
	// so a "/auth/refresh" target silently never fires and the cold-start
	// refresh reaches real Proton and succeeds — yielding a cassette that omits
	// the reject and relogin entirely.
	injected := inject422RefreshRevoked(http.DefaultTransport, "/auth/v4/refresh")
	rt, stop, err := testvcr.NewAtPath(target, testvcr.ModeRecord, testvcr.WithRealTransport(injected))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := stop(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	// Bootstrap a real login (not through the recorder) so the keychain holds
	// the creds + TOTP secret the relogin will reuse.
	kc := keychain.New()
	if _, err := freshLoginForScenario(ctx, kc); err != nil {
		return err
	}

	// Cold-start a fresh Session over the recorder. NewClientWithRefresh hits
	// the injected 422; reloginLocked then re-logs in from the stored creds.
	driver := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	if _, err := driver.Client(ctx); err != nil {
		return fmt.Errorf("expected self-heal relogin to succeed: %w", err)
	}
	return nil
}

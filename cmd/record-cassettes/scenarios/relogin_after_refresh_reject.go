//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testvcr"
)

func registerReloginAfterRefreshReject() {
	Register("relogin_after_refresh_reject", recordReloginAfterRefreshReject)
}

// recordReloginAfterRefreshReject records the unattended self-heal path: a
// rejected cold-start refresh followed by a successful SRP login + 2FA from the
// stored credentials. It runs entirely offline against the in-process fake
// Proton auth server (srp_fixture.go) — no real account is touched. A live
// recording cannot drive this test anyway: SRP server proofs are computed for
// the recording client's random ephemeral and never match a replay's, so the
// consumer must disable proof verification (PROTONMAIL_MCP_TEST_SKIP_PROOFS),
// exactly as the login_with_2fa fixture path already does.
func recordReloginAfterRefreshReject(ctx context.Context) (retErr error) {
	target := filepath.Join("internal", "session", "testdata", "cassettes", "relogin_after_refresh_reject")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	fake, err := newFakeProtonAuthServerReloginTwoFA()
	if err != nil {
		return fmt.Errorf("start fake proton server: %w", err)
	}
	defer fake.Close()

	// One-shot reject the cold-start /auth/v4/refresh so the self-heal relogin
	// fires; every other request routes through to the fake auth server. The
	// target must be the full versioned path — strings.Contains matches by
	// substring and "/auth/refresh" never matches "/api/auth/v4/refresh".
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

	// Seed the keychain the cold start reads from: a session (so the cold-start
	// refresh has a token to present) plus the fixture creds the self-heal
	// relogin reuses. Cleared afterwards in case the recorder ran against a
	// real keychain (built without mockkc).
	kc := keychain.New()
	defer func() {
		if clearErr := kc.Clear(); clearErr != nil && retErr == nil {
			retErr = fmt.Errorf("clear fixture keychain: %w", clearErr)
		}
	}()
	if err := kc.SaveCreds(keychain.Creds{
		Username:   loginFixtureEmail,
		Password:   loginFixturePassword,
		TOTPSecret: loginFixtureTOTPSecret,
	}); err != nil {
		return err
	}
	if err := kc.SaveSession(keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}); err != nil {
		return err
	}

	driver := session.New(fake.URL+"/api", kc,
		session.WithTransport(rt),
		session.WithSkipProofVerificationForRecording(),
	)
	if _, err := driver.Client(ctx); err != nil {
		return fmt.Errorf("expected self-heal relogin to succeed: %w", err)
	}
	return nil
}

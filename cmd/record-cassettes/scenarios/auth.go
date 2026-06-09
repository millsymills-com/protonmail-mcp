//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"os"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testvcr"
)

func defaultAPIURL() string {
	if v := os.Getenv("PROTONMAIL_MCP_API_URL"); v != "" {
		return v
	}
	return "https://mail.proton.me/api"
}

// Cross-scenario session cache. When the recorder runs multiple scenarios in
// one process (batch mode), reuse the first login instead of paying SRP for
// every scenario — that pattern trips Proton's anti-abuse threshold after
// ~10 logins/minute and locks the account for ~30 min with a 422 code 8002
// even when the password is correct. Cache survives until FinalLogout fires.
var (
	cachedSess *session.Session
	cachedKc   *keychain.Keychain
)

func loginAndPersistSession(ctx context.Context, kc *keychain.Keychain) (*session.Session, error) {
	if cachedSess != nil {
		return cachedSess, nil
	}
	email, password, err := testvcr.RecordCredentials()
	if err != nil {
		return nil, err
	}
	in := session.LoginInput{
		Username:   email,
		Password:   password,
		TOTPSecret: os.Getenv("RECORD_TOTP_SECRET"),
	}
	sess := session.New(defaultAPIURL(), kc)
	if err := sess.Login(ctx, in); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	cachedSess = sess
	cachedKc = kc
	return sess, nil
}

// FinalLogout tears down the cached session at end of a batch run.
// Safe to call when no session was ever cached. This is the single
// teardown entry point — scenarios used to defer a per-scenario
// logoutAndClear helper, but the in-process session cache made that
// path a no-op and FinalLogout the only thing that actually runs.
func FinalLogout() {
	if cachedSess == nil {
		return
	}
	_ = cachedSess.Logout()
	if cachedKc != nil {
		_ = cachedKc.Clear()
	}
	cachedSess = nil
	cachedKc = nil
}

// freshLoginForScenario clears any cached session and performs a fresh SRP
// login. Use this for scenarios where each must hold a refresh token Proton
// has not yet consumed — specifically the injector-based error scenarios,
// whose cassettes capture a /auth/v4/refresh interaction that consumes the
// previous batch member's refresh token. Reusing the cached session between
// such scenarios yields 400 Invalid refresh token on the second one and
// onward.
//
// Cost: each call pays ~1 SRP round-trip and counts against Proton's
// anti-abuse threshold (~10 logins/min before 422 Code=8002). Caller is
// responsible for spacing batches.
func freshLoginForScenario(ctx context.Context, kc *keychain.Keychain) (*session.Session, error) {
	if cachedSess != nil {
		_ = cachedSess.Logout()
		if cachedKc != nil {
			_ = cachedKc.Clear()
		}
		cachedSess = nil
		cachedKc = nil
	}
	return loginAndPersistSession(ctx, kc)
}

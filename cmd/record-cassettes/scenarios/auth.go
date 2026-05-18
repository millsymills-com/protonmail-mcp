//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"os"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
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
	email := os.Getenv("RECORD_EMAIL")
	password := os.Getenv("RECORD_PASSWORD")
	if email == "" || password == "" {
		return nil, fmt.Errorf("RECORD_EMAIL or RECORD_PASSWORD unset")
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

// logoutAndClear is a no-op while a cached session is live; FinalLogout is
// the single end-of-batch entry point that actually tears down the session.
func logoutAndClear(sess *session.Session, kc *keychain.Keychain) {
	if cachedSess != nil {
		return
	}
	_ = sess.Logout()
	_ = kc.Clear()
}

// FinalLogout tears down the cached session at the end of a batch run.
// Safe to call when no session was ever cached.
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

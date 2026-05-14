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

func loginAndPersistSession(ctx context.Context, kc *keychain.Keychain) (*session.Session, error) {
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
	return sess, nil
}

func logoutAndClear(sess *session.Session, kc *keychain.Keychain) {
	_ = sess.Logout()
	_ = kc.Clear()
}

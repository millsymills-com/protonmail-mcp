//go:build recording

package scenarios

import (
	"context"
	"os"
	"path/filepath"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func registerLogoutInvalidates() {
	Register("logout_invalidates", recordLogoutInvalidates)
}

func recordLogoutInvalidates(ctx context.Context) (retErr error) {
	target := filepath.Join("internal", "session", "testdata", "cassettes", "logout_invalidates")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rt, stop, err := testvcr.NewAtPath(target, testvcr.ModeRecord)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := stop(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	kc := keychain.New()
	in := session.LoginInput{
		Username:   os.Getenv("RECORD_EMAIL"),
		Password:   os.Getenv("RECORD_PASSWORD"),
		TOTPSecret: os.Getenv("RECORD_TOTP_SECRET"),
	}
	sess := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	if loginErr := sess.Login(ctx, in); loginErr != nil {
		return loginErr
	}
	c, err := sess.Client(ctx)
	if err != nil {
		return err
	}
	if _, err := c.GetUser(ctx); err != nil {
		return err
	}
	if err := sess.Logout(); err != nil {
		return err
	}
	// Post-logout call: Proton returns 401; the response is captured for replay.
	_, _ = c.GetUser(ctx)
	return nil
}

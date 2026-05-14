//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

const cliCassetteDir = "cmd/protonmail-mcp/testdata/cassettes"

func init() {
	Register("status_logged_in", recordStatusLoggedIn)
	Register("login_no_2fa", func(ctx context.Context) error {
		return recordLogin(ctx, "login_no_2fa", cliCassetteDir, false)
	})
	Register("login_with_2fa", func(ctx context.Context) error {
		return recordLogin(ctx, "login_with_2fa", cliCassetteDir, true)
	})
}

func recordStatusLoggedIn(ctx context.Context) (retErr error) {
	target := filepath.Join(cliCassetteDir, "status_logged_in")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rt, stop, err := testvcr.NewAtPath(target, testvcr.ModeRecord)
	if err != nil {
		return err
	}
	defer func() {
		if err := stop(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	kc := keychain.New()
	plainSess, loginErr := loginAndPersistSession(ctx, kc)
	if loginErr != nil {
		return loginErr
	}
	defer logoutAndClear(plainSess, kc)

	recSess := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	c, err := recSess.Client(ctx)
	if err != nil {
		return fmt.Errorf("get client: %w", err)
	}
	_, err = c.GetUser(ctx)
	return err
}

func recordLogin(ctx context.Context, scenario, cassetteDir string, twoFA bool) (retErr error) {
	target := filepath.Join(cassetteDir, scenario)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rt, stop, err := testvcr.NewAtPath(target, testvcr.ModeRecord)
	if err != nil {
		return err
	}
	defer func() {
		if err := stop(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	email := os.Getenv("RECORD_EMAIL")
	password := os.Getenv("RECORD_PASSWORD")
	totpSecret := os.Getenv("RECORD_TOTP_SECRET")
	if email == "" || password == "" {
		return fmt.Errorf("RECORD_EMAIL or RECORD_PASSWORD unset")
	}
	if twoFA && totpSecret == "" {
		return fmt.Errorf("login_with_2fa: RECORD_TOTP_SECRET unset")
	}
	if !twoFA && totpSecret != "" {
		return fmt.Errorf(
			"login_no_2fa: RECORD_TOTP_SECRET is set but this scenario expects 2FA OFF; " +
				"either clear the env var temporarily or use login_with_2fa instead",
		)
	}

	kc := keychain.New()
	sess := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	in := session.LoginInput{
		Username:   email,
		Password:   password,
		TOTPSecret: totpSecret,
	}
	if err := sess.Login(ctx, in); err != nil {
		return err
	}
	return sess.Logout()
}

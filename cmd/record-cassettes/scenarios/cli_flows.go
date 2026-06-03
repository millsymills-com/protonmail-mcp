//go:build recording

package scenarios

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testvcr"
)

const cliCassetteDir = "cmd/protonmail-mcp/testdata/cassettes"

func registerCLIFlows() {
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
		if closeErr := stop(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	kc := keychain.New()
	if _, loginErr := loginAndPersistSession(ctx, kc); loginErr != nil {
		return loginErr
	}

	recSess := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	c, err := recSess.Client(ctx)
	if err != nil {
		return fmt.Errorf("get client: %w", err)
	}
	_, err = c.GetUser(ctx)
	return err
}

// recordLogin records an SRP exchange against an in-process fake Proton
// auth server (cmd/record-cassettes/scenarios/srp_fixture.go). The real
// account is never touched: the fake server precomputes an SRP verifier
// for the fixture password ("hunter2") and runs the server side of the
// challenge via go-srp, so the proton library's client-side math succeeds
// and the cassette captures a complete, deterministic /auth/v4/info +
// /auth/v4 [+ /auth/v4/2fa] exchange that the consumer test can replay.
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
		if closeErr := stop(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	var fakeStart func() (*httptest.Server, error)
	if twoFA {
		fakeStart = newFakeProtonAuthServerTwoFA
	} else {
		fakeStart = newFakeProtonAuthServer
	}
	fake, err := fakeStart()
	if err != nil {
		return fmt.Errorf("start fake Proton server: %w", err)
	}
	defer fake.Close()

	kc := keychain.New()
	sess := session.New(fake.URL+"/api", kc,
		session.WithTransport(rt),
		session.WithSkipProofVerificationForRecording(),
	)
	// Always tear down the keychain after recording. The recorder may run
	// against the host's real macOS keychain if built without `mockkc`, and
	// without this Clear() the operator's keychain ends up holding the
	// fixture creds (user@example.test / hunter2 / REDACTED_* tokens) until
	// the next real `protonmail-mcp logout`.
	defer func() {
		if clearErr := kc.Clear(); clearErr != nil && retErr == nil {
			retErr = fmt.Errorf("clear fixture keychain: %w", clearErr)
		}
	}()
	in := session.LoginInput{
		Username: loginFixtureEmail,
		Password: loginFixturePassword,
	}
	if twoFA {
		// runLogin (the consumer-side CLI) calls sess.Login twice for the
		// 2FA path: once without the TOTP code (yields ErrTOTPRequired),
		// then again with the code after prompting the user. Each Login
		// runs a fresh SRP exchange, so the cassette must contain the
		// full info+v4 pair twice + the final /auth/v4/2fa request.
		// Mirror that pattern here.
		if loginErr := sess.Login(ctx, in); loginErr != nil &&
			!errors.Is(loginErr, session.ErrTOTPRequired) {
			return fmt.Errorf("priming login: %w", loginErr)
		}
		in.TOTPCode = loginFixture2FACode
	}
	return sess.Login(ctx, in)
}

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

func init() {
	Register("token_rotation", recordTokenRotation)
}

func recordTokenRotation(ctx context.Context) (retErr error) {
	target := filepath.Join("internal", "session", "testdata", "cassettes", "token_rotation")
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
	plainSess, err := loginAndPersistSession(ctx, kc)
	if err != nil {
		return err
	}
	defer logoutAndClear(plainSess, kc)

	// Inject a one-shot 401 on /core/v4/users so the session refresh-on-401
	// path fires, then the retry and refresh exchange are captured by the cassette.
	wrapped := inject401AccessTokenExpired(rt, "/core/v4/users")
	sess := session.New(defaultAPIURL(), kc, session.WithTransport(wrapped))
	c, err := sess.Client(ctx)
	if err != nil {
		return err
	}
	_, err = c.GetUser(ctx)
	return err
}

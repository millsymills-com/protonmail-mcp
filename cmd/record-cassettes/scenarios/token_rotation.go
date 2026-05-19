//go:build recording

package scenarios

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func registerTokenRotation() {
	Register("token_rotation", recordTokenRotation)
}

func recordTokenRotation(ctx context.Context) (retErr error) {
	target := filepath.Join("internal", "session", "testdata", "cassettes", "token_rotation")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	// Inject a one-shot 401 on /core/v4/users so the session refresh-on-401
	// path fires; the synthetic 401 and the subsequent real refresh + retry
	// are captured by the cassette via the recorder's normal record path.
	injected := inject401AccessTokenExpired(http.DefaultTransport, "/core/v4/users")
	rt, stop, err := testvcr.NewAtPath(target, testvcr.ModeRecord, testvcr.WithRealTransport(injected))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := stop(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	kc := keychain.New()
	_, err = loginAndPersistSession(ctx, kc)
	if err != nil {
		return err
	}

	sess := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	c, err := sess.Client(ctx)
	if err != nil {
		return err
	}
	_, err = c.GetUser(ctx)
	return err
}

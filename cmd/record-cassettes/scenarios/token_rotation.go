//go:build recording

package scenarios

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testvcr"
)

func registerTokenRotation() {
	Register("token_rotation", recordTokenRotation)
}

// recordTokenRotation captures the four-interaction sequence the consumer
// test (TestTokenRotationOnExpiredAccess) expects:
//
//  1. POST /auth/v4/refresh  (cold-start via sess.Client)
//  2. GET  /core/v4/users    (synthetic 401 from one-shot injector)
//  3. POST /auth/v4/refresh  (proton.Client refresh-on-401 retry)
//  4. GET  /core/v4/users    (succeeds after retry, returns User payload)
//
// All four responses are synthesized. Recording against the real API isn't
// viable because Proton rejects the 401-retry refresh with 400 Invalid
// refresh token: the cold-start refresh just issued a new pair, and the
// retry sends those freshly-issued tokens back — Proton treats that as an
// unused-token replay and de-auths the session before the cassette can
// capture the second 200. Fully synthetic responses keep the cassette
// deterministic and let the consumer exercise the refresh-and-retry path.
func recordTokenRotation(ctx context.Context) (retErr error) {
	target := filepath.Join("internal", "session", "testdata", "cassettes", "token_rotation")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	// Order matters. Outermost wins on each request:
	//   - /auth/v4/refresh always returns 200 (persistent).
	//   - /core/v4/users returns 401 on the first call (one-shot), then falls
	//     through to the inner /core/v4/users persistent 200 on retry.
	injected := inject200User(http.DefaultTransport, "/core/v4/users")
	injected = inject401AccessTokenExpired(injected, "/core/v4/users")
	injected = inject200AuthRefresh(injected, "/auth/v4/refresh")
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
	if _, loginErr := freshLoginForScenario(ctx, kc); loginErr != nil {
		return loginErr
	}

	sess := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	c, err := sess.Client(ctx)
	if err != nil {
		return err
	}
	_, err = c.GetUser(ctx)
	return err
}

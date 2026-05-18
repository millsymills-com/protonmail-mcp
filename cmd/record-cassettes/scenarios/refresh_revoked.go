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

func init() {
	Register("refresh_revoked", recordRefreshRevoked)
}

func recordRefreshRevoked(ctx context.Context) (retErr error) {
	target := filepath.Join("internal", "session", "testdata", "cassettes", "refresh_revoked")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	// Layer order matters: the 401 on /core/v4/users triggers a refresh
	// attempt, which then hits the 422 on /auth/refresh. Both synthetic
	// responses sit inside the recorder (via WithRealTransport) so they
	// land in the cassette via the normal record path.
	injected := inject401AccessTokenExpired(http.DefaultTransport, "/core/v4/users")
	injected = inject422RefreshRevoked(injected, "/auth/refresh")
	rt, stop, err := testvcr.NewAtPath(target, testvcr.ModeRecord, testvcr.WithRealTransport(injected))
	if err != nil {
		return err
	}
	defer func() {
		if err := stop(); err != nil && retErr == nil {
			retErr = err
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

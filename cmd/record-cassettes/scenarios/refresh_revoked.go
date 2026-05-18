//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func registerRefreshRevoked() {
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
	if _, err := loginAndPersistSession(ctx, kc); err != nil {
		return err
	}

	// Seed the proton.Client directly from the keychain-persisted session
	// instead of building a fresh session.Session whose Client(ctx) would
	// run mgr.NewClientWithRefresh on first access. That cold-start
	// refresh hits /auth/v4/refresh and would consume the
	// inject422RefreshRevoked one-shot before the GetUser-triggered
	// refresh we actually want to record (see #95).
	seed, err := kc.LoadSession()
	if err != nil {
		return fmt.Errorf("load session for direct seed: %w", err)
	}
	driver := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	c := driver.ManagerForTest().NewClient(seed.UID, seed.AccessToken, seed.RefreshToken)
	defer c.Close()

	_, err = c.GetUser(ctx)
	return err
}

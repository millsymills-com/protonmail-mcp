//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testvcr"
)

func registerRefreshRevoked() {
	Register("refresh_revoked", recordRefreshRevoked)
}

// recordRefreshRevoked records a single cold-start /auth/v4/refresh that Proton
// rejects with 422 code 10013 ("refresh token revoked"). It runs offline: the
// injected reject short-circuits the request, so no real backend is touched and
// no credentials are stored.
//
// The keychain holds a session but no credentials, so the cold start surfaces
// the rejection without triggering a self-heal relogin during recording — the
// cassette stays a single reject interaction. That shape is what the consumer
// tests replay: a cold-start refresh whose body carries no AccessToken (unlike
// a refresh-on-401 retry, whose AccessToken-bearing body never matches a
// cold-start request and silently turns the tests into cassette-miss passes).
func recordRefreshRevoked(ctx context.Context) (retErr error) {
	target := filepath.Join("internal", "session", "testdata", "cassettes", "refresh_revoked")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	// The target must be the full versioned path: strings.Contains matches by
	// substring and "/auth/refresh" never matches "/api/auth/v4/refresh".
	injected := inject422RefreshRevoked(http.DefaultTransport, "/auth/v4/refresh")
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
	defer func() {
		if clearErr := kc.Clear(); clearErr != nil && retErr == nil {
			retErr = fmt.Errorf("clear fixture keychain: %w", clearErr)
		}
	}()
	if err := kc.SaveSession(keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}); err != nil {
		return err
	}

	driver := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	if _, err := driver.Client(ctx); err == nil {
		return fmt.Errorf("expected cold-start refresh to be rejected, got nil")
	}
	return nil
}

//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	proton "github.com/ProtonMail/go-proton-api"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

const toolsCassetteDir = "internal/tools/testdata/cassettes"

// recordReadTool logs in, wraps the transport with a recording cassette, calls
// fn with a go-proton-api Client, then logs out. The cassette is written to
// cassetteDir/scenario.yaml (relative to the repo root — the working directory
// when invoked as `go run ./cmd/record-cassettes`). All recording goes through
// testvcr.NewAtPath so the scrub hook (redactedHeaders + sensitiveJSONKeys +
// identifier rewriter) is applied before each interaction hits disk.
func recordReadTool(
	ctx context.Context,
	scenario, cassetteDir string,
	fn func(c *proton.Client) error,
) (retErr error) {
	target := filepath.Join(cassetteDir, scenario)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rt, stop, err := testvcr.NewAtPath(target, testvcr.ModeRecord)
	if err != nil {
		return fmt.Errorf("open recorder: %w", err)
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

	// Bind a second session to the cassette transport so all subsequent API
	// calls are captured.
	recSess := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	c, clientErr := recSess.Client(ctx)
	if clientErr != nil {
		return fmt.Errorf("get client: %w", clientErr)
	}
	return fn(c)
}

// recordRawTool is like recordReadTool but provides the Session itself so the
// scenario can drive the raw resty client for endpoints not exposed by
// go-proton-api (e.g. /core/v4/domains).
func recordRawTool(
	ctx context.Context,
	scenario, cassetteDir string,
	fn func(ctx context.Context, s *session.Session) error,
) (retErr error) {
	target := filepath.Join(cassetteDir, scenario)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rt, stop, err := testvcr.NewAtPath(target, testvcr.ModeRecord)
	if err != nil {
		return fmt.Errorf("open recorder: %w", err)
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
	// Initialise the proton.Client so the session bearer is seeded before the
	// raw client makes its first request.
	if _, clientErr := recSess.Client(ctx); clientErr != nil {
		return fmt.Errorf("get client: %w", clientErr)
	}
	return fn(ctx, recSess)
}

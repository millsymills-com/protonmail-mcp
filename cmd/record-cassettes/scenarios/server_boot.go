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

func init() {
	Register("boot_dispatch", recordBootDispatch)
}

func recordBootDispatch(ctx context.Context) (retErr error) {
	target := filepath.Join("internal", "server", "testdata", "cassettes", "boot_dispatch")
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

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func runStatus(
	ctx context.Context, getenv func(string) string, apiURL string,
	transport http.RoundTripper, out io.Writer,
) error {
	return runStatusWithHook(ctx, getenv, apiURL, transport, out, nil)
}

// runStatusWithHook is the test-injectable variant of runStatus. When
// hook is non-nil, it runs after the cold-start Client() call resolves
// (success or failure) and before output is written. This timing lets
// tests inject Session state (e.g. SetPersistDegradedForTest) that
// would otherwise be cleared by the cold-start refresh's own
// SaveSession set/clear logic.
func runStatusWithHook(
	ctx context.Context,
	getenv func(string) string,
	apiURL string,
	transport http.RoundTripper,
	out io.Writer,
	hook func(*session.Session),
) error {
	backend := getenv("PROTONMAIL_MCP_CREDENTIAL_BACKEND")
	if backend == "" {
		backend = "keychain"
	}
	_, _ = fmt.Fprintf(out, "backend: %s\n", backend)

	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	store, err := session.SelectStore(getenv)
	if err != nil {
		return fmt.Errorf("credential backend: %w", err)
	}
	sess := session.New(apiURL, store, session.WithTransport(transport))
	c, err := sess.Client(ctx)
	if err != nil {
		if hook != nil {
			hook(sess)
		}
		_, _ = fmt.Fprintln(out, "not logged in")
		writePersistWarning(out, sess.Status())
		return nil
	}
	if hook != nil {
		hook(sess)
	}
	u, err := c.GetUser(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "%s — %d / %d bytes\n", u.Email, u.UsedSpace, u.MaxSpace)
	writePersistWarning(out, sess.Status())
	return nil
}

func writePersistWarning(out io.Writer, st session.Status) {
	if !st.PersistDegraded {
		return
	}
	_, _ = fmt.Fprintf(out,
		"warning: token persistence degraded — rotated tokens not saved to credential store (%q). Re-run `protonmail-mcp login` to restore.\n",
		st.PersistError)
}

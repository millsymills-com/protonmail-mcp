package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func runStatus(
	ctx context.Context, apiURL string, transport http.RoundTripper, out io.Writer,
) error {
	return runStatusWithHook(ctx, apiURL, transport, out, nil)
}

// runStatusWithHook is the test-injectable variant of runStatus. When hook
// is non-nil, it runs after Session is constructed but before any API
// call, so tests can deterministically seed Session state.
func runStatusWithHook(
	ctx context.Context,
	apiURL string,
	transport http.RoundTripper,
	out io.Writer,
	hook func(*session.Session),
) error {
	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	sess := session.New(apiURL, keychain.New(), session.WithTransport(transport))
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
		"warning: token persistence degraded — rotated tokens not saved to keychain (%q). Re-run `protonmail-mcp login` to restore.\n",
		st.PersistError)
}

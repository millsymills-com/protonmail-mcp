package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

// errStatusDegraded signals that status printed its output but token
// persistence is degraded. run maps it to a distinct non-zero exit so a
// headless monitor can detect the fault from $? without parsing stdout.
var errStatusDegraded = errors.New("token persistence degraded")

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
	_, _ = fmt.Fprintf(out, "backend: %s\n", session.BackendName(getenv))

	sess, err := newSession(getenv, apiURL, transport)
	if err != nil {
		return err
	}
	c, err := sess.Client(ctx)
	if err != nil {
		if hook != nil {
			hook(sess)
		}
		_, _ = fmt.Fprintln(out, "not logged in")
		return statusResult(out, sess.Status())
	}
	if hook != nil {
		hook(sess)
	}
	u, err := c.GetUser(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "%s — %d / %d bytes\n", u.Email, u.UsedSpace, u.MaxSpace)
	return statusResult(out, sess.Status())
}

// statusResult prints any persistence warning and returns errStatusDegraded
// when persistence is degraded, so the caller can exit non-zero after the
// human-readable output has already been written.
func statusResult(out io.Writer, st session.Status) error {
	writePersistWarning(out, st)
	if st.PersistDegraded {
		return errStatusDegraded
	}
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

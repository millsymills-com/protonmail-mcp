package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

func runLogout(
	_ context.Context,
	getenv func(string) string,
	apiURL string,
	transport http.RoundTripper,
	out io.Writer,
) error {
	sess, err := newSession(getenv, apiURL, transport)
	if err != nil {
		return err
	}
	if err := sess.Logout(); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "Logged out. Credentials cleared.")
	return nil
}

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
	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	kc, err := keychain.NewFromEnv()
	if err != nil {
		return err
	}
	sess := session.New(apiURL, kc, session.WithTransport(transport))
	c, err := sess.Client(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(out, "not logged in")
		return nil
	}
	u, err := c.GetUser(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "%s — %d / %d bytes\n", u.Email, u.UsedSpace, u.MaxSpace)
	return nil
}

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func runLogout(_ context.Context, apiURL string, transport http.RoundTripper, out io.Writer) error {
	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	sess := session.New(apiURL, keychain.New(), session.WithTransport(transport))
	if err := sess.Logout(); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "Logged out. Keychain cleared.")
	return nil
}

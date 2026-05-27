package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func runLogout(_ context.Context, apiURL string, transport http.RoundTripper, out io.Writer) error {
	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	store, err := session.SelectStore(os.Getenv)
	if err != nil {
		return fmt.Errorf("credential backend: %w", err)
	}
	sess := session.New(apiURL, store, session.WithTransport(transport))
	if err := sess.Logout(); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "Logged out. Keychain cleared.")
	return nil
}

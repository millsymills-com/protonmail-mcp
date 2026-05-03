package main

import (
	"context"
	"fmt"
	"os"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func runStatus(ctx context.Context) error {
	apiURL := os.Getenv("PROTONMAIL_MCP_API_URL")
	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	kc := keychain.New()
	if _, err := kc.LoadCreds(); err != nil {
		fmt.Println("Not logged in. Run `protonmail-mcp login`.")
		return nil
	}
	sess := session.New(apiURL, kc)
	c, err := sess.Client(ctx)
	if err != nil {
		fmt.Println("Logged in (creds present), but session refresh failed:", err)
		return nil
	}
	u, err := c.GetUser(ctx)
	if err != nil {
		fmt.Println("Logged in, but GetUser failed:", err)
		return nil
	}
	fmt.Printf("Logged in as %s\n", u.Email)
	fmt.Printf("Used: %d / %d bytes\n", u.UsedSpace, u.MaxSpace)
	return nil
}

package session

import (
	"fmt"

	"github.com/millsmillsymills/protonmail-mcp/internal/credfile"
	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

// SelectStore picks the credential backend from
// PROTONMAIL_MCP_CREDENTIAL_BACKEND ("keychain" default | "file"). getenv is
// injected so callers pass os.Getenv (or a test stub).
func SelectStore(getenv func(string) string) (Store, error) {
	switch backend := getenv("PROTONMAIL_MCP_CREDENTIAL_BACKEND"); backend {
	case "", "keychain":
		return keychain.New(), nil
	case "file":
		dir, err := credfile.ResolveStateDir(getenv)
		if err != nil {
			return nil, err
		}
		return credfile.New(dir)
	default:
		return nil, fmt.Errorf("invalid PROTONMAIL_MCP_CREDENTIAL_BACKEND %q (allowed: keychain, file)", backend)
	}
}

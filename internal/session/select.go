package session

import (
	"fmt"

	"github.com/millsmillsymills/protonmail-mcp/internal/credfile"
	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

// BackendName returns the resolved credential backend name (the default when
// the env var is unset), so callers report the value SelectStore acts on
// instead of re-deriving the default independently.
func BackendName(getenv func(string) string) string {
	if b := getenv("PROTONMAIL_MCP_CREDENTIAL_BACKEND"); b != "" {
		return b
	}
	return "keychain"
}

// SelectStore picks the credential backend from
// PROTONMAIL_MCP_CREDENTIAL_BACKEND ("keychain" default | "file"). getenv is
// injected so callers pass os.Getenv (or a test stub).
func SelectStore(getenv func(string) string) (Store, error) {
	switch backend := BackendName(getenv); backend {
	case "keychain":
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

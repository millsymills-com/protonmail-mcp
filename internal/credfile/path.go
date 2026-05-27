// Package credfile stores Proton credentials and session tokens in a
// permission-locked JSON file, for headless hosts without an OS keyring.
package credfile

import (
	"fmt"
	"os"
	"path/filepath"
)

const dirName = "protonmail-mcp"

// resolveStateDir picks the credentials directory: explicit override, then the
// systemd StateDirectory, then XDG, then the home fallback. getenv is injected
// for tests.
func resolveStateDir(getenv func(string) string) (string, error) {
	if v := getenv("PROTONMAIL_MCP_STATE_DIR"); v != "" {
		return v, nil
	}
	if v := getenv("STATE_DIRECTORY"); v != "" {
		return v, nil
	}
	if v := getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, dirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve state dir: no PROTONMAIL_MCP_STATE_DIR/STATE_DIRECTORY/XDG_STATE_HOME and home unknown: %w", err)
	}
	return filepath.Join(home, ".local", "state", dirName), nil
}

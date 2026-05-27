// Package credfile stores Proton credentials and session tokens in a
// permission-locked JSON file, for headless hosts without an OS keyring.
package credfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const dirName = "protonmail-mcp"

// ResolveStateDir picks the credentials directory: explicit override, then the
// systemd StateDirectory, then XDG, then the home fallback. getenv is injected
// for tests.
func ResolveStateDir(getenv func(string) string) (string, error) {
	if v := getenv("PROTONMAIL_MCP_STATE_DIR"); v != "" {
		return v, nil
	}
	if v := getenv("STATE_DIRECTORY"); v != "" {
		// systemd sets STATE_DIRECTORY to a colon-separated list when the unit
		// declares multiple StateDirectory= entries; take the first so the path
		// isn't mistaken for one literal directory named "a:b".
		return strings.SplitN(v, ":", 2)[0], nil
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

package credfile

import (
	"path/filepath"
	"testing"
)

func TestResolveStateDir(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"explicit override wins", map[string]string{"PROTONMAIL_MCP_STATE_DIR": "/srv/p", "STATE_DIRECTORY": "/var/lib/x"}, "/srv/p"},
		{"systemd StateDirectory", map[string]string{"STATE_DIRECTORY": "/var/lib/protonmail-mcp"}, "/var/lib/protonmail-mcp"},
		{"xdg state home", map[string]string{"XDG_STATE_HOME": "/home/u/.local/state"}, filepath.Join("/home/u/.local/state", "protonmail-mcp")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveStateDir(func(k string) string { return tc.env[k] })
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

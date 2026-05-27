package session_test

import (
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/zalando/go-keyring"
)

func TestSelectStore(t *testing.T) {
	keyring.MockInit()
	get := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	t.Run("default keychain", func(t *testing.T) {
		s, err := session.SelectStore(get(nil))
		if err != nil || s == nil {
			t.Fatalf("got %v, %v", s, err)
		}
	})
	t.Run("file backend", func(t *testing.T) {
		s, err := session.SelectStore(get(map[string]string{
			"PROTONMAIL_MCP_CREDENTIAL_BACKEND": "file",
			"PROTONMAIL_MCP_STATE_DIR":          t.TempDir(),
		}))
		if err != nil || s == nil {
			t.Fatalf("got %v, %v", s, err)
		}
	})
	t.Run("invalid backend", func(t *testing.T) {
		if _, err := session.SelectStore(get(map[string]string{"PROTONMAIL_MCP_CREDENTIAL_BACKEND": "vault"})); err == nil {
			t.Fatal("expected error")
		}
	})
}

package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
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
	t.Run("file backend round-trips", func(t *testing.T) {
		// Assert behavior, not nil-ness: the returned store must actually be
		// the file backend, i.e. persist and reload a session.
		s, err := session.SelectStore(get(map[string]string{
			"PROTONMAIL_MCP_CREDENTIAL_BACKEND": "file",
			"PROTONMAIL_MCP_STATE_DIR":          t.TempDir(),
		}))
		if err != nil || s == nil {
			t.Fatalf("got %v, %v", s, err)
		}
		want := keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}
		if serr := s.SaveSession(want); serr != nil {
			t.Fatalf("SaveSession: %v", serr)
		}
		got, err := s.LoadSession()
		if err != nil || got != want {
			t.Fatalf("round-trip: got %+v err=%v want %+v", got, err, want)
		}
	})
	t.Run("invalid backend", func(t *testing.T) {
		if _, err := session.SelectStore(get(map[string]string{"PROTONMAIL_MCP_CREDENTIAL_BACKEND": "vault"})); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestBackendName(t *testing.T) {
	get := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"default keychain", nil, "keychain"},
		{"explicit keychain", map[string]string{"PROTONMAIL_MCP_CREDENTIAL_BACKEND": "keychain"}, "keychain"},
		{"file", map[string]string{"PROTONMAIL_MCP_CREDENTIAL_BACKEND": "file"}, "file"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := session.BackendName(get(tc.env)); got != tc.want {
				t.Fatalf("BackendName = %q want %q", got, tc.want)
			}
		})
	}
}

func TestProbeOtherBackend(t *testing.T) {
	get := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	seedFile := func(t *testing.T, dir string) {
		t.Helper()
		store, err := session.SelectStore(get(map[string]string{
			"PROTONMAIL_MCP_CREDENTIAL_BACKEND": "file",
			"PROTONMAIL_MCP_STATE_DIR":          dir,
		}))
		if err != nil {
			t.Fatalf("seed store: %v", err)
		}
		if err := store.SaveSession(keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}

	t.Run("keychain resolved, file session present -> hint", func(t *testing.T) {
		keyring.MockInit()
		dir := t.TempDir()
		seedFile(t, dir)
		hint, found := session.ProbeOtherBackend(get(map[string]string{
			"PROTONMAIL_MCP_STATE_DIR": dir,
		}))
		if !found {
			t.Fatal("expected to find the file-backend session")
		}
		if hint.Backend != "file" || hint.Dir != dir {
			t.Fatalf("hint = %+v, want backend=file dir=%s", hint, dir)
		}
	})

	t.Run("keychain resolved, no file session -> not found", func(t *testing.T) {
		keyring.MockInit()
		hint, found := session.ProbeOtherBackend(get(map[string]string{
			"PROTONMAIL_MCP_STATE_DIR": t.TempDir(),
		}))
		if found {
			t.Fatalf("expected no session, got hint %+v", hint)
		}
	})

	t.Run("keychain resolved, corrupt file -> unreadable hint not 'not logged in'", func(t *testing.T) {
		keyring.MockInit()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte("{not json"), 0o600); err != nil {
			t.Fatalf("seed corrupt file: %v", err)
		}
		hint, found := session.ProbeOtherBackend(get(map[string]string{
			"PROTONMAIL_MCP_STATE_DIR": dir,
		}))
		if !found {
			t.Fatal("corrupt file masked as 'not logged in'")
		}
		if !hint.Unreadable || hint.Backend != "file" || hint.Err == nil {
			t.Fatalf("hint = %+v, want unreadable file-backend hint with Err set", hint)
		}
	})

	t.Run("file resolved, keychain session present -> hint", func(t *testing.T) {
		keyring.MockInit()
		if err := keychain.New().SaveSession(
			keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}); err != nil {
			t.Fatalf("seed keychain: %v", err)
		}
		hint, found := session.ProbeOtherBackend(get(map[string]string{
			"PROTONMAIL_MCP_CREDENTIAL_BACKEND": "file",
			"PROTONMAIL_MCP_STATE_DIR":          t.TempDir(),
		}))
		if !found {
			t.Fatal("expected to find the keychain-backend session")
		}
		if hint.Backend != "keychain" || hint.Dir != "" {
			t.Fatalf("hint = %+v, want backend=keychain dir empty", hint)
		}
	})

	t.Run("file resolved, no keychain session -> not found", func(t *testing.T) {
		keyring.MockInit()
		hint, found := session.ProbeOtherBackend(get(map[string]string{
			"PROTONMAIL_MCP_CREDENTIAL_BACKEND": "file",
			"PROTONMAIL_MCP_STATE_DIR":          t.TempDir(),
		}))
		if found {
			t.Fatalf("expected no session, got hint %+v", hint)
		}
	})
}

// SelectStore's resolved name must agree with what BackendName reports, so
// status never claims a backend SelectStore wouldn't pick.
func TestSelectStoreUnknownBackendMatchesName(t *testing.T) {
	get := func(k string) string {
		if k == "PROTONMAIL_MCP_CREDENTIAL_BACKEND" {
			return "vault"
		}
		return ""
	}
	if session.BackendName(get) != "vault" {
		t.Fatal("BackendName should echo the requested backend verbatim")
	}
	if _, err := session.SelectStore(get); err == nil {
		t.Fatal("SelectStore should reject the unknown backend")
	}
}

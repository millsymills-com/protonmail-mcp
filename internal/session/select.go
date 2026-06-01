package session

import (
	"fmt"

	"github.com/millsmillsymills/protonmail-mcp/internal/credfile"
	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

const defaultBackend = "keychain"

// BackendName returns the resolved credential backend name (the default when
// the env var is unset), so callers report the value SelectStore acts on
// instead of re-deriving the default independently.
func BackendName(getenv func(string) string) string {
	if b := getenv("PROTONMAIL_MCP_CREDENTIAL_BACKEND"); b != "" {
		return b
	}
	return defaultBackend
}

// OtherBackendHint is a read-only probe result: when status finds no session on
// the resolved backend, it reports whether the OTHER backend holds one and the
// exact env change that would select it.
type OtherBackendHint struct {
	Backend string // the alternate backend that holds a session ("file" | "keychain")
	Dir     string // state dir for the file backend; empty for keychain
}

// ProbeOtherBackend checks whether a session exists under the backend NOT
// currently selected, at the resolved state dir. It is read-only: it constructs
// the alternate store and calls LoadSession, never writing or switching. found
// is false when no session is present there, when the alternate store can't be
// built (e.g. no resolvable state dir for the file backend), or when the load
// fails for any reason other than a stored session — status falls back to the
// plain "not logged in" message in those cases.
func ProbeOtherBackend(getenv func(string) string) (OtherBackendHint, bool) {
	if BackendName(getenv) == "file" {
		if sessionPresent(keychain.New()) {
			return OtherBackendHint{Backend: defaultBackend}, true
		}
		return OtherBackendHint{}, false
	}
	dir, err := credfile.ResolveStateDir(getenv)
	if err != nil {
		return OtherBackendHint{}, false
	}
	store, err := credfile.New(dir)
	if err != nil {
		return OtherBackendHint{}, false
	}
	if sessionPresent(store) {
		return OtherBackendHint{Backend: "file", Dir: dir}, true
	}
	return OtherBackendHint{}, false
}

// sessionPresent reports whether store holds a session, treating ErrNotFound
// (and any load failure) as absent so the probe never turns a backend hiccup
// into a false positive.
func sessionPresent(store Store) bool {
	_, err := store.LoadSession()
	return err == nil
}

// SelectStore picks the credential backend from
// PROTONMAIL_MCP_CREDENTIAL_BACKEND ("keychain" default | "file"). getenv is
// injected so callers pass os.Getenv (or a test stub).
func SelectStore(getenv func(string) string) (Store, error) {
	switch backend := BackendName(getenv); backend {
	case defaultBackend:
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

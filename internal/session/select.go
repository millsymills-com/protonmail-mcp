package session

import (
	"errors"
	"fmt"

	"github.com/millsymills-com/protonmail-mcp/internal/credfile"
	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
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
	Backend    string // the alternate backend that holds (or may hold) a session ("file" | "keychain")
	Dir        string // state dir for the file backend; empty for keychain
	Unreadable bool   // store exists but LoadSession failed for a reason other than ErrNotFound
	Err        error  // the underlying load error when Unreadable; never contains secrets
}

// ProbeOtherBackend checks whether a session exists under the backend NOT
// currently selected, at the resolved state dir. It is read-only: it constructs
// the alternate store and calls LoadSession, never writing or switching. found
// is false only when the alternate backend genuinely has no session
// (ErrNotFound) or its store can't even be built (e.g. no resolvable state dir
// for the file backend). When a store exists but its session can't be read
// (corrupt file, partial keychain bundle), found is true with hint.Unreadable
// set, so status surfaces the recoverable state by name instead of printing a
// bare "not logged in".
func ProbeOtherBackend(getenv func(string) string) (OtherBackendHint, bool) {
	if BackendName(getenv) == "file" {
		return probeBackend(keychain.New(), OtherBackendHint{Backend: defaultBackend})
	}
	dir, err := credfile.ResolveStateDir(getenv)
	if err != nil {
		return OtherBackendHint{}, false
	}
	store, err := credfile.New(dir)
	if err != nil {
		return OtherBackendHint{}, false
	}
	return probeBackend(store, OtherBackendHint{Backend: "file", Dir: dir})
}

// probeBackend loads store and folds the result into base: a stored session
// returns base as-is, ErrNotFound returns absent, and any other load error
// returns base flagged Unreadable so the caller can distinguish a corrupt store
// from a genuinely empty one.
func probeBackend(store Store, base OtherBackendHint) (OtherBackendHint, bool) {
	_, err := store.LoadSession()
	switch {
	case err == nil:
		return base, true
	case errors.Is(err, keychain.ErrNotFound):
		return OtherBackendHint{}, false
	default:
		base.Unreadable = true
		base.Err = err
		return base, true
	}
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

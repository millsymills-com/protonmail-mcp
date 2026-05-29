// Package keychain wraps go-keyring with typed Creds and Session bundles
// stored under the service name "protonmail-mcp".
package keychain

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	dbus "github.com/godbus/dbus/v5"
	"github.com/zalando/go-keyring"
)

const service = "protonmail-mcp"

const (
	keyUsername     = "username"
	keyPassword     = "password"
	keyTOTPSecret   = "totp_secret"
	keyUID          = "session_uid"
	keyAccessToken  = "access_token"
	keyRefreshToken = "refresh_token"
)

// Creds is the long-lived credential bundle written by `protonmail-mcp login`.
// TOTPSecret may be empty when the user opted to enter a one-shot code.
type Creds struct {
	Username   string
	Password   string
	TOTPSecret string
}

// Session is the short-lived auth state. Both tokens are rotated by go-proton-api.
type Session struct {
	UID          string
	AccessToken  string
	RefreshToken string
}

// keyring operations are indirected through package vars so tests can
// substitute a provider that fails on a chosen call ordinal. go-keyring's
// mock is all-or-nothing (every op fails or none do), so a later op failing
// after earlier ones succeed — e.g. the password Set in SaveCreds — is
// otherwise unreachable.
var (
	keyringSet    = keyring.Set
	keyringGet    = keyring.Get
	keyringDelete = keyring.Delete
)

// ErrNotFound signals that no credential bundle is stored — the "not logged
// in" state, distinct from a read/parse failure. Both this backend and the
// file backend return it (wrapped) so session.Client can map it to the
// `protonmail-mcp login` hint while surfacing genuine failures verbatim.
var ErrNotFound = errors.New("credential not stored")

// firstGet wraps the keychain's leading read so a missing primary key (no
// stored bundle) reports ErrNotFound, while any other failure keeps its
// diagnosed message. A partial bundle (primary key present, a later field
// missing) is treated as a genuine read error by the callers below, not as
// "not logged in".
func firstGet(op, key string) (string, error) {
	v, err := keyringGet(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", fmt.Errorf("%s: %w", op, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, diagnoseKeychainErr(err))
	}
	return v, nil
}

// Keychain is the typed wrapper. Construct with New().
type Keychain struct{}

func New() *Keychain { return &Keychain{} }

func (k *Keychain) SaveCreds(c Creds) error {
	if err := keyringSet(service, keyUsername, c.Username); err != nil {
		return fmt.Errorf("save username: %w", diagnoseKeychainErr(err))
	}
	if err := keyringSet(service, keyPassword, c.Password); err != nil {
		return fmt.Errorf("save password: %w", diagnoseKeychainErr(err))
	}
	// TOTP secret is optional. When the caller supplies an empty string, drop
	// any pre-existing entry so a stale secret from a prior login can't bleed
	// through. Tolerate ErrNotFound (no entry to delete).
	if c.TOTPSecret == "" {
		if err := keyringDelete(service, keyTOTPSecret); err != nil &&
			!errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("clear stale totp: %w", diagnoseKeychainErr(err))
		}
		return nil
	}
	if err := keyringSet(service, keyTOTPSecret, c.TOTPSecret); err != nil {
		return fmt.Errorf("save totp: %w", diagnoseKeychainErr(err))
	}
	return nil
}

func (k *Keychain) LoadCreds() (Creds, error) {
	u, err := firstGet("load username", keyUsername)
	if err != nil {
		return Creds{}, err
	}
	p, err := keyringGet(service, keyPassword)
	if err != nil {
		return Creds{}, fmt.Errorf("load password: %w", diagnoseKeychainErr(err))
	}
	t, err := keyringGet(service, keyTOTPSecret)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return Creds{}, fmt.Errorf("load totp: %w", diagnoseKeychainErr(err))
	}
	return Creds{Username: u, Password: p, TOTPSecret: t}, nil
}

func (k *Keychain) SaveSession(s Session) error {
	if err := keyringSet(service, keyUID, s.UID); err != nil {
		return fmt.Errorf("save uid: %w", diagnoseKeychainErr(err))
	}
	if err := keyringSet(service, keyAccessToken, s.AccessToken); err != nil {
		return fmt.Errorf("save access token: %w", diagnoseKeychainErr(err))
	}
	if err := keyringSet(service, keyRefreshToken, s.RefreshToken); err != nil {
		return fmt.Errorf("save refresh token: %w", diagnoseKeychainErr(err))
	}
	return nil
}

func (k *Keychain) LoadSession() (Session, error) {
	uid, err := firstGet("load uid", keyUID)
	if err != nil {
		return Session{}, err
	}
	at, err := keyringGet(service, keyAccessToken)
	if err != nil {
		return Session{}, fmt.Errorf("load access token: %w", diagnoseKeychainErr(err))
	}
	rt, err := keyringGet(service, keyRefreshToken)
	if err != nil {
		return Session{}, fmt.Errorf("load refresh token: %w", diagnoseKeychainErr(err))
	}
	return Session{UID: uid, AccessToken: at, RefreshToken: rt}, nil
}

func (k *Keychain) Clear() error {
	keys := []string{keyUsername, keyPassword, keyTOTPSecret, keyUID, keyAccessToken, keyRefreshToken}
	for _, key := range keys {
		if err := keyringDelete(service, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete %s: %w", key, diagnoseKeychainErr(err))
		}
	}
	return nil
}

// interactionNotAllowedExitCode is the status /usr/bin/security exits with for
// errSecInteractionNotAllowed (-25308): -25308 mod 256 = 36. go-keyring runs
// `security` and surfaces its bare *exec.ExitError (stderr discarded), so a
// locked or unreachable login keychain reaches the caller as this exit code.
// Both the write path (`add-generic-password`) and the secret-read path
// (`find-generic-password -w`) exit 36 when no SecurityAgent can prompt, so the
// Load* methods get the hint as well as the Save* methods.
const interactionNotAllowedExitCode = 36

// secretServiceUnknownDBusName is the D-Bus error name returned when no process
// owns org.freedesktop.secrets — i.e. no Secret Service is running. This is the
// failure on a headless host or a --no-create-home service user with no
// unlocked session keyring.
const secretServiceUnknownDBusName = "org.freedesktop.DBus.Error.ServiceUnknown"

// diagnoseKeychainErr augments the two opaque go-keyring errors that have a
// known, actionable cause:
//   - macOS: errSecInteractionNotAllowed, surfacing as an *exec.ExitError with
//     code 36 when the login keychain is locked or unreachable.
//   - Linux/BSD: org.freedesktop.DBus.Error.ServiceUnknown, surfacing as a
//     dbus.Error when no Secret Service is available — point at the file backend.
//
// All other errors — other exit statuses, other D-Bus errors, and go-keyring's
// own typed errors (ErrNotFound, ErrSetDataTooBig, …) — pass through unchanged,
// since they already carry an actionable message.
func diagnoseKeychainErr(cause error) error {
	return diagnoseKeychainErrFor(cause, runtime.GOOS)
}

// diagnoseKeychainErrFor is the GOOS-parameterized core, split out so tests can
// exercise the darwin and non-darwin branches deterministically. Both matches
// are structural (errors.As on *exec.ExitError / dbus.Error) rather than a
// stringified comparison, so they survive a future go-keyring wrapping the error
// or capturing stderr.
func diagnoseKeychainErrFor(cause error, goos string) error {
	if cause == nil {
		return cause
	}
	if goos == "darwin" {
		var exitErr *exec.ExitError
		if errors.As(cause, &exitErr) && exitErr.ExitCode() == interactionNotAllowedExitCode {
			return fmt.Errorf(
				"%w (login keychain is locked or unreachable from this context — "+
					"unlock via Keychain Access.app, or run `security unlock-keychain "+
					"~/Library/Keychains/login.keychain-db` in a Terminal.app session "+
					"that has a SecurityAgent connection)", cause)
		}
		return cause
	}
	var dbusErr dbus.Error
	if errors.As(cause, &dbusErr) && dbusErr.Name == secretServiceUnknownDBusName {
		return fmt.Errorf(
			"%w (no D-Bus Secret Service is available on this host — set "+
				"PROTONMAIL_MCP_CREDENTIAL_BACKEND=file and PROTONMAIL_MCP_STATE_DIR "+
				"to use the file credential backend; see docs/headless-deployment.md)",
			cause)
	}
	return cause
}

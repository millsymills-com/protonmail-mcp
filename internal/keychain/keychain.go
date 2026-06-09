// Package keychain wraps go-keyring with typed Creds and Session bundles
// stored under the service name "protonmail-mcp".
package keychain

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	dbus "github.com/godbus/dbus/v5"
	"github.com/zalando/go-keyring"
)

const service = "protonmail-mcp"

const (
	keyUsername     = "username"
	keyPassword     = "password"
	keyTOTPSecret   = "totp_secret"
	keyMailboxPass  = "mailbox_password"
	keyUID          = "session_uid"
	keyAccessToken  = "access_token"
	keyRefreshToken = "refresh_token"
	keyScope        = "session_scope"
)

// Creds is the long-lived credential bundle written by `protonmail-mcp login`.
// TOTPSecret may be empty when the user opted to enter a one-shot code.
// MailboxPassword is set only for two-password-mode accounts. Empty means
// the login Password doubles as the mailbox password (one-password mode).
type Creds struct {
	Username        string
	Password        string
	TOTPSecret      string
	MailboxPassword string
}

// Session is the short-lived auth state. Both tokens are rotated by go-proton-api.
// Scope is the token's Proton scope (e.g. "full" once two-factor completes,
// "twofactor" before); empty for sessions persisted before scope was tracked.
// It is observability metadata for keyring-unlock capability, never an
// authorization input — Proton enforces scope server-side on the token itself.
type Session struct {
	UID          string
	AccessToken  string
	RefreshToken string
	Scope        string
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
	if err := saveOptional(keyTOTPSecret, c.TOTPSecret); err != nil {
		return err
	}
	return saveOptional(keyMailboxPass, c.MailboxPassword)
}

// saveOptional sets key to value, or deletes any stale entry when value is
// empty so a secret from a prior login cannot bleed through.
func saveOptional(key, value string) error {
	if value == "" {
		if err := keyringDelete(service, key); err != nil &&
			!errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("clear stale %s: %w", key, diagnoseKeychainErr(err))
		}
		return nil
	}
	if err := keyringSet(service, key, value); err != nil {
		return fmt.Errorf("save %s: %w", key, diagnoseKeychainErr(err))
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
	m, err := keyringGet(service, keyMailboxPass)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return Creds{}, fmt.Errorf("load mailbox password: %w", diagnoseKeychainErr(err))
	}
	return Creds{Username: u, Password: p, TOTPSecret: t, MailboxPassword: m}, nil
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
	return saveOptional(keyScope, s.Scope)
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
	scope, err := keyringGet(service, keyScope)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return Session{}, fmt.Errorf("load scope: %w", diagnoseKeychainErr(err))
	}
	return Session{UID: uid, AccessToken: at, RefreshToken: rt, Scope: scope}, nil
}

func (k *Keychain) Clear() error {
	keys := []string{
		keyUsername, keyPassword, keyTOTPSecret, keyMailboxPass,
		keyUID, keyAccessToken, keyRefreshToken, keyScope,
	}
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

// noSessionBusSubstr matches godbus's failure when there is no session bus at
// all — the dominant headless case, which fails inside dbus.SessionBus() before
// org.freedesktop.secrets is ever contacted. godbus (v5.2.2) returns this as a
// plain errors.New with no stable type, so errors.As(cause, &dbus.Error) misses
// it; match defensively on the substring. Load-bearing and brittle across godbus
// versions — pinned by diagnose_test.go.
const noSessionBusSubstr = "couldn't determine address of session bus"

// diagnoseKeychainErr augments the two opaque go-keyring errors that have a
// known, actionable cause:
//   - macOS: errSecInteractionNotAllowed, surfacing as an *exec.ExitError with
//     code 36 when the login keychain is locked or unreachable.
//   - Linux/BSD: no Secret Service available — either a dbus.Error named
//     org.freedesktop.DBus.Error.ServiceUnknown (session bus up, secrets daemon
//     absent) or a plain no-session-bus error (no session bus at all). Both
//     point at the file backend.
//
// All other errors — other exit statuses, other D-Bus errors, and go-keyring's
// own typed errors (ErrNotFound, ErrSetDataTooBig, …) — pass through unchanged,
// since they already carry an actionable message.
func diagnoseKeychainErr(cause error) error {
	return diagnoseKeychainErrFor(cause, runtime.GOOS)
}

// diagnoseKeychainErrFor is the GOOS-parameterized core, split out so tests can
// exercise the darwin and non-darwin branches deterministically. The exit-code
// and ServiceUnknown matches are structural (errors.As on *exec.ExitError /
// dbus.Error) so they survive go-keyring wrapping the error; the no-session-bus
// case has no stable type and falls back to a substring match (noSessionBusSubstr).
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
	if backendUnavailable(cause) {
		return fmt.Errorf(
			"%w (no D-Bus Secret Service is available on this host — set "+
				"PROTONMAIL_MCP_CREDENTIAL_BACKEND=file and PROTONMAIL_MCP_STATE_DIR "+
				"to use the file credential backend; see docs/headless-deployment.md)",
			cause)
	}
	return cause
}

// backendUnavailable reports whether cause is a no-Secret-Service failure: a
// dbus.Error named ServiceUnknown (session bus up, secrets daemon absent) or a
// plain no-session-bus error (no session bus at all). It is the single source
// of truth shared by diagnoseKeychainErrFor's hint and the exported
// IsBackendUnavailable classifier, so both detect exactly the same condition.
func backendUnavailable(cause error) bool {
	if cause == nil {
		return false
	}
	var dbusErr dbus.Error
	serviceUnknown := errors.As(cause, &dbusErr) && dbusErr.Name == secretServiceUnknownDBusName
	return serviceUnknown || strings.Contains(cause.Error(), noSessionBusSubstr)
}

// IsBackendUnavailable reports whether err stems from the OS keyring backend
// being unreachable on this host — no D-Bus Secret Service is running, the
// headless-host failure where the very first keyring write fails before
// anything is persisted. Callers use it to distinguish "the credential store is
// dead" from "a write partially succeeded": in the former case nothing was
// written, so there is no inconsistent state to clean up and `logout` would hit
// the same dead backend. macOS locked-keychain and all other errors return
// false. Matches the same condition diagnoseKeychainErr augments its hint for.
func IsBackendUnavailable(err error) bool {
	return backendUnavailable(err)
}

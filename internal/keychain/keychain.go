// Package keychain wraps go-keyring with typed Creds and Session bundles
// stored under the service name "protonmail-mcp".
package keychain

import (
	"errors"
	"fmt"

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

// Keychain is the typed wrapper. Construct with New().
type Keychain struct{}

func New() *Keychain { return &Keychain{} }

func (k *Keychain) SaveCreds(c Creds) error {
	if err := keyringSet(service, keyUsername, c.Username); err != nil {
		return fmt.Errorf("save username: %w", err)
	}
	if err := keyringSet(service, keyPassword, c.Password); err != nil {
		return fmt.Errorf("save password: %w", err)
	}
	// TOTP secret is optional. When the caller supplies an empty string, drop
	// any pre-existing entry so a stale secret from a prior login can't bleed
	// through. Tolerate ErrNotFound (no entry to delete).
	if c.TOTPSecret == "" {
		if err := keyringDelete(service, keyTOTPSecret); err != nil &&
			!errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("clear stale totp: %w", err)
		}
		return nil
	}
	if err := keyringSet(service, keyTOTPSecret, c.TOTPSecret); err != nil {
		return fmt.Errorf("save totp: %w", err)
	}
	return nil
}

func (k *Keychain) LoadCreds() (Creds, error) {
	u, err := keyringGet(service, keyUsername)
	if err != nil {
		return Creds{}, fmt.Errorf("load username: %w", err)
	}
	p, err := keyringGet(service, keyPassword)
	if err != nil {
		return Creds{}, fmt.Errorf("load password: %w", err)
	}
	t, err := keyringGet(service, keyTOTPSecret)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return Creds{}, fmt.Errorf("load totp: %w", err)
	}
	return Creds{Username: u, Password: p, TOTPSecret: t}, nil
}

func (k *Keychain) SaveSession(s Session) error {
	if err := keyringSet(service, keyUID, s.UID); err != nil {
		return fmt.Errorf("save uid: %w", err)
	}
	if err := keyringSet(service, keyAccessToken, s.AccessToken); err != nil {
		return fmt.Errorf("save access token: %w", err)
	}
	if err := keyringSet(service, keyRefreshToken, s.RefreshToken); err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

func (k *Keychain) LoadSession() (Session, error) {
	uid, err := keyringGet(service, keyUID)
	if err != nil {
		return Session{}, fmt.Errorf("load uid: %w", err)
	}
	at, err := keyringGet(service, keyAccessToken)
	if err != nil {
		return Session{}, fmt.Errorf("load access token: %w", err)
	}
	rt, err := keyringGet(service, keyRefreshToken)
	if err != nil {
		return Session{}, fmt.Errorf("load refresh token: %w", err)
	}
	return Session{UID: uid, AccessToken: at, RefreshToken: rt}, nil
}

func (k *Keychain) Clear() error {
	keys := []string{keyUsername, keyPassword, keyTOTPSecret, keyUID, keyAccessToken, keyRefreshToken}
	for _, key := range keys {
		if err := keyringDelete(service, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete %s: %w", key, err)
		}
	}
	return nil
}

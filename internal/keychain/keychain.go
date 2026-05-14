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

// Keychain is the typed wrapper. Construct with New().
type Keychain struct{}

func New() *Keychain { return &Keychain{} }

func (k *Keychain) SaveCreds(c Creds) error {
	if err := keyring.Set(service, keyUsername, c.Username); err != nil {
		return fmt.Errorf("save username: %w", err)
	}
	// go-keyring mock fails all ops or none — intermediate failures untestable.
	if err := keyring.Set(service, keyPassword, c.Password); err != nil { //nolint:gocover
		return fmt.Errorf("save password: %w", err)
	}
	// TOTP secret is optional. When the caller supplies an empty string, drop
	// any pre-existing entry so a stale secret from a prior login can't bleed
	// through. Tolerate ErrNotFound (no entry to delete).
	if c.TOTPSecret == "" {
		// go-keyring mock fails all ops or none — intermediate failures untestable.
		if err := keyring.Delete(service, keyTOTPSecret); err != nil && //nolint:gocover
			!errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("clear stale totp: %w", err)
		}
		return nil
	}
	// go-keyring mock fails all ops or none — intermediate failures untestable.
	if err := keyring.Set(service, keyTOTPSecret, c.TOTPSecret); err != nil { //nolint:gocover
		return fmt.Errorf("save totp: %w", err)
	}
	return nil
}

func (k *Keychain) LoadCreds() (Creds, error) {
	u, err := keyring.Get(service, keyUsername)
	if err != nil {
		return Creds{}, fmt.Errorf("load username: %w", err)
	}
	// go-keyring mock fails all ops or none — intermediate failures untestable.
	p, err := keyring.Get(service, keyPassword)
	if err != nil { //nolint:gocover
		return Creds{}, fmt.Errorf("load password: %w", err)
	}
	t, err := keyring.Get(service, keyTOTPSecret)
	// go-keyring mock fails all ops or none — intermediate failures untestable.
	if err != nil && !errors.Is(err, keyring.ErrNotFound) { //nolint:gocover
		return Creds{}, fmt.Errorf("load totp: %w", err)
	}
	return Creds{Username: u, Password: p, TOTPSecret: t}, nil
}

func (k *Keychain) SaveSession(s Session) error {
	if err := keyring.Set(service, keyUID, s.UID); err != nil {
		return fmt.Errorf("save uid: %w", err)
	}
	// go-keyring mock fails all ops or none — intermediate failures untestable.
	if err := keyring.Set(service, keyAccessToken, s.AccessToken); err != nil { //nolint:gocover
		return fmt.Errorf("save access token: %w", err)
	}
	// go-keyring mock fails all ops or none — intermediate failures untestable.
	if err := keyring.Set(service, keyRefreshToken, s.RefreshToken); err != nil { //nolint:gocover
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

func (k *Keychain) LoadSession() (Session, error) {
	uid, err := keyring.Get(service, keyUID)
	if err != nil {
		return Session{}, fmt.Errorf("load uid: %w", err)
	}
	// go-keyring mock fails all ops or none — intermediate failures untestable.
	at, err := keyring.Get(service, keyAccessToken)
	if err != nil { //nolint:gocover
		return Session{}, fmt.Errorf("load access token: %w", err)
	}
	// go-keyring mock fails all ops or none — intermediate failures untestable.
	rt, err := keyring.Get(service, keyRefreshToken)
	if err != nil { //nolint:gocover
		return Session{}, fmt.Errorf("load refresh token: %w", err)
	}
	return Session{UID: uid, AccessToken: at, RefreshToken: rt}, nil
}

func (k *Keychain) Clear() error {
	keys := []string{keyUsername, keyPassword, keyTOTPSecret, keyUID, keyAccessToken, keyRefreshToken}
	for _, key := range keys {
		if err := keyring.Delete(service, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete %s: %w", key, err)
		}
	}
	return nil
}

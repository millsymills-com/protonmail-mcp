// Package session owns the long-lived go-proton-api Manager + Client and a
// parallel raw resty client that shares the same bearer token. All
// authentication mutations (login, refresh, logout) go through here.
package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

type Session struct {
	mu      sync.RWMutex
	mgr     *proton.Manager
	client  *proton.Client
	raw     *rawClient
	kc      *keychain.Keychain
	current keychain.Session
}

func New(apiURL string, kc *keychain.Keychain) *Session {
	mgr := proton.New(
		proton.WithHostURL(apiURL),
		proton.WithAppVersion(appVersionHeader()),
	)
	return &Session{
		mgr: mgr,
		kc:  kc,
		raw: newRawClient(apiURL),
	}
}

// NewForTesting bypasses keychain load and seeds an existing Session directly.
func NewForTesting(apiURL string, seed keychain.Session) (*Session, error) {
	kc := keychain.New()
	if err := kc.SaveSession(seed); err != nil {
		return nil, fmt.Errorf("seed keychain: %w", err)
	}
	s := New(apiURL, kc)
	s.current = seed
	s.raw.setBearer(seed.AccessToken)
	return s, nil
}

func (s *Session) Client(ctx context.Context) (*proton.Client, error) {
	s.mu.RLock()
	if s.client != nil {
		c := s.client
		s.mu.RUnlock()
		return c, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		return s.client, nil
	}
	sess, err := s.kc.LoadSession()
	if err != nil {
		return nil, errors.New("no session in keychain — run `protonmail-mcp login`")
	}
	c, refreshed, err := s.mgr.NewClientWithRefresh(ctx, sess.UID, sess.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh session: %w", err)
	}
	c.AddAuthHandler(func(a proton.Auth) {
		s.OnAuthRotated(keychain.Session{
			UID:          a.UID,
			AccessToken:  a.AccessToken,
			RefreshToken: a.RefreshToken,
		})
	})

	// Cold-start refresh may have rotated the refresh token; persist the new
	// values atomically. We're already holding s.mu.Lock(), so update fields
	// directly and best-effort-save to keychain rather than calling
	// OnAuthRotated (which would re-acquire the lock and deadlock).
	rotated := keychain.Session{
		UID:          refreshed.UID,
		AccessToken:  refreshed.AccessToken,
		RefreshToken: refreshed.RefreshToken,
	}
	if rotated.AccessToken == "" {
		// Some go-proton-api versions return zero-valued Auth on a no-op refresh.
		// In that case, keep the values we already loaded from keychain.
		rotated = sess
	}
	s.client = c
	s.current = rotated
	s.raw.setBearer(rotated.AccessToken)
	if err := s.kc.SaveSession(rotated); err != nil {
		// Best-effort: log only. In-memory state is correct; next cold start
		// will re-login if the persisted state is stale.
		_ = err
	}
	return c, nil
}

func (s *Session) Raw(ctx context.Context) *rawClient {
	s.mu.RLock()
	hasClient := s.client != nil
	s.mu.RUnlock()
	hasBearer := s.raw.hasBearer()
	// Only force a refresh through Client() if we have no bearer yet (cold
	// start: keychain holds tokens but we haven't refreshed yet). If a bearer
	// was seeded directly (e.g. via NewForTesting or Login), skip the refresh —
	// the proton.Client will be lazily initialized on its own first use.
	if !hasClient && !hasBearer {
		_, _ = s.Client(ctx)
	}
	return s.raw
}

func (s *Session) OnAuthRotated(next keychain.Session) {
	s.mu.Lock()
	s.current = next
	s.raw.setBearer(next.AccessToken)
	s.mu.Unlock()
	if err := s.kc.SaveSession(next); err != nil {
		_ = err // best-effort
	}
}

func (s *Session) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	s.current = keychain.Session{}
	s.raw.setBearer("")
	return s.kc.Clear()
}

type LoginInput struct {
	Username   string
	Password   string
	TOTPSecret string // raw seed; if empty, TOTPCode is consumed once
	TOTPCode   string // 6-digit code; only used if TOTPSecret is empty
}

func (s *Session) Login(ctx context.Context, in LoginInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, auth, err := s.mgr.NewClientWithLogin(ctx, in.Username, []byte(in.Password))
	if err != nil {
		return fmt.Errorf("password auth: %w", err)
	}
	if auth.TwoFA.Enabled&proton.HasTOTP != 0 {
		code := in.TOTPCode
		if code == "" && in.TOTPSecret != "" {
			code, err = generateTOTP(in.TOTPSecret)
			if err != nil {
				c.Close()
				return fmt.Errorf("generate totp: %w", err)
			}
		}
		if code == "" {
			c.Close()
			return errors.New("2FA required but no TOTP provided")
		}
		if err := c.Auth2FA(ctx, proton.Auth2FAReq{TwoFactorCode: code}); err != nil {
			c.Close()
			return fmt.Errorf("submit 2fa: %w", err)
		}
	}

	c.AddAuthHandler(func(a proton.Auth) {
		s.OnAuthRotated(keychain.Session{
			UID:          a.UID,
			AccessToken:  a.AccessToken,
			RefreshToken: a.RefreshToken,
		})
	})

	next := keychain.Session{
		UID:          auth.UID,
		AccessToken:  auth.AccessToken,
		RefreshToken: auth.RefreshToken,
	}
	if err := s.kc.SaveCreds(keychain.Creds{
		Username:   in.Username,
		Password:   in.Password,
		TOTPSecret: in.TOTPSecret,
	}); err != nil {
		c.Close()
		return fmt.Errorf("save creds: %w", err)
	}
	if err := s.kc.SaveSession(next); err != nil {
		c.Close()
		return fmt.Errorf("save session: %w", err)
	}

	s.client = c
	s.current = next
	s.raw.setBearer(next.AccessToken)
	return nil
}

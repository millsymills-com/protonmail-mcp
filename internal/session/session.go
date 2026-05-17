// Package session owns the long-lived go-proton-api Manager + Client and a
// parallel raw resty client that shares the same bearer token. All
// authentication mutations (login, refresh, logout) go through here.
package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/go-resty/resty/v2"
	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
)

// keychainStore is the minimal persistence surface Session needs.
// *keychain.Keychain satisfies it; tests inject failure-injecting wrappers.
type keychainStore interface {
	SaveCreds(keychain.Creds) error
	LoadCreds() (keychain.Creds, error)
	SaveSession(keychain.Session) error
	LoadSession() (keychain.Session, error)
	Clear() error
}

type Session struct {
	mu      sync.RWMutex
	mgr     *proton.Manager
	client  *proton.Client
	raw     *rawClient
	kc      keychainStore
	current keychain.Session
}

type Option func(*config)

type config struct {
	transport http.RoundTripper
}

// nil transport (default) falls back to http.DefaultTransport for both clients.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *config) { c.transport = rt }
}

func New(apiURL string, kc keychainStore, opts ...Option) *Session {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	mgrOpts := []proton.Option{
		proton.WithHostURL(apiURL),
		proton.WithAppVersion(appVersionHeader()),
	}
	if cfg.transport != nil {
		mgrOpts = append(mgrOpts, proton.WithTransport(cfg.transport))
	}
	return &Session{
		mgr: proton.New(mgrOpts...),
		kc:  kc,
		raw: newRawClient(apiURL, cfg.transport),
	}
}

// RawClientForTest is the minimal surface tests need from the raw client.
type RawClientForTest interface {
	Get(ctx context.Context, path string) (*resty.Response, error)
}

func (s *Session) RawForTest() RawClientForTest { return s.raw }

func (s *Session) ManagerForTest() *proton.Manager { return s.mgr }

// NewForTesting bypasses keychain load and seeds an existing Session directly.
func NewForTesting(apiURL string, seed keychain.Session, opts ...Option) (*Session, error) {
	kc := keychain.New()
	if err := kc.SaveSession(seed); err != nil {
		return nil, fmt.Errorf("seed keychain: %w", err)
	}
	s := New(apiURL, kc, opts...)
	s.current = seed
	s.raw.setAuth(seed.AccessToken, seed.UID)
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
		return nil, fmt.Errorf("%w — run `protonmail-mcp login`", proterr.ErrNoSession)
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
	s.raw.setAuth(rotated.AccessToken, rotated.UID)
	if err := s.kc.SaveSession(rotated); err != nil {
		slog.Warn("session: persist rotated tokens failed", "err", err)
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
	s.raw.setAuth(next.AccessToken, next.UID)
	s.mu.Unlock()
	if err := s.kc.SaveSession(next); err != nil {
		slog.Warn("session: persist rotated tokens failed", "err", err)
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
	s.raw.setAuth("", "")
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
	if err := persistLoginState(s.kc, keychain.Creds{
		Username:   in.Username,
		Password:   in.Password,
		TOTPSecret: in.TOTPSecret,
	}, next); err != nil {
		c.Close()
		return err
	}

	s.client = c
	s.current = next
	s.raw.setAuth(next.AccessToken, next.UID)
	return nil
}

// persistLoginState writes credentials and the post-auth session to the
// keychain. On any failure between starting and finishing those two writes,
// it rolls back via kc.Clear() so the keychain does not end up holding a
// password without a matching session (or vice versa). The original cause is
// preserved; a rollback failure is folded in via errors.Join.
//
// Trade-off: rollback clears to the *empty* state, not to whatever was
// present before Login was invoked. Re-logging in over a prior successful
// login with bad new credentials will leave the keychain empty rather than
// restored to the prior state. Snapshotting the prior state is out of scope.
func persistLoginState(kc keychainStore, creds keychain.Creds, sess keychain.Session) error {
	if err := kc.SaveCreds(creds); err != nil {
		return rollbackLoginPersist(kc, "save creds", err)
	}
	if err := kc.SaveSession(sess); err != nil {
		return rollbackLoginPersist(kc, "save session", err)
	}
	return nil
}

func rollbackLoginPersist(kc keychainStore, op string, cause error) error {
	primary := fmt.Errorf("%s: %w", op, cause)
	if rerr := kc.Clear(); rerr != nil {
		// Clear failed — keychain may hold partial state that can't be
		// reconciled here. Surface a recovery hint so the user knows to
		// invoke `protonmail-mcp logout` (which re-tries Clear) before
		// the next login attempt.
		return errors.Join(primary, fmt.Errorf(
			"login rollback: %w (keychain may be inconsistent; run `protonmail-mcp logout` to clear)",
			rerr))
	}
	return primary
}

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
	"os"
	"strings"
	"sync"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/go-resty/resty/v2"
	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/keyring"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
)

// Store is the persistence surface a Session needs. *keychain.Keychain and
// *credfile.Store both satisfy it; tests inject failure-injecting wrappers.
type Store interface {
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
	kc      Store
	current keychain.Session
	// unlockMu single-flights keyring Unlock so concurrent first-use callers
	// don't each run the salt/key network round-trips; it is taken only on the
	// cache-miss path and never while holding mu (Unlock does network I/O).
	unlockMu sync.Mutex
	// keyrings is the lazily-unlocked, session-lifetime PGP keyring cache. Holds
	// decrypted private key material; nil until first crypto use and dropped on
	// logout/relogin. Never persisted, never logged.
	keyrings *keyring.Keyrings
	// keyFetcher resolves the KeyFetcher the cache-miss unlock runs against. nil
	// in production, where it falls back to s.Client(ctx); tests set it to drive
	// the unlock orchestration (LoadCreds, mailbox fallback, cache population)
	// without standing up a live backend.
	keyFetcher func(context.Context) (keyring.KeyFetcher, error)
	// poisoned indicates the in-process Session and the keychain are known
	// to be in inconsistent states because a Login persist rollback's Clear
	// itself failed. Subsequent operations that would otherwise read from
	// the keychain (e.g. cold-start refresh in Client) short-circuit until
	// the user re-runs Logout (which retries Clear) or Login (which writes
	// fresh state).
	poisoned bool

	// persistDegraded is true when the most recent SaveSession write failed;
	// in-memory tokens still work but the keychain holds stale tokens.
	// Cleared by the next successful SaveSession or by Logout.
	persistDegraded  bool
	persistErrReason string

	// reloginExhausted is set after an unattended self-heal relogin attempt
	// did not produce a working client (it failed, hit a CAPTCHA, or no usable
	// creds were stored). It stops Client from re-running SRP on every
	// subsequent call — which would hammer Proton's login endpoint and risk its
	// anti-abuse lockout — until Login or Logout resets it.
	reloginExhausted bool
}

// ErrSessionInconsistent is returned when a prior Login persist rollback
// failed to clear the keychain, so the in-memory and on-disk state diverge.
// The hint is to invoke Logout (which retries Clear) and Login again.
var ErrSessionInconsistent = errors.New(
	"session state inconsistent (prior login rollback failed to clear keychain); " +
		"run `protonmail-mcp logout` then `protonmail-mcp login`")

// ErrTOTPRequired is returned from Login when the account has 2FA enabled but
// the LoginInput supplied no TOTP code or secret. Callers should use
// errors.Is(err, ErrTOTPRequired) to branch into a 2FA-prompt flow rather
// than matching the error string.
var ErrTOTPRequired = errors.New("2FA required but no TOTP provided")

// ErrMailboxPasswordRequired is returned from Login when the account uses
// two-password mode but LoginInput supplied no MailboxPassword. Callers should
// use errors.Is to branch into a mailbox-password prompt.
var ErrMailboxPasswordRequired = errors.New(
	"mailbox password required (two-password mode) but none provided")

// Status reports session health. PersistDegraded is true when the most recent
// SaveSession write failed; in-memory tokens still work for the current
// process. KeyringUnlock classifies whether the current token's scope can
// unlock the mailbox keyring ("ok", "under_scoped", or "unknown").
type Status struct {
	PersistDegraded bool
	PersistError    string
	Scope           string
	KeyringUnlock   string
}

// Status returns a snapshot of session health.
func (s *Session) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{
		PersistDegraded: s.persistDegraded,
		PersistError:    s.persistErrReason,
		Scope:           s.current.Scope,
		KeyringUnlock:   keyringUnlockState(s.current.Scope),
	}
}

// keyringUnlockState classifies a Proton token scope by whether it can unlock
// the mailbox keyring. A scope containing "full" can (the post-two-factor
// state); a non-empty scope without it cannot (e.g. "twofactor", the
// pre-2FA state); an empty scope is unknown — the session predates scope
// tracking, so capability can't be asserted without attempting an unlock.
func keyringUnlockState(scope string) string {
	if scope == "" {
		return "unknown"
	}
	for _, f := range strings.Fields(scope) {
		if f == "full" {
			return "ok"
		}
	}
	return "under_scoped"
}

// Service is the session surface the tools package depends on. *Session
// implements it; tests wrap a real *Session to inject failures (e.g. a keyring
// unlock error) at the handler boundary without standing up a live backend.
type Service interface {
	Raw(ctx context.Context) *rawClient
	Status() Status
	Client(ctx context.Context) (*proton.Client, error)
	Keyrings(ctx context.Context) (*keyring.Keyrings, error)
}

// clearKeyringCache wipes the cached keyrings' private key material and drops
// the reference. Caller must hold s.mu.
func (s *Session) clearKeyringCache() {
	if s.keyrings != nil {
		s.keyrings.ClearPrivateParams()
	}
	s.keyrings = nil
}

// Keyrings returns the session's unlocked PGP keyrings, unlocking them on
// first use and caching the result for the session lifetime. The mailbox
// password is the stored MailboxPassword, or the login password for
// one-password accounts.
func (s *Session) Keyrings(ctx context.Context) (*keyring.Keyrings, error) {
	s.mu.RLock()
	cached := s.keyrings
	s.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	// Single-flight the unlock: serialize cache-miss callers and re-check the
	// cache after acquiring, so only the first runs the salt/key round-trips
	// and a concurrent winner's keyrings are reused instead of unlocked twice.
	s.unlockMu.Lock()
	defer s.unlockMu.Unlock()
	s.mu.RLock()
	cached = s.keyrings
	s.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	f, err := s.fetcher(ctx)
	if err != nil {
		return nil, err
	}
	creds, err := s.kc.LoadCreds()
	if err != nil {
		return nil, fmt.Errorf("load creds for keyring unlock: %w", err)
	}
	mailbox := creds.MailboxPassword
	if mailbox == "" {
		mailbox = creds.Password
	}
	krs, err := keyring.Unlock(ctx, f, []byte(mailbox))
	if err != nil {
		return nil, fmt.Errorf("unlock keyrings: %w", err)
	}
	s.mu.Lock()
	s.keyrings = krs
	s.mu.Unlock()
	return krs, nil
}

// fetcher resolves the KeyFetcher the cache-miss unlock runs against. It uses
// the injected keyFetcher seam when set (tests), otherwise the live
// *proton.Client.
func (s *Session) fetcher(ctx context.Context) (keyring.KeyFetcher, error) {
	if s.keyFetcher != nil {
		return s.keyFetcher(ctx)
	}
	return s.Client(ctx)
}

// SetPersistDegradedForTest injects degraded state for tests that hold
// only a *Session and cannot drive a Store failure directly.
func (s *Session) SetPersistDegradedForTest(reason string) {
	s.mu.Lock()
	s.persistDegraded = reason != ""
	s.persistErrReason = reason
	s.mu.Unlock()
}

type Option func(*config)

type config struct {
	transport       http.RoundTripper
	skipProofVerify bool
}

// nil transport (default) falls back to http.DefaultTransport for both clients.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *config) { c.transport = rt }
}

// WithSkipProofVerificationForRecording disables the SRP ServerProof check
// on the underlying proton.Manager. Test/recording-only: a login_no_2fa
// cassette can't reproduce the same ServerProof on replay because the
// client's ephemeral changes per run; without this, the replay client
// rejects the recorded ServerProof and login fails. The verbose name is
// intentional — calling this from production code disables a MITM check.
func WithSkipProofVerificationForRecording() Option {
	return func(c *config) { c.skipProofVerify = true }
}

func New(apiURL string, kc Store, opts ...Option) *Session {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	// Test hook: the login_no_2fa replay test sets
	// PROTONMAIL_MCP_TEST_SKIP_PROOFS=1 to disable the SRP ServerProof check
	// on the underlying Manager (the recorded ServerProof can't match the
	// per-run client ephemeral). testing.Testing() gates the env-var read to
	// `go test` binaries only, so a shipped production binary never honours
	// the var — it can't be set by a misconfigured agent, leaked container
	// env, or a curious user inspecting strings(1).
	if !cfg.skipProofVerify && testing.Testing() &&
		os.Getenv("PROTONMAIL_MCP_TEST_SKIP_PROOFS") != "" {
		cfg.skipProofVerify = true
		slog.Warn("session: SRP ServerProof verification disabled by test hook")
	}
	mgrOpts := []proton.Option{
		proton.WithHostURL(apiURL),
		proton.WithAppVersion(appVersionHeader()),
	}
	if cfg.transport != nil {
		mgrOpts = append(mgrOpts, proton.WithTransport(cfg.transport))
	}
	if cfg.skipProofVerify {
		mgrOpts = append(mgrOpts, proton.WithSkipVerifyProofs())
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

// CurrentForTest returns the in-memory session snapshot for tests that
// need to verify rotation reached the in-memory state independently of
// the keychain (e.g. asserting that a persist failure did not block
// the rotation itself).
func (s *Session) CurrentForTest() keychain.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

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
	if s.poisoned {
		return nil, ErrSessionInconsistent
	}
	sess, err := s.kc.LoadSession()
	if errors.Is(err, keychain.ErrNotFound) {
		return nil, fmt.Errorf("%w — run `protonmail-mcp login`", proterr.ErrNoSession)
	}
	if err != nil {
		// A corrupt file, a permission error, or a locked keychain is not the
		// "not logged in" state — surface it verbatim so the user can fix the
		// real problem instead of looping on `login` (which would hit the same
		// error). Only keychain.ErrNotFound maps to the login hint above.
		return nil, fmt.Errorf("load session from credential store: %w", err)
	}
	c, refreshed, err := s.mgr.NewClientWithRefresh(ctx, sess.UID, sess.RefreshToken)
	if err != nil {
		pe := proterr.Map(err)
		// Only a rejected/revoked refresh token warrants an unattended relogin
		// (proterr.RefreshRejected: 401, or code 10013 "refresh token
		// revoked"). A generic 422/400, a transport error, a 5xx, or a 429 must
		// NOT trigger relogin: re-login can't fix Proton being down, and
		// repeated SRP attempts risk tripping its anti-abuse lockout
		// (~10 logins/min). The reloginExhausted latch bounds it to one attempt
		// until Login/Logout resets it. reloginLocked sets s.client on success;
		// we already hold s.mu.
		if proterr.RefreshRejected(err) && !s.reloginExhausted {
			healed, captcha := s.reloginLocked(ctx)
			if healed != nil {
				return healed, nil
			}
			s.reloginExhausted = true
			if captcha != nil {
				return nil, captcha
			}
		}
		// No relogin, or it failed: forward a stable auth_required code + hint
		// when Proton classified it that way, otherwise the wrapped error.
		if pe != nil && pe.Code == "proton/auth_required" {
			return nil, pe
		}
		return nil, fmt.Errorf("refresh session: %w", err)
	}
	c.AddAuthHandler(func(a proton.Auth) {
		s.OnAuthRotated(keychain.Session{
			UID:          a.UID,
			AccessToken:  a.AccessToken,
			RefreshToken: a.RefreshToken,
			Scope:        a.Scope,
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
		Scope:        refreshed.Scope,
	}
	if rotated.AccessToken == "" {
		// Some go-proton-api versions return zero-valued Auth on a no-op refresh.
		// In that case, keep the values we already loaded from keychain.
		rotated = sess
	}
	if rotated.Scope == "" {
		// A refresh that rotated tokens but omitted scope must not erase the
		// scope we loaded from the keychain — Status would otherwise flip a
		// full-scope session to "unknown".
		rotated.Scope = sess.Scope
	}
	s.client = c
	s.current = rotated
	s.raw.setAuth(rotated.AccessToken, rotated.UID)
	if err := s.kc.SaveSession(rotated); err != nil {
		s.persistDegraded = true
		s.persistErrReason = err.Error()
		slog.Warn("session: persist rotated tokens failed", "err", err)
	} else {
		s.persistDegraded = false
		s.persistErrReason = ""
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
	if next.Scope == "" {
		// go-proton-api's auth handler omits scope on a plain token refresh;
		// preserve the scope already established (e.g. "full" from login) rather
		// than regressing it to unknown.
		next.Scope = s.current.Scope
	}
	s.current = next
	s.raw.setAuth(next.AccessToken, next.UID)
	s.mu.Unlock()
	if err := s.kc.SaveSession(next); err != nil {
		s.mu.Lock()
		s.persistDegraded = true
		s.persistErrReason = err.Error()
		s.mu.Unlock()
		slog.Warn("session: persist rotated tokens failed", "err", err)
		return
	}
	s.mu.Lock()
	s.persistDegraded = false
	s.persistErrReason = ""
	s.mu.Unlock()
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
	if err := s.kc.Clear(); err != nil {
		// Leave poisoned flag set if it was set — Clear failed again, so
		// state is still inconsistent.
		return err
	}
	s.poisoned = false
	s.persistDegraded = false
	s.persistErrReason = ""
	s.reloginExhausted = false
	s.clearKeyringCache()
	return nil
}

type LoginInput struct {
	Username        string
	Password        string
	TOTPSecret      string // raw seed; if empty, TOTPCode is consumed once
	TOTPCode        string // 6-digit code; only used if TOTPSecret is empty
	MailboxPassword string // required only in two-password mode
}

func (s *Session) Login(ctx context.Context, in LoginInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loginLocked(ctx, in)
}

// reloginLocked attempts unattended self-heal after a cold-start refresh
// failure by re-running login from stored credentials. The caller MUST hold
// s.mu. It returns:
//   - (client, nil)  on a successful relogin (in-memory + persisted state set);
//   - (nil, captcha) when Proton answered with a human-verification challenge
//     that cannot be solved without an operator;
//   - (nil, nil)     when no usable creds are stored, the stored TOTP secret is
//     absent, or the relogin failed for any other reason — leaving the caller
//     to surface the original refresh error.
func (s *Session) reloginLocked(ctx context.Context) (*proton.Client, error) {
	creds, err := s.kc.LoadCreds()
	if err != nil {
		// A real load failure (corrupt file, permission error) is distinct from
		// "no creds stored"; surface it so a headless operator can diagnose why
		// self-heal could not run instead of seeing only the refresh error.
		if !errors.Is(err, keychain.ErrNotFound) {
			slog.Warn("session: self-heal aborted, could not load stored credentials", "err", err)
		}
		return nil, nil
	}
	if creds.Username == "" || creds.Password == "" {
		return nil, nil
	}
	rerr := s.loginLocked(ctx, LoginInput{
		Username:        creds.Username,
		Password:        creds.Password,
		TOTPSecret:      creds.TOTPSecret,
		MailboxPassword: creds.MailboxPassword,
	})
	if rerr != nil {
		cpe := proterr.Map(rerr)
		if cpe != nil && cpe.Code == "proton/captcha" {
			return nil, cpe
		}
		// Log the mapped code (never the wrapped cause, which could echo
		// credentials) so the failed self-heal is observable in daemon logs.
		code := "unknown"
		if cpe != nil {
			code = cpe.Code
		}
		slog.Warn("session: self-heal relogin failed", "code", code)
		return nil, nil
	}
	return s.client, nil
}

// loginLocked performs password+2FA auth and persists state. Caller MUST hold
// s.mu (so the cold-start self-heal path can reuse it without re-locking).
func (s *Session) loginLocked(ctx context.Context, in LoginInput) error {
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
			return ErrTOTPRequired
		}
		// Proton rotates the refresh token during /auth/v4/2fa, but
		// go-proton-api's Auth2FA discards the response body. Without
		// intercepting it, keychain ends up holding the pre-2FA refresh
		// token, which Proton rejects with Code=10013 on the next
		// /auth/v4/refresh (#86). The post-request hook captures the
		// fresh credentials from the 2FA response so the keychain and
		// the in-memory client both use the post-2FA tokens.
		//
		// Cost: AddPostRequestHook registers the closure on the shared
		// resty.Client owned by the Manager, not on this Client. Client
		// disposal does not unregister it. Each successful 2FA login
		// adds one closure to that chain; entries from disposed
		// clients short-circuit via the clientID guard. Bounded by the
		// number of 2FA logins in a process's lifetime, not by request
		// volume, so the leak stays small in practice.
		captured := newAuth2FACapture()
		c.AddPostRequestHook(captured.hook)
		if err = c.Auth2FA(ctx, proton.Auth2FAReq{TwoFactorCode: code}); err != nil {
			c.Close()
			return fmt.Errorf("submit 2fa: %w", err)
		}
		if post := captured.merge(auth); post != nil {
			c.Close()
			c = s.mgr.NewClient(post.UID, post.AccessToken, post.RefreshToken)
			auth = *post
		}
	}

	mailboxPassword, err := chooseMailboxPassword(auth.PasswordMode, in.MailboxPassword)
	if err != nil {
		c.Close()
		return err
	}

	c.AddAuthHandler(func(a proton.Auth) {
		s.OnAuthRotated(keychain.Session{
			UID:          a.UID,
			AccessToken:  a.AccessToken,
			RefreshToken: a.RefreshToken,
			Scope:        a.Scope,
		})
	})

	next := keychain.Session{
		UID:          auth.UID,
		AccessToken:  auth.AccessToken,
		RefreshToken: auth.RefreshToken,
		Scope:        auth.Scope,
	}
	if err := s.persistLoginState(keychain.Creds{
		Username:        in.Username,
		Password:        in.Password,
		TOTPSecret:      in.TOTPSecret,
		MailboxPassword: mailboxPassword,
	}, next); err != nil {
		c.Close()
		return err
	}

	s.client = c
	s.current = next
	s.raw.setAuth(next.AccessToken, next.UID)
	s.reloginExhausted = false
	s.clearKeyringCache()
	return nil
}

// chooseMailboxPassword resolves the mailbox password to persist for the
// account's password mode. Two-password accounts require a supplied value;
// one-password accounts reuse the login password (persisted empty so the
// unlock path falls back to it), keeping existing accounts migration-free.
func chooseMailboxPassword(mode proton.PasswordMode, supplied string) (string, error) {
	switch mode {
	case proton.TwoPasswordMode:
		if supplied == "" {
			return "", ErrMailboxPasswordRequired
		}
		return supplied, nil
	case proton.OnePasswordMode:
		return "", nil
	default:
		return "", fmt.Errorf("unrecognised password mode %d", mode)
	}
}

// persistLoginState writes credentials and the post-auth session to the
// keychain. On any failure between starting and finishing those two writes,
// it rolls back via kc.Clear() so the keychain does not end up holding a
// password without a matching session (or vice versa). The original cause is
// preserved; a rollback failure is folded in via errors.Join, and the
// Session is marked poisoned so subsequent in-process operations short-
// circuit with ErrSessionInconsistent instead of acting on stale keychain
// state.
//
// Caller must hold s.mu.Lock(); this method writes s.poisoned.
//
// Trade-off: rollback clears to the *empty* state, not to whatever was
// present before Login was invoked. Re-logging in over a prior successful
// login with bad new credentials will leave the keychain empty rather than
// restored to the prior state. Snapshotting the prior state is out of scope.
func (s *Session) persistLoginState(creds keychain.Creds, sess keychain.Session) error {
	if err := s.kc.SaveCreds(creds); err != nil {
		return s.rollbackLoginPersist("save creds", err)
	}
	if err := s.kc.SaveSession(sess); err != nil {
		return s.rollbackLoginPersist("save session", err)
	}
	s.persistDegraded = false
	s.persistErrReason = ""
	return nil
}

func (s *Session) rollbackLoginPersist(op string, cause error) error {
	primary := fmt.Errorf("%s: %w", op, cause)
	if keychain.IsBackendUnavailable(cause) {
		// The first write failed before persisting anything, so there is no
		// partial state to reconcile and no point clearing — `logout` would
		// hit the same dead backend. Don't poison; report the dead backend.
		return fmt.Errorf(
			"%w (nothing was persisted; the credential backend is unreachable, "+
				"so no cleanup is needed)", primary)
	}
	if rerr := s.kc.Clear(); rerr != nil {
		// Clear failed — keychain may hold partial state that can't be
		// reconciled here. Mark the Session poisoned so Client/Raw fail
		// loud with ErrSessionInconsistent, and surface a recovery hint
		// pointing the user at Logout (which re-tries Clear).
		s.poisoned = true
		return errors.Join(primary, fmt.Errorf(
			"login rollback: %w (keychain may be inconsistent; run `protonmail-mcp logout` to clear)",
			rerr))
	}
	return primary
}

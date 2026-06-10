package session

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	proton "github.com/ProtonMail/go-proton-api"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/keyring"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
)

// scopeDeniedFetcher fails the unlock at GetSalts with the exact #195 sentinel
// (HTTP 403 / Code 9100), so keyring.Unlock tags ErrKeyringUnlockScope and the
// self-heal trigger in Keyrings fires.
type scopeDeniedFetcher struct{}

func (scopeDeniedFetcher) GetSalts(context.Context) (proton.Salts, error) {
	return nil, &proton.APIError{Status: http.StatusForbidden, Code: 9100}
}
func (scopeDeniedFetcher) GetUser(context.Context) (proton.User, error) {
	return proton.User{}, errors.New("GetUser must not be reached on a salts scope denial")
}
func (scopeDeniedFetcher) GetAddresses(context.Context) ([]proton.Address, error) {
	return nil, errors.New("GetAddresses must not be reached on a salts scope denial")
}

// transientFetcher fails the unlock at GetSalts with a non-scope error (HTTP 500
// / Code 2001), so keyring.Unlock returns an error that is NOT the #195 scope
// sentinel — modelling a transient outage on the post-relogin retry-unlock.
type transientFetcher struct{}

func (transientFetcher) GetSalts(context.Context) (proton.Salts, error) {
	return nil, &proton.APIError{Status: http.StatusInternalServerError, Code: 2001}
}
func (transientFetcher) GetUser(context.Context) (proton.User, error) {
	return proton.User{}, errors.New("GetUser must not be reached on a salts error")
}
func (transientFetcher) GetAddresses(context.Context) ([]proton.Address, error) {
	return nil, errors.New("GetAddresses must not be reached on a salts error")
}

// TestKeyringsSelfHealsUnderScopedSession proves the success path: an
// under-scoped session whose first unlock is scope-denied self-heals via one
// relogin (the seam swaps in a full-scope fetcher), the unlock then succeeds and
// caches, and the latch stays reset so a later denial can self-heal again.
func TestKeyringsSelfHealsUnderScopedSession(t *testing.T) {
	const pass = "mailbox-pw"
	current := keyring.KeyFetcher(scopeDeniedFetcher{})
	s := &Session{
		kc:  &credKC{creds: keychain.Creds{Password: pass}},
		raw: newRawClient("http://invalid.test", nil),
	}
	s.keyFetcher = func(context.Context) (keyring.KeyFetcher, error) { return current, nil }

	var reloginCalls int
	s.reloginScope = func(context.Context) (bool, error) {
		reloginCalls++
		current = lockedFetcher(t, pass) // fresh full-scope session can unlock
		return true, nil
	}

	krs, err := s.Keyrings(t.Context())
	if err != nil {
		t.Fatalf("self-heal Keyrings: %v", err)
	}
	if krs == nil || krs.User == nil {
		t.Fatal("expected an unlocked user keyring after self-heal")
	}
	if reloginCalls != 1 {
		t.Fatalf("relogin attempted %d times, want exactly 1", reloginCalls)
	}
	if s.keyrings != krs {
		t.Fatal("healed keyrings must be cached")
	}
	if s.scopeReloginSpent() {
		t.Fatal("latch must stay reset after a successful heal so a later denial can self-heal")
	}
}

// TestKeyringsScopeDenialIneligibleCredsFallsThrough proves the eligibility
// gate: with no usable stored creds (username empty) the real reloginLocked
// short-circuits before any network login, so Keyrings returns the actionable
// #195 error and latches — no relogin loop. No test seam: the real relogin path
// runs and must decline without a live call.
func TestKeyringsScopeDenialIneligibleCredsFallsThrough(t *testing.T) {
	s := newSessionWithFetcher(
		&credKC{creds: keychain.Creds{Password: "pw"}}, // Username empty → ineligible
		scopeDeniedFetcher{},
	)

	_, err := s.Keyrings(t.Context())
	if !errors.Is(err, proterr.ErrKeyringUnlockScope) {
		t.Fatalf("want ErrKeyringUnlockScope fall-through, got %v", err)
	}
	if !s.scopeReloginSpent() {
		t.Fatal("latch must hold after an ineligible self-heal attempt")
	}
	if s.keyrings != nil {
		t.Fatal("cache must stay empty when the unlock remains denied")
	}
}

// TestKeyringsScopeSelfHealLatchHoldsOnUnfixableDenial proves loop safety: when
// the relogin reports success but the fresh token is still under-scoped (an
// unfixable 9100), exactly one relogin runs, the latch holds, and a second
// cache-miss does not attempt another relogin.
func TestKeyringsScopeSelfHealLatchHoldsOnUnfixableDenial(t *testing.T) {
	s := newSessionWithFetcher(
		&credKC{creds: keychain.Creds{Password: "pw"}},
		scopeDeniedFetcher{}, // stays scope-denied even after the "successful" relogin
	)
	var reloginCalls int
	s.reloginScope = func(context.Context) (bool, error) { reloginCalls++; return true, nil }

	if _, err := s.Keyrings(t.Context()); !errors.Is(err, proterr.ErrKeyringUnlockScope) {
		t.Fatalf("first call: want ErrKeyringUnlockScope, got %v", err)
	}
	if !s.scopeReloginSpent() {
		t.Fatal("latch must hold after the one attempt failed to restore scope")
	}
	if _, err := s.Keyrings(t.Context()); !errors.Is(err, proterr.ErrKeyringUnlockScope) {
		t.Fatalf("second call: want ErrKeyringUnlockScope, got %v", err)
	}
	if reloginCalls != 1 {
		t.Fatalf("relogin ran %d times, want exactly 1 (latch must bound it)", reloginCalls)
	}
}

// TestKeyringsScopeSelfHealLogsNoSecrets captures the self-heal log output and
// asserts no credential or token material appears on any line. The ineligible
// path exercises the real reloginLocked plus the new attempt/latch log lines.
func TestKeyringsScopeSelfHealLogsNoSecrets(t *testing.T) {
	const secret = "do-not-log-this-mailbox-password"
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	s := newSessionWithFetcher(
		&credKC{creds: keychain.Creds{Password: secret}}, // ineligible (no username)
		scopeDeniedFetcher{},
	)
	if _, err := s.Keyrings(t.Context()); !errors.Is(err, proterr.ErrKeyringUnlockScope) {
		t.Fatalf("want ErrKeyringUnlockScope, got %v", err)
	}

	logged := buf.String()
	if strings.Contains(logged, secret) {
		t.Fatalf("self-heal logs leaked the mailbox password:\n%s", logged)
	}
	if !strings.Contains(logged, "self-heal relogin") {
		t.Fatalf("expected a structured self-heal log line, got:\n%s", logged)
	}
}

// TestKeyringsScopeSelfHealConcurrentSingleRelogin proves the self-heal stays
// inside the unlockMu single-flight: two concurrent first-callers on a
// scope-denied session trigger exactly one relogin (not two hammering Proton's
// login endpoint), and the waiter reuses the winner's healed keyrings via the
// post-lock cache re-check rather than re-running the relogin.
func TestKeyringsScopeSelfHealConcurrentSingleRelogin(t *testing.T) {
	const pass = "mailbox-pw"
	current := keyring.KeyFetcher(scopeDeniedFetcher{})
	s := &Session{
		kc:  &credKC{creds: keychain.Creds{Password: pass}},
		raw: newRawClient("http://invalid.test", nil),
	}
	s.keyFetcher = func(context.Context) (keyring.KeyFetcher, error) { return current, nil }

	var reloginCalls atomic.Int32
	s.reloginScope = func(context.Context) (bool, error) {
		reloginCalls.Add(1)
		current = lockedFetcher(t, pass)
		return true, nil
	}

	var wg sync.WaitGroup
	results := make([]*keyring.Keyrings, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = s.Keyrings(t.Context())
		}()
	}
	wg.Wait()

	for i := range results {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if results[i] == nil || results[i].User == nil {
			t.Fatalf("caller %d: expected an unlocked user keyring", i)
		}
	}
	if results[0] != results[1] {
		t.Fatal("both callers must observe the same cached keyrings")
	}
	if got := reloginCalls.Load(); got != 1 {
		t.Fatalf("relogin ran %d times, want exactly 1 under single-flight", got)
	}
}

// TestKeyringsScopeSelfHealNonScopeRetryErrorLeavesLatch proves a transient
// failure on the post-relogin retry-unlock is surfaced verbatim and does NOT
// spend the one-attempt latch: a relogin succeeds, but the retry GetSalts hits a
// non-scope error (500). The caller gets that error (not the scope sentinel) and
// the latch stays reset so a genuine 9100 after the blip can still self-heal.
func TestKeyringsScopeSelfHealNonScopeRetryErrorLeavesLatch(t *testing.T) {
	const pass = "mailbox-pw"
	current := keyring.KeyFetcher(scopeDeniedFetcher{})
	s := &Session{
		kc:  &credKC{creds: keychain.Creds{Password: pass}},
		raw: newRawClient("http://invalid.test", nil),
	}
	s.keyFetcher = func(context.Context) (keyring.KeyFetcher, error) { return current, nil }
	s.reloginScope = func(context.Context) (bool, error) {
		current = transientFetcher{} // relogin "worked" but retry-unlock now 500s
		return true, nil
	}

	_, err := s.Keyrings(t.Context())
	if err == nil {
		t.Fatal("expected the transient retry-unlock error to surface")
	}
	if errors.Is(err, proterr.ErrKeyringUnlockScope) {
		t.Fatalf("transient error must not be reported as a scope denial, got %v", err)
	}
	if s.scopeReloginSpent() {
		t.Fatal("a transient retry failure must not spend the self-heal latch")
	}
}

// TestKeyringsScopeSelfHealNonScopeRetryStaysBounded proves the un-latched
// non-scope retry path does NOT reopen an unbounded-relogin loop: once a relogin
// upgrades the session, the fetcher it installed is what the NEXT cache-miss
// unlocks against. A persistently failing salts endpoint (500) therefore surfaces
// at the second call's first unlock — which is no longer a 9100 scope denial — so
// healScopeDenial is never re-entered and the relogin runs exactly once across
// both calls, even though the latch was deliberately left reset. This pins the
// bound that makes leaving the latch intact on a non-scope error safe.
func TestKeyringsScopeSelfHealNonScopeRetryStaysBounded(t *testing.T) {
	const pass = "mailbox-pw"
	current := keyring.KeyFetcher(scopeDeniedFetcher{})
	s := &Session{
		kc:  &credKC{creds: keychain.Creds{Password: pass}},
		raw: newRawClient("http://invalid.test", nil),
	}
	s.keyFetcher = func(context.Context) (keyring.KeyFetcher, error) { return current, nil }

	var reloginCalls atomic.Int32
	s.reloginScope = func(context.Context) (bool, error) {
		reloginCalls.Add(1)
		current = transientFetcher{} // scope upgraded, but salts now persistently 500s
		return true, nil
	}

	for call := 1; call <= 2; call++ {
		_, err := s.Keyrings(t.Context())
		if err == nil {
			t.Fatalf("call %d: expected the persistent non-scope error to surface", call)
		}
		if errors.Is(err, proterr.ErrKeyringUnlockScope) {
			t.Fatalf("call %d: non-scope error must not be reported as a scope denial, got %v", call, err)
		}
		if s.scopeReloginSpent() {
			t.Fatalf("call %d: a non-scope retry failure must not spend the self-heal latch", call)
		}
	}
	if got := reloginCalls.Load(); got != 1 {
		t.Fatalf("relogin ran %d times across two calls, want exactly 1 (no relogin loop)", got)
	}
}

// TestKeyringsScopeSelfHealSurfacesCaptchaError proves Keyrings surfaces a
// CAPTCHA challenge from the self-heal relogin instead of the generic scope
// hint: when reloginForScope reports (false, captcha), the caller gets the
// captcha error (the actionable verification step) and the latch holds.
func TestKeyringsScopeSelfHealSurfacesCaptchaError(t *testing.T) {
	captcha := &proterr.Error{Code: "proton/captcha", Message: "Human verification required."}
	s := newSessionWithFetcher(
		&credKC{creds: keychain.Creds{Password: "pw"}},
		scopeDeniedFetcher{},
	)
	s.reloginScope = func(context.Context) (bool, error) { return false, captcha }

	_, err := s.Keyrings(t.Context())
	if !errors.Is(err, captcha) {
		t.Fatalf("want the CAPTCHA challenge surfaced, got %v", err)
	}
	if errors.Is(err, proterr.ErrKeyringUnlockScope) {
		t.Fatal("CAPTCHA must replace the generic scope hint, not fall through to it")
	}
	if !s.scopeReloginSpent() {
		t.Fatal("latch must hold after a CAPTCHA-blocked self-heal")
	}
}

// TestReloginForScopeSurfacesCaptcha drives the production reloginForScope body
// (no seam): the relogin login returns Proton's human-verification challenge
// (Code 9001), and reloginForScope reports (false, captcha) so the caller can
// surface the actionable verification step instead of the generic scope hint.
func TestReloginForScopeSurfacesCaptcha(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"Code":9001,"Error":"Human verification required"}`))
	}))
	defer srv.Close()

	kc := &credKC{creds: keychain.Creds{Username: "u@example.test", Password: "pw"}}
	s := New(srv.URL, kc)

	relogged, challenge := s.reloginForScope(t.Context())
	if relogged {
		t.Fatal("relogin must not report success when Proton demands human verification")
	}
	var pe *proterr.Error
	if !errors.As(challenge, &pe) || pe.Code != "proton/captcha" {
		t.Fatalf("want a proton/captcha challenge surfaced, got %v", challenge)
	}
}

// TestLogoutResetsScopeReloginLatch proves the scope self-heal latch is cleared
// by an explicit Logout, so an operator who re-authenticates recovers a session
// that latched on an unfixable denial. Without the reset a latched session would
// stay un-healable until process restart.
func TestLogoutResetsScopeReloginLatch(t *testing.T) {
	s := &Session{kc: &credKC{}, raw: newRawClient("http://invalid.test", nil)}
	s.mu.Lock()
	s.scopeReloginExhausted = true
	s.mu.Unlock()

	if err := s.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if s.scopeReloginSpent() {
		t.Fatal("Logout must reset the scope self-heal latch")
	}
}

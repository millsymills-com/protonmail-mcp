package session

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

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
	s.reloginScope = func(context.Context) bool {
		reloginCalls++
		current = lockedFetcher(t, pass) // fresh full-scope session can unlock
		return true
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
	s.reloginScope = func(context.Context) bool { reloginCalls++; return true }

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

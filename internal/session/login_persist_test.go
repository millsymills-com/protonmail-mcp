package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	dbus "github.com/godbus/dbus/v5"
	"github.com/zalando/go-keyring"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

// failingStore wraps *keychain.Keychain so individual methods can fail
// deterministically; the go-keyring mock backend can't simulate that on its
// own (it fails all ops or none).
type failingStore struct {
	*keychain.Keychain
	failCreds   error
	failSession error
}

func (f *failingStore) SaveCreds(c keychain.Creds) error {
	if f.failCreds != nil {
		return f.failCreds
	}
	return f.Keychain.SaveCreds(c)
}

func (f *failingStore) SaveSession(s keychain.Session) error {
	if f.failSession != nil {
		return f.failSession
	}
	return f.Keychain.SaveSession(s)
}

func newSessionWithStore(kc Store) *Session {
	return &Session{kc: kc, raw: newRawClient("http://invalid.test", nil)}
}

func TestPersistLoginStateRollsBackOnSaveSessionFailure(t *testing.T) {
	keyring.MockInit()
	real := keychain.New()
	want := errors.New("simulated session save failure")
	store := &failingStore{Keychain: real, failSession: want}
	s := newSessionWithStore(store)

	creds := keychain.Creds{Username: "u@example.test", Password: "hunter2", TOTPSecret: "seed"}
	sess := keychain.Session{UID: "uid-1", AccessToken: "at-1", RefreshToken: "rt-1"}

	err := s.persistLoginState(creds, sess)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, want) {
		t.Fatalf("wrapped err lost original cause; got %v", err)
	}
	if !strings.Contains(err.Error(), "save session") {
		t.Fatalf("err missing op tag: %v", err)
	}

	if _, err := real.LoadCreds(); err == nil {
		t.Fatal("creds still in keychain after rollback")
	}
	if _, err := real.LoadSession(); err == nil {
		t.Fatal("session still in keychain after rollback")
	}
	if s.poisoned {
		t.Fatal("Session must NOT be poisoned when rollback's Clear succeeds")
	}
}

func TestPersistLoginStateRollsBackOnSaveCredsFailure(t *testing.T) {
	keyring.MockInit()
	real := keychain.New()
	if err := real.SaveSession(keychain.Session{UID: "stale", AccessToken: "stale", RefreshToken: "stale"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	want := errors.New("simulated creds save failure")
	store := &failingStore{Keychain: real, failCreds: want}
	s := newSessionWithStore(store)

	err := s.persistLoginState(
		keychain.Creds{Username: "u", Password: "p"},
		keychain.Session{UID: "uid-1", AccessToken: "at-1", RefreshToken: "rt-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, want) {
		t.Fatalf("lost cause: %v", err)
	}
	if !strings.Contains(err.Error(), "save creds") {
		t.Fatalf("err missing op tag: %v", err)
	}
	if _, err := real.LoadSession(); err == nil {
		t.Fatal("stale session not cleared on rollback")
	}
	if s.poisoned {
		t.Fatal("Session must NOT be poisoned when rollback's Clear succeeds")
	}
}

func TestPersistLoginStateSucceedsClean(t *testing.T) {
	keyring.MockInit()
	real := keychain.New()
	s := newSessionWithStore(real)
	creds := keychain.Creds{Username: "u@example.test", Password: "hunter2"}
	sess := keychain.Session{UID: "uid-1", AccessToken: "at-1", RefreshToken: "rt-1"}

	if err := s.persistLoginState(creds, sess); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if s.poisoned {
		t.Fatal("clean path must not poison the Session")
	}
	if s.persistDegraded || s.persistErrReason != "" {
		t.Fatalf("persistDegraded must be clear after success; got degraded=%v reason=%q",
			s.persistDegraded, s.persistErrReason)
	}
	gotCreds, err := real.LoadCreds()
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if gotCreds.Username != creds.Username || gotCreds.Password != creds.Password {
		t.Fatalf("creds = %+v, want %+v", gotCreds, creds)
	}
	gotSess, err := real.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if gotSess != sess {
		t.Fatalf("session = %+v, want %+v", gotSess, sess)
	}
}

func TestPersistLoginStateClearsDegradedFlag(t *testing.T) {
	keyring.MockInit()
	real := keychain.New()
	s := newSessionWithStore(real)
	s.persistDegraded = true
	s.persistErrReason = "disk full"

	creds := keychain.Creds{Username: "u@example.test", Password: "hunter2"}
	sess := keychain.Session{UID: "uid-2", AccessToken: "at-2", RefreshToken: "rt-2"}

	if err := s.persistLoginState(creds, sess); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if s.persistDegraded || s.persistErrReason != "" {
		t.Fatalf("persistDegraded not cleared by successful Login persist; got degraded=%v reason=%q",
			s.persistDegraded, s.persistErrReason)
	}
}

// clearFailingStore satisfies Store but fails SaveSession and Clear
// so we can confirm both the rollback secondary failure surfaces and that
// the Session marks itself poisoned.
type clearFailingStore struct {
	*keychain.Keychain
	saveErr  error
	clearErr error
}

func (c *clearFailingStore) SaveSession(_ keychain.Session) error { return c.saveErr }
func (c *clearFailingStore) Clear() error                         { return c.clearErr }

func TestRollbackSurfacesSecondaryFailureAndPoisons(t *testing.T) {
	keyring.MockInit()
	savePrimary := errors.New("save fail")
	clearSecondary := errors.New("clear fail")
	store := &clearFailingStore{
		Keychain: keychain.New(),
		saveErr:  savePrimary,
		clearErr: clearSecondary,
	}
	s := newSessionWithStore(store)

	err := s.persistLoginState(
		keychain.Creds{Username: "u", Password: "p"},
		keychain.Session{UID: "uid", AccessToken: "at", RefreshToken: "rt"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, savePrimary) {
		t.Fatalf("primary cause unreachable via errors.Is: %v", err)
	}
	if !errors.Is(err, clearSecondary) {
		t.Fatalf("secondary cause unreachable via errors.Is: %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "save session") {
		t.Fatalf("missing primary op tag: %v", err)
	}
	if !strings.Contains(msg, "login rollback") {
		t.Fatalf("missing rollback op tag: %v", err)
	}
	if !strings.Contains(msg, "protonmail-mcp logout") {
		t.Fatalf("missing recovery hint: %v", err)
	}
	if !s.poisoned {
		t.Fatal("Session must be marked poisoned when rollback's Clear fails")
	}
}

// clearTrackingStore records whether Clear was called, so the backend-
// unavailable rollback path can assert it skips the pointless Clear against a
// dead backend.
type clearTrackingStore struct {
	*keychain.Keychain
	saveErr      error
	clearedCount int
}

func (c *clearTrackingStore) SaveCreds(_ keychain.Creds) error { return c.saveErr }
func (c *clearTrackingStore) Clear() error {
	c.clearedCount++
	return c.Keychain.Clear()
}

func TestRollbackBackendUnavailableSkipsCleanupAndDoesNotPoison(t *testing.T) {
	keyring.MockInit()
	// A first write failing with a ServiceUnknown dbus error is the headless
	// host case: nothing was persisted, so the keychain is NOT inconsistent.
	cause := dbus.Error{
		Name: "org.freedesktop.DBus.Error.ServiceUnknown",
		Body: []any{"The name org.freedesktop.secrets was not provided by any .service files"},
	}
	store := &clearTrackingStore{Keychain: keychain.New(), saveErr: cause}
	s := newSessionWithStore(store)

	err := s.persistLoginState(
		keychain.Creds{Username: "u", Password: "p"},
		keychain.Session{UID: "uid", AccessToken: "at", RefreshToken: "rt"})
	if err == nil {
		t.Fatal("expected error")
	}
	// dbus.Error is non-comparable (slice Body), so errors.Is can't match it by
	// identity; assert the wrap stays errors.As-recoverable instead.
	var gotDBus dbus.Error
	if !errors.As(err, &gotDBus) || gotDBus.Name != cause.Name {
		t.Fatalf("primary cause unreachable via errors.As: %v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "inconsistent") {
		t.Fatalf("backend-unavailable error must not claim the keychain is inconsistent: %v", err)
	}
	if strings.Contains(msg, "protonmail-mcp logout") {
		t.Fatalf("backend-unavailable error must not advise running logout against the dead backend: %v", err)
	}
	if !strings.Contains(msg, "nothing was persisted") {
		t.Fatalf("backend-unavailable error must say nothing was persisted: %v", err)
	}
	if s.poisoned {
		t.Fatal("backend-unavailable rollback must NOT poison the Session")
	}
	if store.clearedCount != 0 {
		t.Fatalf("backend-unavailable rollback must skip Clear; got %d calls", store.clearedCount)
	}
}

func TestPoisonedClientShortCircuits(t *testing.T) {
	keyring.MockInit()
	// Seed a stale session entry the way a same-process failed-rollback would
	// leave it: keychain holds a session, but the Session was marked poisoned.
	real := keychain.New()
	if err := real.SaveSession(keychain.Session{UID: "stale", AccessToken: "at", RefreshToken: "rt"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s, err := NewForTesting("http://invalid.test", keychain.Session{UID: "stale", AccessToken: "at", RefreshToken: "rt"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	// Force the cold-start branch: drop the in-memory client/auth and poison.
	s.client = nil
	s.poisoned = true

	_, err = s.Client(context.Background())
	if err == nil {
		t.Fatal("expected ErrSessionInconsistent, got nil")
	}
	if !errors.Is(err, ErrSessionInconsistent) {
		t.Fatalf("err = %v, want ErrSessionInconsistent", err)
	}
}

func TestLogoutClearsPoisonOnSuccess(t *testing.T) {
	keyring.MockInit()
	s, err := NewForTesting("http://invalid.test", keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	s.poisoned = true

	if err := s.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if s.poisoned {
		t.Fatal("Logout with successful Clear must reset poisoned flag")
	}
}

func TestLogoutLeavesPoisonOnClearFailure(t *testing.T) {
	keyring.MockInit()
	clearErr := errors.New("clear still failing")
	store := &clearFailingStore{Keychain: keychain.New(), clearErr: clearErr}
	s := newSessionWithStore(store)
	s.poisoned = true

	err := s.Logout()
	if err == nil {
		t.Fatal("expected Logout to surface Clear error")
	}
	if !errors.Is(err, clearErr) {
		t.Fatalf("wrong wrapped err: %v", err)
	}
	if !s.poisoned {
		t.Fatal("Logout with failed Clear must leave poisoned flag set")
	}
}

func TestErrMailboxPasswordRequiredIsDistinct(t *testing.T) {
	// Guards the sentinel used by the CLI to branch into a mailbox-password
	// prompt instead of treating it as a generic auth failure.
	if !errors.Is(ErrMailboxPasswordRequired, ErrMailboxPasswordRequired) {
		t.Fatal("sentinel must match itself")
	}
	if errors.Is(ErrMailboxPasswordRequired, ErrTOTPRequired) {
		t.Fatal("mailbox-password and TOTP sentinels must be distinct")
	}
}

func TestChooseMailboxPassword(t *testing.T) {
	tests := []struct {
		name     string
		mode     proton.PasswordMode
		supplied string
		want     string
		wantErr  error
	}{
		{"two-password requires value", proton.TwoPasswordMode, "", "", ErrMailboxPasswordRequired},
		{"two-password keeps supplied", proton.TwoPasswordMode, "mbox", "mbox", nil},
		{"one-password forces empty", proton.OnePasswordMode, "ignored", "", nil},
		{"unknown mode errors", proton.PasswordMode(0), "x", "", nil}, // expects a non-nil error; see check below
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := chooseMailboxPassword(tc.mode, tc.supplied)
			if tc.name == "unknown mode errors" {
				if err == nil {
					t.Fatal("expected error for unknown mode")
				}
				return
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

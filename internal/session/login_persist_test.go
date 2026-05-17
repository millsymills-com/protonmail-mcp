package session

import (
	"context"
	"errors"
	"strings"
	"testing"

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

func newSessionWithStore(kc keychainStore) *Session {
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

// clearFailingStore satisfies keychainStore but fails SaveSession and Clear
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

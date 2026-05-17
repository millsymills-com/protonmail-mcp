package session

import (
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

func TestPersistLoginStateRollsBackOnSaveSessionFailure(t *testing.T) {
	keyring.MockInit()
	real := keychain.New()
	want := errors.New("simulated session save failure")
	store := &failingStore{Keychain: real, failSession: want}

	creds := keychain.Creds{Username: "u@example.test", Password: "hunter2", TOTPSecret: "seed"}
	sess := keychain.Session{UID: "uid-1", AccessToken: "at-1", RefreshToken: "rt-1"}

	err := persistLoginState(store, creds, sess)
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
}

func TestPersistLoginStateRollsBackOnSaveCredsFailure(t *testing.T) {
	keyring.MockInit()
	real := keychain.New()
	// Pre-seed a stale session entry so we can verify Clear wipes it.
	if err := real.SaveSession(keychain.Session{UID: "stale", AccessToken: "stale", RefreshToken: "stale"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	want := errors.New("simulated creds save failure")
	store := &failingStore{Keychain: real, failCreds: want}

	err := persistLoginState(store,
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
}

func TestPersistLoginStateSucceedsClean(t *testing.T) {
	keyring.MockInit()
	real := keychain.New()
	creds := keychain.Creds{Username: "u@example.test", Password: "hunter2"}
	sess := keychain.Session{UID: "uid-1", AccessToken: "at-1", RefreshToken: "rt-1"}

	if err := persistLoginState(real, creds, sess); err != nil {
		t.Fatalf("unexpected: %v", err)
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

// clearFailingStore satisfies keychainStore but fails both SaveSession and Clear,
// so we can confirm the rollback secondary failure is surfaced.
type clearFailingStore struct {
	*keychain.Keychain
	saveErr  error
	clearErr error
}

func (c *clearFailingStore) SaveSession(_ keychain.Session) error { return c.saveErr }
func (c *clearFailingStore) Clear() error                         { return c.clearErr }

func TestRollbackSurfacesSecondaryFailure(t *testing.T) {
	keyring.MockInit()
	store := &clearFailingStore{Keychain: keychain.New()}

	savePrimary := errors.New("save fail")
	clearSecondary := errors.New("clear fail")
	store.saveErr = savePrimary
	store.clearErr = clearSecondary

	err := persistLoginState(store,
		keychain.Creds{Username: "u", Password: "p"},
		keychain.Session{UID: "uid", AccessToken: "at", RefreshToken: "rt"})
	if err == nil {
		t.Fatal("expected error")
	}
	// errors.Is must reach both the primary cause and the secondary failure
	// through the errors.Join wrapper. If this regresses, callers can no
	// longer typed-check the cause.
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
}

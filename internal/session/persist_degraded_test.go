package session_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/zalando/go-keyring"
)

func TestStatusZeroOnFreshSession(t *testing.T) {
	keyring.MockInit()
	s, err := session.NewForTesting("http://invalid.test",
		keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	defer func() { _ = s.Logout() }()

	got := s.Status()
	if got.PersistDegraded || got.PersistError != "" {
		t.Fatalf("fresh Status() = %+v, want zero", got)
	}
}

func TestSetPersistDegradedForTestRoundTrip(t *testing.T) {
	keyring.MockInit()
	s, err := session.NewForTesting("http://invalid.test",
		keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	defer func() { _ = s.Logout() }()

	s.SetPersistDegradedForTest("disk full")
	got := s.Status()
	if !got.PersistDegraded || got.PersistError != "disk full" {
		t.Fatalf("after set: Status() = %+v", got)
	}

	s.SetPersistDegradedForTest("")
	got = s.Status()
	if got.PersistDegraded || got.PersistError != "" {
		t.Fatalf("after clear: Status() = %+v", got)
	}
}

// fakeKC satisfies the unexported keychainStore method set. Go's
// assignability rules permit external packages to pass values that
// satisfy unexported interfaces, even though the interface type itself
// is not visible.
type fakeKC struct {
	seed       keychain.Session
	saveErr    error
	loadErr    error
	clearErr   error
	saveCalled int
}

func (f *fakeKC) SaveCreds(keychain.Creds) error     { return nil }
func (f *fakeKC) LoadCreds() (keychain.Creds, error) { return keychain.Creds{}, nil }
func (f *fakeKC) SaveSession(s keychain.Session) error {
	f.saveCalled++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.seed = s
	return nil
}
func (f *fakeKC) LoadSession() (keychain.Session, error) {
	if f.loadErr != nil {
		return keychain.Session{}, f.loadErr
	}
	return f.seed, nil
}
func (f *fakeKC) Clear() error { f.seed = keychain.Session{}; return f.clearErr }

func TestStatusPersistDegradedOnRotation(t *testing.T) {
	kc := &fakeKC{
		seed:    keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"},
		saveErr: errors.New("save session: keychain locked"),
	}
	s := session.New("http://invalid.test", kc)

	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "b", RefreshToken: "r2"})

	got := s.Status()
	if !got.PersistDegraded {
		t.Fatalf("PersistDegraded = false, want true")
	}
	if got.PersistError != "save session: keychain locked" {
		t.Fatalf("PersistError = %q, want %q", got.PersistError, "save session: keychain locked")
	}
	cur := s.CurrentForTest()
	if cur.AccessToken != "b" || cur.RefreshToken != "r2" {
		t.Fatalf("in-memory tokens not rotated despite persist failure: %+v", cur)
	}
}

func TestStatusPersistDegradedClearsOnNextRotationSuccess(t *testing.T) {
	kc := &fakeKC{
		seed:    keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"},
		saveErr: errors.New("save session: keychain locked"),
	}
	s := session.New("http://invalid.test", kc)

	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "b", RefreshToken: "r2"})
	if !s.Status().PersistDegraded {
		t.Fatalf("setup: expected PersistDegraded=true")
	}

	kc.saveErr = nil
	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "c", RefreshToken: "r3"})

	got := s.Status()
	if got.PersistDegraded || got.PersistError != "" {
		t.Fatalf("Status() = %+v, want zero after successful rotation", got)
	}
}

func TestStatusPersistDegradedClearsOnLogout(t *testing.T) {
	kc := &fakeKC{
		seed:    keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"},
		saveErr: errors.New("save session: keychain locked"),
	}
	s := session.New("http://invalid.test", kc)

	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "b", RefreshToken: "r2"})
	if !s.Status().PersistDegraded {
		t.Fatalf("setup: expected PersistDegraded=true")
	}

	if err := s.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	got := s.Status()
	if got.PersistDegraded || got.PersistError != "" {
		t.Fatalf("Status() = %+v, want zero after Logout", got)
	}
}

// TestStatusPersistDegradedSurvivesFailedLogout guards the invariant that
// when Logout's kc.Clear() fails, neither poisoned nor persistDegraded
// gets cleared — the session is still inconsistent on disk and callers
// must be able to observe that.
func TestStatusPersistDegradedSurvivesFailedLogout(t *testing.T) {
	kc := &fakeKC{
		seed:     keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"},
		saveErr:  errors.New("save session: keychain locked"),
		clearErr: errors.New("clear: keychain locked"),
	}
	s := session.New("http://invalid.test", kc)

	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "b", RefreshToken: "r2"})
	if !s.Status().PersistDegraded {
		t.Fatalf("setup: expected PersistDegraded=true")
	}

	if err := s.Logout(); err == nil {
		t.Fatalf("Logout: expected error from failing Clear(), got nil")
	}
	got := s.Status()
	if !got.PersistDegraded {
		t.Fatalf("PersistDegraded cleared after failed Logout: %+v", got)
	}
	if got.PersistError != "save session: keychain locked" {
		t.Fatalf("PersistError = %q, want %q", got.PersistError, "save session: keychain locked")
	}
}

func TestStatusPersistDegradedOnColdStart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v4/refresh" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		_, _ = w.Write([]byte(`{
			"Code": 1000,
			"AccessToken": "fresh-access",
			"RefreshToken": "fresh-refresh",
			"UID": "u",
			"Scope": "self",
			"TokenType": "Bearer",
			"ExpiresIn": 86400
		}`))
	}))
	defer srv.Close()

	kc := &fakeKC{
		seed:    keychain.Session{UID: "u", AccessToken: "stale-access", RefreshToken: "stale-refresh"},
		saveErr: errors.New("save session: keychain locked"),
	}
	s := session.New(srv.URL, kc)

	if _, err := s.Client(context.Background()); err != nil {
		t.Fatalf("Client: %v", err)
	}

	got := s.Status()
	if !got.PersistDegraded {
		t.Fatalf("PersistDegraded = false, want true")
	}
	if got.PersistError != "save session: keychain locked" {
		t.Fatalf("PersistError = %q, want %q", got.PersistError, "save session: keychain locked")
	}

	// Sanity: the in-memory client got the rotated tokens even though persist failed.
	cur := s.CurrentForTest()
	if cur.AccessToken != "fresh-access" || cur.RefreshToken != "fresh-refresh" {
		t.Fatalf("in-memory tokens not rotated: %+v", cur)
	}
}

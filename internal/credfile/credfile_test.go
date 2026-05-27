package credfile

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

func newTmp(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestCredsRoundTrip(t *testing.T) {
	s := newTmp(t)
	in := keychain.Creds{Username: "u@example.test", Password: "pw", TOTPSecret: "otpauth://x"}
	if err := s.SaveCreds(in); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	got, err := s.LoadCreds()
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if got != in {
		t.Fatalf("got %+v want %+v", got, in)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	s := newTmp(t)
	in := keychain.Session{UID: "uid", AccessToken: "at", RefreshToken: "rt"}
	if err := s.SaveSession(in); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, err := s.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got != in {
		t.Fatalf("got %+v want %+v", got, in)
	}
}

func TestCredsAndSessionCoexist(t *testing.T) {
	s := newTmp(t)
	if err := s.SaveCreds(keychain.Creds{Username: "u"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSession(keychain.Session{UID: "uid"}); err != nil {
		t.Fatal(err)
	}
	c, err := s.LoadCreds()
	if err != nil || c.Username != "u" {
		t.Fatalf("creds clobbered: %+v err=%v", c, err)
	}
	sess, err := s.LoadSession()
	if err != nil || sess.UID != "uid" {
		t.Fatalf("session clobbered: %+v err=%v", sess, err)
	}
}

func TestPermissions(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "sub")
	s, err := New(inner)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSession(keychain.Session{UID: "x"}); err != nil {
		t.Fatal(err)
	}
	di, err := os.Stat(inner)
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != fs.FileMode(0o700) {
		t.Fatalf("dir mode = %o want 700", di.Mode().Perm())
	}
	fi, err := os.Stat(filepath.Join(inner, "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != fs.FileMode(0o600) {
		t.Fatalf("file mode = %o want 600", fi.Mode().Perm())
	}
}

func TestLoadAbsentIsNotFound(t *testing.T) {
	s := newTmp(t)
	if _, err := s.LoadSession(); err == nil {
		t.Fatal("expected error loading absent session")
	}
	if _, err := s.LoadCreds(); err == nil {
		t.Fatal("expected error loading absent creds")
	}
}

func TestClearRemovesFile(t *testing.T) {
	s := newTmp(t)
	if err := s.SaveSession(keychain.Session{UID: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := s.LoadSession(); err == nil {
		t.Fatal("expected not-found after Clear")
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear on absent should be nil, got %v", err)
	}
}

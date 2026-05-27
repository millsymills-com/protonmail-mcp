package credfile

import (
	"errors"
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
	if serr := s.SaveSession(keychain.Session{UID: "x"}); serr != nil {
		t.Fatal(serr)
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
	if _, err := s.LoadSession(); !errors.Is(err, keychain.ErrNotFound) {
		t.Fatalf("LoadSession: want keychain.ErrNotFound, got %v", err)
	}
	if _, err := s.LoadCreds(); !errors.Is(err, keychain.ErrNotFound) {
		t.Fatalf("LoadCreds: want keychain.ErrNotFound, got %v", err)
	}
}

// A file holding only one bundle must report the absent bundle as ErrNotFound
// (matching the keychain backend), not as a zero-valued success — otherwise
// session.Client would refresh with an empty UID and get an opaque API error.
func TestLoadEmptyBundleIsNotFound(t *testing.T) {
	s := newTmp(t)
	if err := s.SaveCreds(keychain.Creds{Username: "u@example.test", Password: "pw"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadSession(); !errors.Is(err, keychain.ErrNotFound) {
		t.Fatalf("LoadSession with only creds stored: want keychain.ErrNotFound, got %v", err)
	}
	if _, err := s.LoadCreds(); err != nil {
		t.Fatalf("LoadCreds with creds stored: %v", err)
	}
}

func TestCorruptFileLoadSurfacesError(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		load func() error
	}{
		{"LoadSession", func() error { _, e := s.LoadSession(); return e }},
		{"LoadCreds", func() error { _, e := s.LoadCreds(); return e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.load()
			if err == nil {
				t.Fatal("expected parse error on corrupt file")
			}
			// A parse failure is not "not logged in" — it must not be mistaken
			// for ErrNotFound, or session.Client would tell the user to log in
			// instead of naming the corrupt file.
			if errors.Is(err, keychain.ErrNotFound) {
				t.Fatalf("corrupt file misreported as not-found: %v", err)
			}
		})
	}
}

func TestCorruptFileSaveDoesNotClobber(t *testing.T) {
	garbage := []byte("{not json")
	for _, tc := range []struct {
		name string
		save func(*Store) error
	}{
		{"SaveCreds", func(s *Store) error { return s.SaveCreds(keychain.Creds{Username: "u"}) }},
		{"SaveSession", func(s *Store) error { return s.SaveSession(keychain.Session{UID: "x"}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			s, err := New(dir)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "credentials.json")
			if werr := os.WriteFile(path, garbage, 0o600); werr != nil {
				t.Fatal(werr)
			}
			if serr := tc.save(s); serr == nil {
				t.Fatal("expected save to surface parse error on corrupt file")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(garbage) {
				t.Fatalf("corrupt file was overwritten: got %q want %q", got, garbage)
			}
		})
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

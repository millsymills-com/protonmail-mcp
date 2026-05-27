package credfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

const fileName = "credentials.json"

// Store is a file-backed credential store: both bundles in one 0600 JSON
// document under dir (0700). It satisfies the credential-store interface
// consumed by package session.
type Store struct{ dir string }

type doc struct {
	Creds   keychain.Creds   `json:"creds"`
	Session keychain.Session `json:"session"`
}

// New returns a Store rooted at dir. The directory is created lazily on first
// write, so construction has no side effects.
func New(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("credfile: empty state dir")
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path() string { return filepath.Join(s.dir, fileName) }

func (s *Store) load() (doc, error) {
	b, err := os.ReadFile(s.path())
	if err != nil {
		return doc{}, err
	}
	var d doc
	if err := json.Unmarshal(b, &d); err != nil {
		return doc{}, fmt.Errorf("credfile parse %s: %w", s.path(), err)
	}
	return d, nil
}

func (s *Store) save(d doc) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("credfile mkdir %s: %w", s.dir, err)
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return fmt.Errorf("credfile chmod dir %s: %w", s.dir, err)
	}
	b, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("credfile marshal: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, fileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("credfile temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once renamed; cleans up on any error path
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("credfile chmod: %w", err)
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("credfile write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("credfile sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("credfile close: %w", err)
	}
	if err := os.Rename(tmpName, s.path()); err != nil {
		return fmt.Errorf("credfile rename: %w", err)
	}
	return nil
}

// merge loads the current doc, applies fn, and saves. An absent file starts
// from an empty doc; a parse/permission error on an existing file is surfaced
// rather than silently overwritten.
func (s *Store) merge(fn func(*doc)) error {
	d, err := s.load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fn(&d)
	return s.save(d)
}

func (s *Store) SaveCreds(c keychain.Creds) error {
	return s.merge(func(d *doc) { d.Creds = c })
}

func (s *Store) LoadCreds() (keychain.Creds, error) {
	d, err := s.load()
	if errors.Is(err, os.ErrNotExist) || (err == nil && d.Creds.Username == "") {
		return keychain.Creds{}, fmt.Errorf("credfile load creds %s: %w", s.path(), keychain.ErrNotFound)
	}
	if err != nil {
		return keychain.Creds{}, err
	}
	return d.Creds, nil
}

func (s *Store) SaveSession(sess keychain.Session) error {
	return s.merge(func(d *doc) { d.Session = sess })
}

func (s *Store) LoadSession() (keychain.Session, error) {
	d, err := s.load()
	if errors.Is(err, os.ErrNotExist) || (err == nil && d.Session.UID == "") {
		return keychain.Session{}, fmt.Errorf("credfile load session %s: %w", s.path(), keychain.ErrNotFound)
	}
	if err != nil {
		return keychain.Session{}, err
	}
	return d.Session, nil
}

// Clear removes the credentials file. An absent file is not an error.
func (s *Store) Clear() error {
	if err := os.Remove(s.path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("credfile clear: %w", err)
	}
	return nil
}

package credfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
)

const (
	fileName = "credentials.json"
	lockName = "credentials.lock"
)

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
	if !flockSupported {
		return nil, errors.New(
			"credfile: the file credential backend requires advisory file locking, " +
				"unsupported on this OS (use the keychain backend)")
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

// merge loads the current doc, applies fn, and saves under an exclusive file
// lock so two concurrent writers (e.g. the daemon's token rotation and a
// manual `status` invocation in another process) can't lose an update via the
// read-modify-write. An absent file starts from an empty doc; a parse or
// permission error on an existing file is surfaced rather than overwritten.
func (s *Store) merge(fn func(*doc)) error {
	if err := s.prepareDir(); err != nil {
		return err
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	d, err := s.load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fn(&d)
	return s.save(d)
}

// prepareDir creates the state dir (0700) if absent and makes it safe to store
// secrets in. It runs before lock() and save() so a hostile dir never receives
// even the sidecar lock file. A symlinked dir or one owned by another user is
// rejected; these are checked before the chmod so we never follow a symlink to
// an attacker's target or tamper with a dir we do not own. Group/other
// permission bits on our own dir are tightened to 0700 rather than rejected.
func (s *Store) prepareDir() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("credfile mkdir %s: %w", s.dir, err)
	}
	if err := ensureDirSafe(s.dir); err != nil {
		return err
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return fmt.Errorf("credfile chmod dir %s: %w", s.dir, err)
	}
	return nil
}

// lock acquires an exclusive advisory lock on a sidecar lock file, serializing
// merges across goroutines and processes that share the state dir. The
// returned func releases the lock. The caller must have run prepareDir first.
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.dir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("credfile lock open: %w", err)
	}
	if err := flockExclusive(f.Fd()); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("credfile flock: %w", err)
	}
	return func() {
		_ = flockUnlock(f.Fd())
		_ = f.Close()
	}, nil
}

// ensureDirSafe refuses to write secrets into a state dir that is a symlink (an
// attacker could swap the target) or is owned by another user (we cannot secure
// it). It is called before prepareDir's chmod so neither check follows a symlink
// nor races a chmod. Group/other permission bits are handled by the caller's
// chmod, not here.
func ensureDirSafe(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("credfile stat dir %s: %w", dir, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("credfile state dir %s is a symlink; refusing to store credentials there", dir)
	}
	if !ownedByCurrentUser(fi) {
		return fmt.Errorf("credfile state dir %s is not owned by the current user", dir)
	}
	return nil
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

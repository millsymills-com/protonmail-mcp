package keychain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// FileStore persists Creds and Session bundles to a single JSON file on disk
// with mode 0600. Used when the macOS Keychain is unreachable from the
// calling shell context (e.g. Claude Code's MCP runner, which inherits a
// process tree without a SecurityAgent connection and thus gets
// errSecInteractionNotAllowed on every keychain write).
//
// Threat model: file-based storage with 0600 mode relies on filesystem
// permissions. On a single-user Mac this is roughly comparable to a generic
// keychain item, which is also protected by the user's login secret at rest;
// the keychain has the edge because items are encrypted with a key derived
// from the login password and unlocked on first sign-in, while a 0600 file
// is readable by anyone with root or the user's UID once mounted. Treat
// this backend as opt-in for contexts where the keychain isn't an option.
//
// Layout: ~/.config/protonmail-mcp/secrets.json. Both Creds and Session
// live in the same file so a single fsync gives an atomic-ish write.
type FileStore struct {
	mu   sync.Mutex
	path string
}

type fileBundle struct {
	Creds   *Creds   `json:"creds,omitempty"`
	Session *Session `json:"session,omitempty"`
}

// NewFileStore returns a FileStore rooted at ~/.config/protonmail-mcp.
// The directory is created lazily on first write; reads of a missing file
// return zero-value bundles with a not-found error so callers can branch
// on errors.Is(err, ErrFileStoreEmpty).
func NewFileStore() (*FileStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("file store: resolve $HOME: %w", err)
	}
	return &FileStore{
		path: filepath.Join(home, ".config", "protonmail-mcp", "secrets.json"),
	}, nil
}

// ErrFileStoreEmpty signals that the on-disk bundle is missing or has no
// entry for the requested field. Callers should treat it like
// keyring.ErrNotFound for parity with the Keychain backend.
var ErrFileStoreEmpty = errors.New("file store: empty")

func (f *FileStore) load() (fileBundle, error) {
	b, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fileBundle{}, ErrFileStoreEmpty
		}
		return fileBundle{}, fmt.Errorf("file store: read %s: %w", f.path, err)
	}
	var bundle fileBundle
	if err := json.Unmarshal(b, &bundle); err != nil {
		return fileBundle{}, fmt.Errorf("file store: parse %s: %w", f.path, err)
	}
	return bundle, nil
}

func (f *FileStore) save(bundle fileBundle) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return fmt.Errorf("file store: mkdir %s: %w", filepath.Dir(f.path), err)
	}
	b, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("file store: marshal: %w", err)
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("file store: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("file store: rename %s -> %s: %w", tmp, f.path, err)
	}
	return nil
}

func (f *FileStore) SaveCreds(c Creds) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	bundle, err := f.load()
	if err != nil && !errors.Is(err, ErrFileStoreEmpty) {
		return err
	}
	bundle.Creds = &c
	return f.save(bundle)
}

func (f *FileStore) LoadCreds() (Creds, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	bundle, err := f.load()
	if err != nil {
		return Creds{}, err
	}
	if bundle.Creds == nil {
		return Creds{}, ErrFileStoreEmpty
	}
	return *bundle.Creds, nil
}

func (f *FileStore) SaveSession(s Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	bundle, err := f.load()
	if err != nil && !errors.Is(err, ErrFileStoreEmpty) {
		return err
	}
	bundle.Session = &s
	return f.save(bundle)
}

func (f *FileStore) LoadSession() (Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	bundle, err := f.load()
	if err != nil {
		return Session{}, err
	}
	if bundle.Session == nil {
		return Session{}, ErrFileStoreEmpty
	}
	return *bundle.Session, nil
}

func (f *FileStore) Clear() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.Remove(f.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("file store: remove %s: %w", f.path, err)
	}
	return nil
}

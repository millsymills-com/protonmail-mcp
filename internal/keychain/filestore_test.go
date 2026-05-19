package keychain

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *FileStore {
	t.Helper()
	dir := t.TempDir()
	return &FileStore{path: filepath.Join(dir, "secrets.json")}
}

func TestFileStoreRoundTrip(t *testing.T) {
	store := newTestStore(t)

	creds := Creds{Username: "a@b.c", Password: "hunter2", TOTPSecret: "JBSW"}
	if err := store.SaveCreds(creds); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	got, err := store.LoadCreds()
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if got != creds {
		t.Fatalf("creds round trip mismatch: got %+v want %+v", got, creds)
	}

	sess := Session{UID: "u", AccessToken: "a", RefreshToken: "r"}
	if err := store.SaveSession(sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	gotS, err := store.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if gotS != sess {
		t.Fatalf("session round trip mismatch: got %+v want %+v", gotS, sess)
	}

	// Save order independence: writing session after creds shouldn't drop creds.
	gotC2, err := store.LoadCreds()
	if err != nil {
		t.Fatalf("LoadCreds (post-session): %v", err)
	}
	if gotC2 != creds {
		t.Fatalf("creds dropped after SaveSession: got %+v", gotC2)
	}
}

func TestFileStoreLoadMissing(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.LoadCreds(); !errors.Is(err, ErrFileStoreEmpty) {
		t.Fatalf("expected ErrFileStoreEmpty, got %v", err)
	}
	if _, err := store.LoadSession(); !errors.Is(err, ErrFileStoreEmpty) {
		t.Fatalf("expected ErrFileStoreEmpty, got %v", err)
	}
}

func TestFileStoreClear(t *testing.T) {
	store := newTestStore(t)
	if err := store.SaveCreds(Creds{Username: "a@b.c"}); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(store.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected file removed, stat err = %v", err)
	}
	// Clear on already-cleared store is a no-op, not an error.
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear on empty: %v", err)
	}
}

func TestFileStoreMode0600(t *testing.T) {
	store := newTestStore(t)
	if err := store.SaveCreds(Creds{Username: "a@b.c"}); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected mode 0600, got %#o", perm)
	}
}

func TestFileStoreSaveCredsClearTotp(t *testing.T) {
	store := newTestStore(t)
	first := Creds{Username: "a@b.c", Password: "p", TOTPSecret: "JBSW"}
	if err := store.SaveCreds(first); err != nil {
		t.Fatalf("SaveCreds first: %v", err)
	}
	second := Creds{Username: "a@b.c", Password: "p"} // no totp
	if err := store.SaveCreds(second); err != nil {
		t.Fatalf("SaveCreds second: %v", err)
	}
	got, err := store.LoadCreds()
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if got.TOTPSecret != "" {
		t.Fatalf("expected TOTPSecret cleared on overwrite, got %q", got.TOTPSecret)
	}
}

func TestNewFromEnvDispatch(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_STORAGE", "file")
	got, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv(file): %v", err)
	}
	if _, ok := got.(*FileStore); !ok {
		t.Fatalf("PROTONMAIL_MCP_STORAGE=file should yield *FileStore, got %T", got)
	}

	t.Setenv("PROTONMAIL_MCP_STORAGE", "")
	got, err = NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv(default): %v", err)
	}
	if _, ok := got.(*Keychain); !ok {
		t.Fatalf("default should yield *Keychain, got %T", got)
	}
}

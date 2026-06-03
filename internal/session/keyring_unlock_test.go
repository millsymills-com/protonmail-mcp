package session

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	srp "github.com/ProtonMail/go-srp"
	"github.com/ProtonMail/gopenpgp/v2/crypto"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/keyring"
)

// fakeFetcher is a keyring.KeyFetcher backed by locally generated keys, so the
// cache-miss unlock path runs end-to-end without a live backend.
type fakeFetcher struct {
	salts proton.Salts
	user  proton.User
	addrs []proton.Address
}

func (f fakeFetcher) GetSalts(context.Context) (proton.Salts, error) { return f.salts, nil }
func (f fakeFetcher) GetUser(context.Context) (proton.User, error)   { return f.user, nil }
func (f fakeFetcher) GetAddresses(context.Context) ([]proton.Address, error) {
	return f.addrs, nil
}

// lockedFetcher builds a fakeFetcher whose user+address keys are locked under
// mailboxPassword, so keyring.Unlock(ctx, f, []byte(mailboxPassword)) succeeds.
func lockedFetcher(t *testing.T, mailboxPassword string) fakeFetcher {
	t.Helper()
	rawSalt := []byte("abcdef1234567890") // 16 bytes
	userKey, userSalt := lockedKey(t, "user-key", mailboxPassword, rawSalt)
	addrKey, _ := lockedKey(t, "addr-key", mailboxPassword, rawSalt)
	return fakeFetcher{
		salts: proton.Salts{userSalt},
		user:  proton.User{Keys: proton.Keys{userKey}},
		addrs: []proton.Address{{ID: "addr-1", Keys: proton.Keys{addrKey}}},
	}
}

func lockedKey(t *testing.T, keyID, masterPass string, rawSalt []byte) (proton.Key, proton.Salt) {
	t.Helper()
	hashed, err := srp.MailboxPassword([]byte(masterPass), rawSalt)
	if err != nil {
		t.Fatalf("mailbox password: %v", err)
	}
	derived := hashed[len(hashed)-31:]
	unlocked, err := crypto.GenerateKey("tester", "tester@example.test", "x25519", 0)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	locked, err := unlocked.Lock(derived)
	if err != nil {
		t.Fatalf("lock key: %v", err)
	}
	raw, err := locked.Serialize()
	if err != nil {
		t.Fatalf("serialize key: %v", err)
	}
	return proton.Key{
			ID:         keyID,
			PrivateKey: raw,
			Primary:    proton.Bool(true),
			Active:     proton.Bool(true),
		}, proton.Salt{
			ID:      keyID,
			KeySalt: base64.StdEncoding.EncodeToString(rawSalt),
		}
}

// credKC is a Store that records LoadCreds calls and returns the configured
// creds/error so the cache-miss orchestration (LoadCreds + mailbox fallback)
// can be driven. Only LoadCreds is exercised by these tests.
type credKC struct {
	creds     keychain.Creds
	credsErr  error
	loadCalls int
}

func (k *credKC) LoadCreds() (keychain.Creds, error) {
	k.loadCalls++
	if k.credsErr != nil {
		return keychain.Creds{}, k.credsErr
	}
	return k.creds, nil
}

func (*credKC) SaveCreds(keychain.Creds) error         { return nil }
func (*credKC) SaveSession(keychain.Session) error     { return nil }
func (*credKC) LoadSession() (keychain.Session, error) { return keychain.Session{}, nil }
func (*credKC) Clear() error                           { return nil }

func newSessionWithFetcher(kc Store, f keyring.KeyFetcher) *Session {
	s := &Session{kc: kc, raw: newRawClient("http://invalid.test", nil)}
	s.keyFetcher = func(context.Context) (keyring.KeyFetcher, error) { return f, nil }
	return s
}

// TestKeyringsOnePasswordFallback proves that when MailboxPassword is empty the
// login Password is used to unlock — the one-password-account path. A regression
// dropping the fallback would pass an empty mailbox password and unlock would
// fail, so a successful unlock here is the assertion.
func TestKeyringsOnePasswordFallback(t *testing.T) {
	const pass = "login-password"
	kc := &credKC{creds: keychain.Creds{Password: pass}} // MailboxPassword empty
	s := newSessionWithFetcher(kc, lockedFetcher(t, pass))

	krs, err := s.Keyrings(t.Context())
	if err != nil {
		t.Fatalf("Keyrings (one-password fallback): %v", err)
	}
	if krs.User == nil {
		t.Fatal("expected an unlocked user keyring")
	}
}

// TestKeyringsPrefersMailboxPassword proves the two-password path: a non-empty
// MailboxPassword is used verbatim and the login Password is NOT substituted.
// The fetcher is locked under the mailbox password while Password is wrong, so
// success means the fallback did not fire.
func TestKeyringsPrefersMailboxPassword(t *testing.T) {
	const mailbox = "mailbox-password"
	kc := &credKC{creds: keychain.Creds{Password: "wrong-login-pass", MailboxPassword: mailbox}}
	s := newSessionWithFetcher(kc, lockedFetcher(t, mailbox))

	if _, err := s.Keyrings(t.Context()); err != nil {
		t.Fatalf("Keyrings (two-password): %v", err)
	}
}

// TestKeyringsCachesAcrossCalls proves the cache-miss path populates s.keyrings
// under the write lock and the second call reuses it: LoadCreds runs once, and
// both calls return the same pointer.
func TestKeyringsCachesAcrossCalls(t *testing.T) {
	const pass = "login-password"
	kc := &credKC{creds: keychain.Creds{Password: pass}}
	s := newSessionWithFetcher(kc, lockedFetcher(t, pass))

	first, err := s.Keyrings(t.Context())
	if err != nil {
		t.Fatalf("first Keyrings: %v", err)
	}
	second, err := s.Keyrings(t.Context())
	if err != nil {
		t.Fatalf("second Keyrings: %v", err)
	}
	if first != second {
		t.Fatal("second call must return the cached keyrings pointer")
	}
	if kc.loadCalls != 1 {
		t.Fatalf("LoadCreds called %d times, want 1 (result must be cached)", kc.loadCalls)
	}
	if s.keyrings != first {
		t.Fatal("s.keyrings must hold the unlocked result after the first call")
	}
}

// TestKeyringsLoadCredsError covers the LoadCreds error wrapper. The fetcher
// resolves, but LoadCreds fails, so Keyrings must return the wrapped error and
// leave the cache empty.
func TestKeyringsLoadCredsError(t *testing.T) {
	cause := errors.New("credfile read failed")
	kc := &credKC{credsErr: cause}
	s := newSessionWithFetcher(kc, fakeFetcher{})

	_, err := s.Keyrings(t.Context())
	if err == nil {
		t.Fatal("expected error when LoadCreds fails")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("LoadCreds cause not preserved: %v", err)
	}
	const want = "load creds for keyring unlock"
	if got := err.Error(); !strings.Contains(got, want) {
		t.Fatalf("error %q missing wrapper %q", got, want)
	}
	if s.keyrings != nil {
		t.Fatal("cache must stay empty after a LoadCreds failure")
	}
}

// TestKeyringsUnlockError covers the keyring.Unlock error wrapper: the fetcher
// returns a user with no primary key, so Unlock fails and Keyrings wraps it.
func TestKeyringsUnlockError(t *testing.T) {
	kc := &credKC{creds: keychain.Creds{Password: "x"}}
	s := newSessionWithFetcher(kc, fakeFetcher{user: proton.User{Keys: proton.Keys{}}})

	_, err := s.Keyrings(t.Context())
	if err == nil {
		t.Fatal("expected error when unlock fails")
	}
	const want = "unlock keyrings"
	if got := err.Error(); !strings.Contains(got, want) {
		t.Fatalf("error %q missing wrapper %q", got, want)
	}
	if s.keyrings != nil {
		t.Fatal("cache must stay empty after an unlock failure")
	}
}

// TestKeyringsFetcherError covers the fetcher-resolution error path: when the
// client/fetcher can't be obtained, Keyrings returns that error before
// attempting LoadCreds or unlock.
func TestKeyringsFetcherError(t *testing.T) {
	cause := errors.New("no client")
	kc := &credKC{}
	s := &Session{kc: kc, raw: newRawClient("http://invalid.test", nil)}
	s.keyFetcher = func(context.Context) (keyring.KeyFetcher, error) { return nil, cause }

	_, err := s.Keyrings(t.Context())
	if !errors.Is(err, cause) {
		t.Fatalf("want fetcher error, got %v", err)
	}
	if kc.loadCalls != 0 {
		t.Fatal("LoadCreds must not run when the fetcher fails")
	}
}

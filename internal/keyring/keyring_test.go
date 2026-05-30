package keyring

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	srp "github.com/ProtonMail/go-srp"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

// newTestKeyRing generates an unlocked in-memory keyring for tests.
func newTestKeyRing(t *testing.T) *crypto.KeyRing {
	t.Helper()
	key, err := crypto.GenerateKey("tester", "tester@example.test", "x25519", 0)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	kr, err := crypto.NewKeyRing(key)
	if err != nil {
		t.Fatalf("new keyring: %v", err)
	}
	return kr
}

// lockedProtonKey creates a proton.Key locked under a passphrase derived the
// same way SaltForKey does (srp.MailboxPassword, last 31 bytes). Returns the
// Key and the Salts entry so callers can pass them to a fakeFetcher.
func lockedProtonKey(
	t *testing.T,
	keyID string,
	masterPass []byte,
	rawSalt []byte,
) (proton.Key, proton.Salt) {
	t.Helper()

	// Derive the same passphrase that SaltForKey produces.
	hashed, err := srp.MailboxPassword(masterPass, rawSalt)
	if err != nil {
		t.Fatalf("mailbox password: %v", err)
	}
	derived := hashed[len(hashed)-31:]

	// Generate a gopenpgp key and lock it with the derived passphrase.
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

	key := proton.Key{
		ID:         keyID,
		PrivateKey: raw,
		Primary:    proton.Bool(true),
		Active:     proton.Bool(true),
	}
	salt := proton.Salt{
		ID:      keyID,
		KeySalt: base64.StdEncoding.EncodeToString(rawSalt),
	}
	return key, salt
}

func TestDecryptBodyRecoversPlaintext(t *testing.T) {
	kr := newTestKeyRing(t)
	plaintext := "delivery confirmed"
	armored, err := kr.Encrypt(crypto.NewPlainMessageFromString(plaintext), nil)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	pgp, err := armored.GetArmored()
	if err != nil {
		t.Fatalf("armor: %v", err)
	}

	krs := &Keyrings{User: kr, Addr: map[string]*crypto.KeyRing{"addr-1": kr}}
	got, err := krs.DecryptBody("addr-1", pgp)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plaintext {
		t.Fatalf("DecryptBody = %q, want %q", got, plaintext)
	}
}

func TestDecryptBodyUnknownAddressErrors(t *testing.T) {
	krs := &Keyrings{User: nil, Addr: map[string]*crypto.KeyRing{}}
	if _, err := krs.DecryptBody("missing", "irrelevant"); err == nil {
		t.Fatal("expected error for unknown address ID")
	}
}

func TestDecryptBodyBadArmoredErrors(t *testing.T) {
	kr := newTestKeyRing(t)
	krs := &Keyrings{User: kr, Addr: map[string]*crypto.KeyRing{"addr-1": kr}}
	if _, err := krs.DecryptBody("addr-1", "not-armored-pgp-data"); err == nil {
		t.Fatal("expected error for malformed armored body")
	}
}

func TestDecryptBodyWrongKeyErrors(t *testing.T) {
	// Encrypt to one key, attempt to decrypt with a different key.
	encKR := newTestKeyRing(t)
	decKR := newTestKeyRing(t)
	armored, err := encKR.Encrypt(crypto.NewPlainMessageFromString("secret"), nil)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	pgp, err := armored.GetArmored()
	if err != nil {
		t.Fatalf("armor: %v", err)
	}
	krs := &Keyrings{User: decKR, Addr: map[string]*crypto.KeyRing{"addr-1": decKR}}
	if _, err := krs.DecryptBody("addr-1", pgp); err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
}

type fakeFetcher struct {
	salts    proton.Salts
	saltsErr error
	user     proton.User
	userErr  error
	addrs    []proton.Address
	addrsErr error
}

func (f fakeFetcher) GetSalts(context.Context) (proton.Salts, error) {
	return f.salts, f.saltsErr
}

func (f fakeFetcher) GetUser(context.Context) (proton.User, error) {
	return f.user, f.userErr
}

func (f fakeFetcher) GetAddresses(context.Context) ([]proton.Address, error) {
	return f.addrs, f.addrsErr
}

func TestUnlockSaltsError(t *testing.T) {
	f := fakeFetcher{saltsErr: errors.New("network error")}
	_, err := Unlock(context.Background(), f, []byte("pass"))
	if err == nil {
		t.Fatal("expected error when GetSalts fails")
	}
}

func TestUnlockUserError(t *testing.T) {
	f := fakeFetcher{
		salts:   proton.Salts{},
		userErr: errors.New("auth error"),
	}
	_, err := Unlock(context.Background(), f, []byte("pass"))
	if err == nil {
		t.Fatal("expected error when GetUser fails")
	}
}

func TestUnlockNoPrimaryKey(t *testing.T) {
	f := fakeFetcher{
		salts: proton.Salts{},
		user:  proton.User{Keys: proton.Keys{}},
	}
	_, err := Unlock(context.Background(), f, []byte("pass"))
	if err == nil {
		t.Fatal("expected error when user has no primary key")
	}
}

// TestUnlockNoPrimaryKeyNonEmpty covers a user with keys where none is marked
// primary: proton.Keys.Primary() would panic, so Unlock must return an error.
func TestUnlockNoPrimaryKeyNonEmpty(t *testing.T) {
	f := fakeFetcher{
		user: proton.User{Keys: proton.Keys{{ID: "k1"}}},
	}
	if _, err := Unlock(context.Background(), f, []byte("pw")); err == nil {
		t.Fatal("expected error when no key is primary, got nil (or it panicked)")
	}
}

// TestUnlockSaltNotFoundError covers the SaltForKey error path: user has a key
// but the salts slice has no matching entry.
func TestUnlockSaltNotFoundError(t *testing.T) {
	f := fakeFetcher{
		salts: proton.Salts{},
		user: proton.User{
			Keys: proton.Keys{
				{ID: "key-1", Primary: proton.Bool(true), Active: proton.Bool(true)},
			},
		},
	}
	_, err := Unlock(context.Background(), f, []byte("pass"))
	if err == nil {
		t.Fatal("expected error when salt not found for key")
	}
}

// TestUnlockUserKeysUnlockError covers the user.Keys.Unlock error path: the
// key bytes are garbage so every unlock attempt fails, returning an error.
func TestUnlockUserKeysUnlockError(t *testing.T) {
	rawSalt := []byte("1234567890abcdef") // 16 bytes
	hashed, err := srp.MailboxPassword([]byte("pass"), rawSalt)
	if err != nil {
		t.Fatalf("mailbox password: %v", err)
	}
	_ = hashed // salt round-trip is valid; key bytes are intentionally garbage

	f := fakeFetcher{
		salts: proton.Salts{{
			ID:      "key-1",
			KeySalt: base64.StdEncoding.EncodeToString(rawSalt),
		}},
		user: proton.User{
			Keys: proton.Keys{
				{
					ID:         "key-1",
					PrivateKey: []byte("garbage-not-a-real-key"),
					Primary:    proton.Bool(true),
					Active:     proton.Bool(true),
				},
			},
		},
	}
	_, err = Unlock(context.Background(), f, []byte("pass"))
	if err == nil {
		t.Fatal("expected error when user key bytes are invalid")
	}
}

// TestUnlockGetAddressesError covers the GetAddresses error path.
func TestUnlockGetAddressesError(t *testing.T) {
	masterPass := []byte("mailbox-pass")
	rawSalt := []byte("abcdef1234567890") // 16 bytes
	key, salt := lockedProtonKey(t, "key-1", masterPass, rawSalt)

	f := fakeFetcher{
		salts:    proton.Salts{salt},
		user:     proton.User{Keys: proton.Keys{key}},
		addrsErr: errors.New("addresses fetch failed"),
	}
	_, err := Unlock(context.Background(), f, masterPass)
	if err == nil {
		t.Fatal("expected error when GetAddresses fails")
	}
}

// TestUnlockHappyPath covers the successful unlock + address iteration path.
func TestUnlockHappyPath(t *testing.T) {
	masterPass := []byte("mailbox-pass")
	rawSalt := []byte("abcdef1234567890") // 16 bytes
	userKey, userSalt := lockedProtonKey(t, "key-1", masterPass, rawSalt)
	addrKey, _ := lockedProtonKey(t, "addr-key-1", masterPass, rawSalt)

	f := fakeFetcher{
		salts: proton.Salts{userSalt},
		user:  proton.User{Keys: proton.Keys{userKey}},
		addrs: []proton.Address{
			{ID: "addr-1", Keys: proton.Keys{addrKey}},
		},
	}
	krs, err := Unlock(context.Background(), f, masterPass)
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if krs.User == nil {
		t.Fatal("expected non-nil user keyring")
	}
	if _, ok := krs.Addr["addr-1"]; !ok {
		t.Fatal("expected keyring for addr-1")
	}
}

// TestUnlockAddressSkippedOnError covers the continue path when an address key
// fails to unlock — the address is skipped but the call still succeeds.
func TestUnlockAddressSkippedOnError(t *testing.T) {
	masterPass := []byte("mailbox-pass")
	rawSalt := []byte("abcdef1234567890") // 16 bytes
	userKey, userSalt := lockedProtonKey(t, "key-1", masterPass, rawSalt)

	f := fakeFetcher{
		salts: proton.Salts{userSalt},
		user:  proton.User{Keys: proton.Keys{userKey}},
		addrs: []proton.Address{
			{
				ID: "bad-addr",
				Keys: proton.Keys{
					{
						ID:         "bad-key",
						PrivateKey: []byte("garbage"),
						Primary:    proton.Bool(true),
						Active:     proton.Bool(true),
					},
				},
			},
		},
	}
	krs, err := Unlock(context.Background(), f, masterPass)
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if _, ok := krs.Addr["bad-addr"]; ok {
		t.Fatal("expected bad-addr to be skipped, not present")
	}
}

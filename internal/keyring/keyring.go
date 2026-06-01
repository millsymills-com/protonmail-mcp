// Package keyring turns fetched Proton salts/keys plus the mailbox password
// into unlocked PGP keyrings and decrypts message bodies. Unlocked keyrings
// hold decrypted private key material: never persist or log a *Keyrings.
package keyring

import (
	"context"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
)

// KeyFetcher is the subset of *proton.Client the unlocker needs. *proton.Client
// satisfies it; tests inject fakes.
type KeyFetcher interface {
	GetSalts(ctx context.Context) (proton.Salts, error)
	GetUser(ctx context.Context) (proton.User, error)
	GetAddresses(ctx context.Context) ([]proton.Address, error)
}

// Keyrings holds the unlocked user keyring and one keyring per address ID.
type Keyrings struct {
	User *crypto.KeyRing
	Addr map[string]*crypto.KeyRing
}

// Unlock derives the salted mailbox passphrase, unlocks the user keyring, then
// unlocks each address keyring against it. mailboxPassword must be the actual
// mailbox password (login password for one-password-mode accounts).
//
// Unlock consumes mailboxPassword: it zeroes the caller's slice and the derived
// salted passphrase before returning (on every path, success or error) so the
// plaintext inputs to the keyring don't linger on the heap for GC. The unlocked
// *Keyrings still holds decrypted private key material — see ClearPrivateParams.
//
// Note: proton.Keys.Primary() panics when no key is marked primary, so we find
// the primary key with an explicit loop and return an error instead.
func Unlock(ctx context.Context, f KeyFetcher, mailboxPassword []byte) (*Keyrings, error) {
	defer zero(mailboxPassword)
	salts, err := f.GetSalts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get salts: %w", err)
	}
	user, err := f.GetUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	var primary proton.Key
	hasPrimary := false
	for _, k := range user.Keys {
		if k.Primary {
			primary = k
			hasPrimary = true
			break
		}
	}
	if !hasPrimary {
		return nil, fmt.Errorf("user has no primary key")
	}
	saltedKeyPass, err := salts.SaltForKey(mailboxPassword, primary.ID)
	if err != nil {
		return nil, fmt.Errorf("salt for key: %w", err)
	}
	defer zero(saltedKeyPass)
	userKR, err := user.Keys.Unlock(saltedKeyPass, nil)
	if err != nil {
		return nil, fmt.Errorf("unlock user keyring: %w: %w", proterr.ErrKeyringLocked, err)
	}
	addrs, err := f.GetAddresses(ctx)
	if err != nil {
		return nil, fmt.Errorf("get addresses: %w", err)
	}
	addrKRs := make(map[string]*crypto.KeyRing, len(addrs))
	for _, a := range addrs {
		kr, err := a.Keys.Unlock(saltedKeyPass, userKR)
		if err != nil {
			// A disabled or inactive address can have no usable key; skip it so one
			// such address doesn't block decryption for the others. Total failure
			// (no address unlocked at all) is caught by the guard after the loop.
			continue
		}
		addrKRs[a.ID] = kr
	}
	if len(addrs) > 0 && len(addrKRs) == 0 {
		return nil, fmt.Errorf(
			"%w: no address keyring could be unlocked (wrong password or all addresses disabled)",
			proterr.ErrKeyringLocked,
		)
	}
	return &Keyrings{User: userKR, Addr: addrKRs}, nil
}

// zero overwrites b in place to shorten the window secret material (a mailbox
// password or salted passphrase) sits unencrypted on the heap. Best-effort: Go
// strings can't be wiped and the runtime may have copied the bytes elsewhere,
// so this reduces residue rather than guaranteeing erasure.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// ClearPrivateParams wipes the decrypted private key material from every held
// keyring, shortening the window it sits unencrypted on the heap. Call it when
// dropping a cached *Keyrings (logout/relogin) rather than relying on GC. The
// receiver stays usable only for public-key operations afterward; decryption
// will fail, so callers must also drop their reference.
func (k *Keyrings) ClearPrivateParams() {
	if k.User != nil {
		k.User.ClearPrivateParams()
	}
	for _, kr := range k.Addr {
		kr.ClearPrivateParams()
	}
}

// DecryptBody decrypts an armored PGP body using the keyring for addrID.
func (k *Keyrings) DecryptBody(addrID, armoredBody string) (string, error) {
	kr, ok := k.Addr[addrID]
	if !ok {
		// The user keyring unlocked fine; this message's address just has no
		// usable keyring (disabled/skipped address, or an unknown addrID). That
		// is a body-level problem, not a mailbox-password one — classify it as
		// undecryptable so the operator isn't sent to re-check their password.
		return "", fmt.Errorf("%w: no unlocked keyring for address %s", proterr.ErrBodyUndecryptable, addrID)
	}
	msg, err := crypto.NewPGPMessageFromArmored(armoredBody)
	if err != nil {
		return "", fmt.Errorf("parse armored body: %w: %w", proterr.ErrBodyUndecryptable, err)
	}
	plain, err := kr.Decrypt(msg, nil, 0)
	if err != nil {
		// A parseable PGP body that won't decrypt with an unlocked keyring was
		// encrypted to a key we don't hold (forwarded/foreign-key message, key
		// rotation) — the keyring is fine, so this is not ErrKeyringLocked.
		return "", fmt.Errorf("decrypt body: %w: %w", proterr.ErrBodyUndecryptable, err)
	}
	return plain.GetString(), nil
}

// Package keyring turns fetched Proton salts/keys plus the mailbox password
// into unlocked PGP keyrings and decrypts message bodies. Unlocked keyrings
// hold decrypted private key material: never persist or log a *Keyrings.
package keyring

import (
	"context"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
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
// Note: proton.Keys.Primary() panics when no key is marked primary, so we find
// the primary key with an explicit loop and return an error instead.
func Unlock(ctx context.Context, f KeyFetcher, mailboxPassword []byte) (*Keyrings, error) {
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
	userKR, err := user.Keys.Unlock(saltedKeyPass, nil)
	if err != nil {
		return nil, fmt.Errorf("unlock user keyring: %w", err)
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
			"no address keyring could be unlocked (wrong password or all addresses disabled)",
		)
	}
	return &Keyrings{User: userKR, Addr: addrKRs}, nil
}

// DecryptBody decrypts an armored PGP body using the keyring for addrID.
func (k *Keyrings) DecryptBody(addrID, armoredBody string) (string, error) {
	kr, ok := k.Addr[addrID]
	if !ok {
		return "", fmt.Errorf("no unlocked keyring for address %s", addrID)
	}
	msg, err := crypto.NewPGPMessageFromArmored(armoredBody)
	if err != nil {
		return "", fmt.Errorf("parse armored body: %w", err)
	}
	plain, err := kr.Decrypt(msg, nil, 0)
	if err != nil {
		return "", fmt.Errorf("decrypt body: %w", err)
	}
	return plain.GetString(), nil
}

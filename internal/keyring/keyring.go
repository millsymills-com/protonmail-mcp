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
// Note: Keys.Primary() panics if the user has no keys; we guard with a len
// check before calling it.
func Unlock(ctx context.Context, f KeyFetcher, mailboxPassword []byte) (*Keyrings, error) {
	salts, err := f.GetSalts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get salts: %w", err)
	}
	user, err := f.GetUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if len(user.Keys) == 0 {
		return nil, fmt.Errorf("user has no primary key")
	}
	primary := user.Keys.Primary()
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
			// A disabled address with no usable key is not fatal — skip it so a
			// single bad address can't block decryption for all the others.
			continue
		}
		addrKRs[a.ID] = kr
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

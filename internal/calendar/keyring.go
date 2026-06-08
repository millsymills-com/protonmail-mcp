// Package calendar resolves Proton calendar keyrings and turns encrypted
// calendar events into structured, time-expanded occurrences. It holds and
// uses decrypted private key material; never persist or log a *crypto.KeyRing
// returned from here.
package calendar

import (
	"context"
	"fmt"
	"strings"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/millsymills-com/protonmail-mcp/internal/keyring"
)

// KeyResolver is the subset of *proton.Client needed to resolve a calendar
// keyring. *proton.Client satisfies it; tests inject fakes.
type KeyResolver interface {
	GetCalendarMembers(ctx context.Context, calendarID string) ([]proton.CalendarMember, error)
	GetCalendarPassphrase(ctx context.Context, calendarID string) (proton.CalendarPassphrase, error)
	GetCalendarKeys(ctx context.Context, calendarID string) (proton.CalendarKeys, error)
	GetAddresses(ctx context.Context) ([]proton.Address, error)
}

// ResolveKeyring unlocks the calendar keyring for calendarID. It matches a
// calendar member to one of the user's addresses, decrypts that member's
// passphrase with the address keyring supplied in krs, then unlocks the
// calendar keys with the passphrase.
func ResolveKeyring(ctx context.Context, c KeyResolver, krs *keyring.Keyrings, calendarID string) (*crypto.KeyRing, error) {
	members, err := c.GetCalendarMembers(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("get calendar members: %w", err)
	}
	addrs, err := c.GetAddresses(ctx)
	if err != nil {
		return nil, fmt.Errorf("get addresses: %w", err)
	}
	emailToID := make(map[string]string, len(addrs))
	for _, a := range addrs {
		emailToID[strings.ToLower(a.Email)] = a.ID
	}
	pass, err := c.GetCalendarPassphrase(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("get calendar passphrase: %w", err)
	}

	var passphrase []byte
	for _, m := range members {
		addrID, ok := emailToID[strings.ToLower(m.Email)]
		if !ok {
			continue
		}
		addrKR := krs.Addr[addrID]
		if addrKR == nil {
			continue
		}
		dec, derr := pass.Decrypt(m.ID, addrKR)
		if derr != nil {
			continue
		}
		passphrase = dec
		break
	}
	if passphrase == nil {
		return nil, fmt.Errorf("no calendar member matched an unlocked address keyring for %s", calendarID)
	}

	keys, err := c.GetCalendarKeys(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("get calendar keys: %w", err)
	}
	calKR, err := keys.Unlock(passphrase)
	if err != nil {
		return nil, fmt.Errorf("unlock calendar keyring %s: %w", calendarID, err)
	}
	if calKR.CountDecryptionEntities() == 0 {
		return nil, fmt.Errorf("calendar keyring %s: no key unlocked with the resolved passphrase", calendarID)
	}
	return calKR, nil
}

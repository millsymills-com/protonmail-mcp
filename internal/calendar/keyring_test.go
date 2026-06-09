package calendar_test

import (
	"context"
	"errors"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/millsymills-com/protonmail-mcp/internal/calendar"
	"github.com/millsymills-com/protonmail-mcp/internal/keyring"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
)

// fakeResolver returns canned calendar metadata, built per-test by the fixture
// helpers.
type fakeResolver struct {
	members []proton.CalendarMember
	pass    proton.CalendarPassphrase
	keys    proton.CalendarKeys
	addrs   []proton.Address
}

func (f fakeResolver) GetCalendarMembers(_ context.Context, _ string) ([]proton.CalendarMember, error) {
	return f.members, nil
}
func (f fakeResolver) GetCalendarPassphrase(_ context.Context, _ string) (proton.CalendarPassphrase, error) {
	return f.pass, nil
}
func (f fakeResolver) GetCalendarKeys(_ context.Context, _ string) (proton.CalendarKeys, error) {
	return f.keys, nil
}
func (f fakeResolver) GetAddresses(_ context.Context) ([]proton.Address, error) {
	return f.addrs, nil
}

func TestResolveKeyring_Success(t *testing.T) {
	fx := newCalendarFixture(t) // helper: builds address key + calendar key + member passphrase
	krs := &keyring.Keyrings{Addr: map[string]*crypto.KeyRing{fx.AddrID: fx.AddrKR}}

	calKR, err := calendar.ResolveKeyring(context.Background(), fx.Resolver(), krs, "cal-1")
	if err != nil {
		t.Fatalf("ResolveKeyring: %v", err)
	}
	if calKR == nil {
		t.Fatal("nil calendar keyring")
	}

	plain := crypto.NewPlainMessageFromString("ping")
	enc, err := calKR.Encrypt(plain, nil)
	if err != nil {
		t.Fatalf("encrypt to calKR: %v", err)
	}
	dec, err := calKR.Decrypt(enc, nil, crypto.GetUnixTime())
	if err != nil {
		t.Fatalf("decrypt with calKR: %v", err)
	}
	if dec.GetString() != "ping" {
		t.Fatalf("round-trip = %q", dec.GetString())
	}
}

func TestResolveKeyring_AddressKeyringAbsent(t *testing.T) {
	fx := newCalendarFixture(t)
	krs := &keyring.Keyrings{Addr: map[string]*crypto.KeyRing{"other-addr": fx.AddrKR}}

	_, err := calendar.ResolveKeyring(context.Background(), fx.Resolver(), krs, "cal-1")
	if err == nil {
		t.Fatal("expected error when no address keyring matches a member")
	}
	if !errors.Is(err, proterr.ErrKeyringLocked) {
		t.Fatalf("error = %v, want classified as ErrKeyringLocked", err)
	}
}

func TestResolveKeyring_NoMatchingEmail(t *testing.T) {
	fx := newCalendarFixture(t)
	r := fx.Resolver()
	r.addrs = nil
	krs := &keyring.Keyrings{Addr: map[string]*crypto.KeyRing{fx.AddrID: fx.AddrKR}}

	_, err := calendar.ResolveKeyring(context.Background(), r, krs, "cal-1")
	if err == nil {
		t.Fatal("expected error when no member email matches an address")
	}
	if !errors.Is(err, proterr.ErrKeyringLocked) {
		t.Fatalf("error = %v, want classified as ErrKeyringLocked", err)
	}
}

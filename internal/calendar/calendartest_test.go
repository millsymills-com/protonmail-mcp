package calendar_test

import (
	"encoding/base64"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/ProtonMail/gopenpgp/v2/helper"
)

type calFixture struct {
	AddrID  string
	AddrKR  *crypto.KeyRing
	calKR   *crypto.KeyRing
	members []proton.CalendarMember
	pass    proton.CalendarPassphrase
	keys    proton.CalendarKeys
	addrs   []proton.Address
}

// newCalendarFixture builds a self-consistent calendar crypto context:
//   - an address keyring (stands in for the Phase 0 addrKR),
//   - a calendar passphrase encrypted+signed to that address keyring,
//   - a calendar key locked with that passphrase.
func newCalendarFixture(t *testing.T) *calFixture {
	t.Helper()
	const email = "me@example.test"

	addrKey, err := crypto.GenerateKey("addr", email, "x25519", 0)
	if err != nil {
		t.Fatalf("gen addr key: %v", err)
	}
	addrKR, err := crypto.NewKeyRing(addrKey)
	if err != nil {
		t.Fatalf("addr keyring: %v", err)
	}

	// A 32-byte calendar passphrase, encrypted+signed to the address keyring.
	passphrase := []byte("0123456789abcdef0123456789abcdef")
	encArmored, err := addrKR.Encrypt(crypto.NewPlainMessage(passphrase), addrKR)
	if err != nil {
		t.Fatalf("encrypt passphrase: %v", err)
	}
	sig, err := addrKR.SignDetached(crypto.NewPlainMessage(passphrase))
	if err != nil {
		t.Fatalf("sign passphrase: %v", err)
	}
	encArm, err := encArmored.GetArmored()
	if err != nil {
		t.Fatalf("armor passphrase msg: %v", err)
	}
	sigArm, err := sig.GetArmored()
	if err != nil {
		t.Fatalf("armor passphrase sig: %v", err)
	}

	calKeyArmored, err := helper.GenerateKey("cal", "cal@calendar.proton.me", passphrase, "x25519", 0)
	if err != nil {
		t.Fatalf("gen cal key: %v", err)
	}
	calKey, err := crypto.NewKeyFromArmored(calKeyArmored)
	if err != nil {
		t.Fatalf("parse cal key: %v", err)
	}
	unlockedCalKey, err := calKey.Unlock(passphrase)
	if err != nil {
		t.Fatalf("unlock cal key: %v", err)
	}
	calKR, err := crypto.NewKeyRing(unlockedCalKey)
	if err != nil {
		t.Fatalf("cal keyring: %v", err)
	}

	return &calFixture{
		AddrID: "addr-1",
		AddrKR: addrKR,
		calKR:  calKR,
		members: []proton.CalendarMember{
			{ID: "member-1", Email: email, CalendarID: "cal-1"},
		},
		pass: proton.CalendarPassphrase{
			ID: "pass-1",
			MemberPassphrases: []proton.MemberPassphrase{
				{MemberID: "member-1", Passphrase: encArm, Signature: sigArm},
			},
		},
		keys: proton.CalendarKeys{
			{ID: "calkey-1", CalendarID: "cal-1", PassphraseID: "pass-1", PrivateKey: calKeyArmored, Flags: proton.CalendarKeyFlagPrimary},
		},
		addrs: []proton.Address{{ID: "addr-1", Email: email}},
	}
}

func (f *calFixture) Resolver() fakeResolver {
	return fakeResolver{members: f.members, pass: f.pass, keys: f.keys, addrs: f.addrs}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) } //nolint:unused // used by sibling _test.go files

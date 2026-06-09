package session

import (
	"context"
	"testing"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

func TestCalendarKeyring_Caches(t *testing.T) {
	s := &Session{calKeyrings: map[string]*crypto.KeyRing{}}
	kr, _ := crypto.NewKeyRing(nil)
	s.calKeyrings["cal-1"] = kr // pre-seed to assert the cache hit path

	got, err := s.CalendarKeyring(context.Background(), "cal-1")
	if err != nil {
		t.Fatalf("CalendarKeyring: %v", err)
	}
	if got != kr {
		t.Fatal("expected cached calendar keyring")
	}
}

func TestCalendarKeyring_ClearedOnReset(t *testing.T) {
	kr, _ := crypto.NewKeyRing(nil)
	s := &Session{calKeyrings: map[string]*crypto.KeyRing{"cal-1": kr}}
	s.clearKeyringCache()
	if len(s.calKeyrings) != 0 {
		t.Fatalf("calKeyrings not cleared: %d entries", len(s.calKeyrings))
	}
}

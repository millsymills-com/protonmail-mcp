package calendar_test

import (
	"strings"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/millsymills-com/protonmail-mcp/internal/calendar"
)

const sampleICS = "BEGIN:VEVENT\r\nUID:evt-1\r\nSUMMARY:Standup\r\nEND:VEVENT"

func TestDecryptSharedEvent_Armored(t *testing.T) {
	fx := newCalendarFixture(t)
	enc, err := fx.calKR.Encrypt(crypto.NewPlainMessageFromString(sampleICS), nil)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	armored, err := enc.GetArmored()
	if err != nil {
		t.Fatalf("armor: %v", err)
	}
	ev := proton.CalendarEvent{
		SharedEvents: []proton.CalendarEventPart{
			{Type: proton.CalendarEventTypeEncrypted, Data: armored},
		},
	}

	got, err := calendar.DecryptSharedICS(ev, fx.calKR)
	if err != nil {
		t.Fatalf("DecryptSharedICS: %v", err)
	}
	if !strings.Contains(got, "SUMMARY:Standup") {
		t.Fatalf("decrypted ICS missing summary: %q", got)
	}
}

func TestDecryptSharedEvent_Clear(t *testing.T) {
	fx := newCalendarFixture(t)
	ev := proton.CalendarEvent{
		SharedEvents: []proton.CalendarEventPart{
			{Type: proton.CalendarEventTypeClear, Data: sampleICS},
		},
	}
	got, err := calendar.DecryptSharedICS(ev, fx.calKR)
	if err != nil {
		t.Fatalf("DecryptSharedICS: %v", err)
	}
	if !strings.Contains(got, "SUMMARY:Standup") {
		t.Fatalf("clear ICS missing summary: %q", got)
	}
}

// TestDecryptSharedEvent_KeyPacket covers the SharedKeyPacket branch: data is a
// base64 data packet and the key packet rides on the event, not the part.
func TestDecryptSharedEvent_KeyPacket(t *testing.T) {
	fx := newCalendarFixture(t)
	enc, err := fx.calKR.Encrypt(crypto.NewPlainMessageFromString(sampleICS), nil)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	split, err := enc.SplitMessage()
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	ev := proton.CalendarEvent{
		SharedKeyPacket: b64(split.GetBinaryKeyPacket()),
		SharedEvents: []proton.CalendarEventPart{
			{Type: proton.CalendarEventTypeEncrypted, Data: b64(split.GetBinaryDataPacket())},
		},
	}
	got, err := calendar.DecryptSharedICS(ev, fx.calKR)
	if err != nil {
		t.Fatalf("DecryptSharedICS: %v", err)
	}
	if !strings.Contains(got, "SUMMARY:Standup") {
		t.Fatalf("key-packet ICS missing summary: %q", got)
	}
}

func TestDecryptSharedEvent_BadKeyPacket(t *testing.T) {
	fx := newCalendarFixture(t)
	ev := proton.CalendarEvent{
		SharedKeyPacket: "!!!not-base64!!!",
		SharedEvents: []proton.CalendarEventPart{
			{Type: proton.CalendarEventTypeEncrypted, Data: b64([]byte("x"))},
		},
	}
	if _, err := calendar.DecryptSharedICS(ev, fx.calKR); err == nil {
		t.Fatal("expected error for invalid key-packet base64")
	}
}

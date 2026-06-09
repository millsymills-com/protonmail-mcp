package calendar

import (
	"encoding/base64"
	"fmt"
	"strings"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

// DecryptSharedICS decrypts and concatenates the SharedEvents parts of ev into
// a single iCalendar string. Encrypted parts use the event's SharedKeyPacket
// when present, otherwise they are treated as armored PGP. Clear parts pass
// through. The calendar keyring calKR must be unlocked (see ResolveKeyring).
func DecryptSharedICS(ev proton.CalendarEvent, calKR *crypto.KeyRing) (string, error) {
	var b strings.Builder
	for i, part := range ev.SharedEvents {
		text, err := decryptPart(part, ev.SharedKeyPacket, calKR)
		if err != nil {
			return "", fmt.Errorf("decrypt shared part %d: %w", i, err)
		}
		b.WriteString(text)
	}
	return b.String(), nil
}

func decryptPart(part proton.CalendarEventPart, sharedKeyPacket string, calKR *crypto.KeyRing) (string, error) {
	// CalendarEventType values are bit flags (Clear=0, Encrypted=1, Signed=2),
	// matching upstream Decode: a part can be both encrypted and signed, so the
	// Encrypted bit is tested with a bitwise AND rather than equality.
	if part.Type&proton.CalendarEventTypeEncrypted == 0 {
		return part.Data, nil
	}

	var enc *crypto.PGPMessage
	if sharedKeyPacket != "" {
		kp, err := base64.StdEncoding.DecodeString(sharedKeyPacket)
		if err != nil {
			return "", fmt.Errorf("decode key packet: %w", err)
		}
		raw, err := base64.StdEncoding.DecodeString(part.Data)
		if err != nil {
			return "", fmt.Errorf("decode data packet: %w", err)
		}
		enc = crypto.NewPGPSplitMessage(kp, raw).GetPGPMessage()
	} else {
		m, err := crypto.NewPGPMessageFromArmored(part.Data)
		if err != nil {
			return "", fmt.Errorf("parse armored part: %w", err)
		}
		enc = m
	}

	dec, err := calKR.Decrypt(enc, nil, crypto.GetUnixTime())
	if err != nil {
		return "", fmt.Errorf("decrypt part: %w", err)
	}
	return dec.GetString(), nil
}

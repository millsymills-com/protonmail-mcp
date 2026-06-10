package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

// calEventsKey and the list_events_happy cassette's encrypted event body are a
// matched pair: calEventsICS encrypted to this key, armored, with no key packet
// (so decryptPart takes the armored branch). To regenerate, build a
// *crypto.KeyRing from calEventsKey, call
// Encrypt(NewPlainMessageFromString(calEventsICS)).GetArmored(), and replace
// interaction id=3's SharedEvents[0].Data with the result.
const calEventsKey = `-----BEGIN PGP PRIVATE KEY BLOCK-----
Comment: https://gopenpgp.org
Version: GopenPGP 2.10.0

xVgEaicghRYJKwYBBAHaRw8BAQdA1HHf0TkbB8gJLtE1uZrYOorbpbbV9tDzvrTa
7SIM+2gAAQDHXny8sMzLtVlQPkarWQabxpJXE1zp9Wbl5KmWLgirsBEnzRJjYWwg
PGNhbEBjYWwudGVzdD7CvwQTFggAcQWCaicghQMLCQcJEG7/FbhVTaY3NRQAAAAA
ABwAEHNhbHRAbm90YXRpb25zLm9wZW5wZ3Bqcy5vcmcR9XZaLttyyY79aXHY2prq
AhUIAxYAAgIZAQKbAwIeARYhBFydl3YH7EdokoBEGG7/FbhVTaY3AADxrwEAyq6y
oFkqYHDkRjEFR63V8ZH0FVMIah5EdX0xFu7T0JABAMs3DQt+/cP5RjbwIWVct/M0
vJ+tfI51bvsKU1jQOzMMx10EaicghRIKKwYBBAGXVQEFAQEHQI7YzFcO2uGnBbgY
s/0USUkDS2CpU9KCpFbu80cUqhRtAwEKCQAA/30nEk6sGDf2+Di3VbdJ+q9l1PPO
w5OMfr0Wp/9rPp7wEuPCrgQYFggAYAWCaicghQkQbv8VuFVNpjc1FAAAAAAAHAAQ
c2FsdEBub3RhdGlvbnMub3BlbnBncGpzLm9yZzNog5cvxfUG73lE13lxlnECmwwW
IQRcnZd2B+xHaJKARBhu/xW4VU2mNwAA8FAA/2Ze73bsF6pxgjwuhiY+mfoUNocI
FCvpWushDM+MY5+eAQCOxasCOs1L1Mh2/mWTU1eNT9uVqijqZ7XsqPcDqsLvDA==
=xn9y
-----END PGP PRIVATE KEY BLOCK-----`

// calEventsICS is the plaintext encrypted into interaction id=3's event body,
// kept so the regeneration recipe on calEventsKey is self-contained. Its
// BEGIN:VCALENDAR…END wrapper is assumed, not yet live-confirmed (#196 item 2);
// once the calendar_list_events_happy live trace reports the real wrapper shape,
// reconcile this to match and re-encrypt.
const calEventsICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
	"UID:u1\r\nSUMMARY:Weekly Standup\r\nLOCATION:Room 1\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=MO\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR"

// fixedCalKeyring delegates the whole Service surface to a real session but
// returns a fixed in-memory calendar keyring for any CalendarKeyring call,
// so the encrypted cassette event decrypts without resolving keys over HTTP.
type fixedCalKeyring struct {
	session.Service
	kr *crypto.KeyRing
}

func (f fixedCalKeyring) CalendarKeyring(context.Context, string) (*crypto.KeyRing, error) {
	return f.kr, nil
}

func newFixedCalKeyring(t *testing.T) func(session.Service) session.Service {
	t.Helper()
	key, err := crypto.NewKeyFromArmored(calEventsKey)
	if err != nil {
		t.Fatalf("parse cal key: %v", err)
	}
	kr, err := crypto.NewKeyRing(key)
	if err != nil {
		t.Fatalf("cal keyring: %v", err)
	}
	return func(s session.Service) session.Service {
		return fixedCalKeyring{Service: s, kr: kr}
	}
}

func TestListEventsHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "list_events_happy",
		testharness.WithSessionService(newFixedCalKeyring(t)))
	defer h.Close()

	out, err := h.Call(context.Background(), "proton_list_events", map[string]any{
		"start": "2026-06-01T00:00:00Z",
		"end":   "2026-06-30T23:59:59Z",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	raw, ok := out["events"]
	if !ok {
		t.Fatalf("envelope missing 'events': %#v", out)
	}
	events, ok := raw.([]any)
	if !ok {
		t.Fatalf("events is not a slice: %T", raw)
	}

	// c1: weekly-Monday event expands to 5 occurrences in June 2026 (Mondays
	// 1,8,15,22,29). c2: one clear-text event + one garbage encrypted event
	// (skipped). Total = 6 occurrences, 1 skipped.
	if len(events) != 6 {
		t.Fatalf("expected 6 occurrences (5 from c1 + 1 from c2), got %d", len(events))
	}
	if skipped, _ := out["skipped"].(float64); skipped != 1 {
		t.Errorf("skipped: want 1 (garbage c2 event), got %v", out["skipped"])
	}

	// The merged result must be sorted ascending; c2's 08:00 clear event sorts
	// before all of c1's 09:00 occurrences.
	first, ok := events[0].(map[string]any)
	if !ok {
		t.Fatalf("events[0] is not a map: %T", events[0])
	}
	if first["calendar_id"] != "c2" {
		t.Errorf("events[0] calendar_id: want c2 (earliest start), got %q", first["calendar_id"])
	}
	if first["summary"] != "Team Sync" {
		t.Errorf("events[0] summary: want %q, got %q", "Team Sync", first["summary"])
	}

	// Verify the c1 occurrences are present, recurring, and carry the SUMMARY
	// from calEventsICS (proves real decryption + ICS parse ran).
	if !strings.Contains(calEventsICS, "SUMMARY:Weekly Standup") {
		t.Fatal("calEventsICS drifted from the expected SUMMARY; regenerate the cassette")
	}
	var c1Count int
	for _, e := range events {
		m, _ := e.(map[string]any)
		if m["calendar_id"] != "c1" {
			continue
		}
		c1Count++
		if m["summary"] != "Weekly Standup" {
			t.Errorf("c1 occurrence summary: want %q, got %q", "Weekly Standup", m["summary"])
		}
		if recurring, _ := m["is_recurring_instance"].(bool); !recurring {
			t.Errorf("c1 occurrence is_recurring_instance: want true, got %v", m["is_recurring_instance"])
		}
	}
	if c1Count != 5 {
		t.Errorf("expected 5 c1 occurrences, got %d", c1Count)
	}

	// Occurrences must be sorted ascending by start.
	var prev string
	for i, e := range events {
		m, _ := e.(map[string]any)
		s, _ := m["start"].(string)
		if i > 0 && s < prev {
			t.Fatalf("events not sorted: events[%d].start=%s < prev=%s", i, s, prev)
		}
		prev = s
	}
}

func TestGetEventHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "get_event_happy",
		testharness.WithSessionService(newFixedCalKeyring(t)))
	defer h.Close()

	out, err := h.Call(context.Background(), "proton_get_event", map[string]any{
		"calendar_id": "c1",
		"event_id":    "e1",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	ev, ok := out["event"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing 'event' map: %#v", out)
	}

	// get_event returns the master event, not expanded occurrences.
	if !strings.Contains(calEventsICS, "SUMMARY:Weekly Standup") {
		t.Fatal("calEventsICS drifted from the expected SUMMARY; regenerate the cassette")
	}
	if ev["summary"] != "Weekly Standup" {
		t.Errorf("summary: want %q, got %q", "Weekly Standup", ev["summary"])
	}
	if ev["location"] != "Room 1" {
		t.Errorf("location: want %q, got %q", "Room 1", ev["location"])
	}
	if ev["recurrence_rule"] != "FREQ=WEEKLY;BYDAY=MO" {
		t.Errorf("recurrence_rule: want %q, got %q", "FREQ=WEEKLY;BYDAY=MO", ev["recurrence_rule"])
	}
	if recurring, _ := ev["is_recurring_instance"].(bool); !recurring {
		t.Errorf("is_recurring_instance: want true, got %v", ev["is_recurring_instance"])
	}
	if ev["calendar_id"] != "c1" || ev["event_id"] != "e1" {
		t.Errorf("identifiers: got calendar_id=%q event_id=%q", ev["calendar_id"], ev["event_id"])
	}
}

// TestGetEventFetchError drives getEvent past the keyring resolution (a fixed
// in-memory keyring) onto an unrecorded event GET, exercising the
// GetCalendarEvent failure branch.
func TestGetEventFetchError(t *testing.T) {
	h := testharness.BootWithCassette(t, "whoami_happy",
		testharness.WithSessionService(newFixedCalKeyring(t)))
	defer h.Close()

	_, err := h.Call(context.Background(), "proton_get_event", map[string]any{
		"calendar_id": "c1",
		"event_id":    "missing",
	})
	if err == nil {
		t.Fatal("expected error when the event GET is unrecorded")
	}
	if !strings.Contains(err.Error(), "proton/") {
		t.Fatalf("error did not surface a proton/* code: %v", err)
	}
}

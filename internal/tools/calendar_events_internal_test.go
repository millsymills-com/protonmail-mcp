package tools

import (
	"strconv"
	"testing"
	"time"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/millsymills-com/protonmail-mcp/internal/calendar"
)

// syntheticCalKR generates a fresh x25519 calendar keyring for each test.
func syntheticCalKR(t *testing.T) *crypto.KeyRing {
	t.Helper()
	key, err := crypto.GenerateKey("cal", "cal@cal.test", "x25519", 0)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	kr, err := crypto.NewKeyRing(key)
	if err != nil {
		t.Fatalf("NewKeyRing: %v", err)
	}
	return kr
}

// encryptICS encrypts ics to calKR and returns an armored PGP message.
func encryptICS(t *testing.T, calKR *crypto.KeyRing, ics string) string {
	t.Helper()
	enc, err := calKR.Encrypt(crypto.NewPlainMessageFromString(ics), nil)
	if err != nil {
		t.Fatalf("encrypt ICS: %v", err)
	}
	armored, err := enc.GetArmored()
	if err != nil {
		t.Fatalf("armor ICS: %v", err)
	}
	return armored
}

// weeklyMonday returns an ICS VCALENDAR with a weekly-Monday RRULE.
const weeklyMondayICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
	"UID:weekly-1\r\n" +
	"SUMMARY:Weekly Standup\r\n" +
	"LOCATION:Room 1\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=MO\r\n" +
	"ATTENDEE;PARTSTAT=ACCEPTED:mailto:alice@example.test\r\n" +
	"ATTENDEE;PARTSTAT=DECLINED:mailto:bob@example.test\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR"

func TestOccurrencesFor_RecurringExpandsInWindow(t *testing.T) {
	calKR := syntheticCalKR(t)
	armored := encryptICS(t, calKR, weeklyMondayICS)

	// Base Monday 2026-06-01 09:00 UTC, 1-hour event.
	baseStart := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	baseEnd := baseStart.Add(time.Hour)
	winStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	winEnd := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	ev := proton.CalendarEvent{
		ID:         "e1",
		UID:        "weekly-1",
		CalendarID: "c1",
		StartTime:  baseStart.Unix(),
		EndTime:    baseEnd.Unix(),
		SharedEvents: []proton.CalendarEventPart{
			{Type: proton.CalendarEventTypeEncrypted, Data: armored},
		},
	}

	occ, trunc, err := occurrencesFor(ev, calKR, winStart, winEnd, 500)
	if err != nil {
		t.Fatalf("occurrencesFor: %v", err)
	}
	if trunc {
		t.Error("expected truncated=false with plenty of remaining budget")
	}
	// June 2026: Mondays are 1,8,15,22,29 → 5 occurrences.
	if len(occ) != 5 {
		t.Fatalf("expected 5 monday occurrences in June 2026, got %d", len(occ))
	}

	// Verify sort order.
	for i := 1; i < len(occ); i++ {
		if occ[i].Start < occ[i-1].Start {
			t.Fatalf("occurrences not sorted: occ[%d].Start=%s < occ[%d].Start=%s",
				i, occ[i].Start, i-1, occ[i-1].Start)
		}
	}

	// First occurrence.
	first := occ[0]
	if first.Start != "2026-06-01T09:00:00Z" {
		t.Errorf("first.Start: want 2026-06-01T09:00:00Z, got %s", first.Start)
	}
	if first.End != "2026-06-01T10:00:00Z" {
		t.Errorf("first.End: want 2026-06-01T10:00:00Z, got %s", first.End)
	}
	if first.Summary != "Weekly Standup" {
		t.Errorf("first.Summary: want %q, got %q", "Weekly Standup", first.Summary)
	}
	if first.Location != "Room 1" {
		t.Errorf("first.Location: want %q, got %q", "Room 1", first.Location)
	}
	if first.CalendarID != "c1" {
		t.Errorf("first.CalendarID: want c1, got %q", first.CalendarID)
	}
	if first.EventID != "e1" {
		t.Errorf("first.EventID: want e1, got %q", first.EventID)
	}
	if !first.Recurring {
		t.Error("first.Recurring: want true")
	}

	// Duration preserved across all occurrences (each must be 1 hour).
	for i, o := range occ {
		s, _ := time.Parse(time.RFC3339, o.Start)
		e, _ := time.Parse(time.RFC3339, o.End)
		if e.Sub(s) != time.Hour {
			t.Errorf("occ[%d]: duration want 1h, got %v", i, e.Sub(s))
		}
	}

	// Attendees.
	if len(first.Attendees) != 2 {
		t.Fatalf("first.Attendees: want 2, got %d", len(first.Attendees))
	}
	if first.Attendees[0].Email != "alice@example.test" {
		t.Errorf("attendee[0].Email: want alice@example.test, got %q", first.Attendees[0].Email)
	}
	if first.Attendees[0].Status != "ACCEPTED" {
		t.Errorf("attendee[0].Status: want ACCEPTED, got %q", first.Attendees[0].Status)
	}
	if first.Attendees[1].Email != "bob@example.test" {
		t.Errorf("attendee[1].Email: want bob@example.test, got %q", first.Attendees[1].Email)
	}
	if first.Attendees[1].Status != "DECLINED" {
		t.Errorf("attendee[1].Status: want DECLINED, got %q", first.Attendees[1].Status)
	}
}

func TestOccurrencesFor_TruncatesAtRemaining(t *testing.T) {
	calKR := syntheticCalKR(t)
	armored := encryptICS(t, calKR, weeklyMondayICS)

	baseStart := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	winStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	winEnd := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	ev := proton.CalendarEvent{
		ID:        "e1",
		StartTime: baseStart.Unix(),
		EndTime:   baseStart.Add(time.Hour).Unix(),
		SharedEvents: []proton.CalendarEventPart{
			{Type: proton.CalendarEventTypeEncrypted, Data: armored},
		},
	}

	occ, trunc, err := occurrencesFor(ev, calKR, winStart, winEnd, 2)
	if err != nil {
		t.Fatalf("occurrencesFor: %v", err)
	}
	if !trunc {
		t.Error("expected truncated=true when remaining=2 and window has 5 occurrences")
	}
	if len(occ) != 2 {
		t.Errorf("expected 2 occurrences (capped), got %d", len(occ))
	}
}

func TestOccurrencesFor_NonRecurringInWindow(t *testing.T) {
	calKR := syntheticCalKR(t)
	const singleICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"UID:single-1\r\nSUMMARY:One-off\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR"
	armored := encryptICS(t, calKR, singleICS)

	baseStart := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	winStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	winEnd := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	ev := proton.CalendarEvent{
		StartTime: baseStart.Unix(),
		EndTime:   baseStart.Add(time.Hour).Unix(),
		SharedEvents: []proton.CalendarEventPart{
			{Type: proton.CalendarEventTypeEncrypted, Data: armored},
		},
	}

	occ, trunc, err := occurrencesFor(ev, calKR, winStart, winEnd, 500)
	if err != nil {
		t.Fatalf("occurrencesFor: %v", err)
	}
	if trunc {
		t.Error("expected truncated=false for single event")
	}
	if len(occ) != 1 {
		t.Fatalf("expected 1 occurrence, got %d", len(occ))
	}
	if occ[0].Recurring {
		t.Error("non-recurring event should not have Recurring=true")
	}
}

func TestOccurrencesFor_AllDay(t *testing.T) {
	calKR := syntheticCalKR(t)
	const allDayICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"UID:allday-1\r\nSUMMARY:Holiday\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR"
	armored := encryptICS(t, calKR, allDayICS)

	baseStart := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	winStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	winEnd := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	ev := proton.CalendarEvent{
		FullDay:   true,
		StartTime: baseStart.Unix(),
		EndTime:   baseStart.Add(24 * time.Hour).Unix(),
		SharedEvents: []proton.CalendarEventPart{
			{Type: proton.CalendarEventTypeEncrypted, Data: armored},
		},
	}

	occ, _, err := occurrencesFor(ev, calKR, winStart, winEnd, 500)
	if err != nil {
		t.Fatalf("occurrencesFor: %v", err)
	}
	if len(occ) != 1 {
		t.Fatalf("expected 1 occurrence, got %d", len(occ))
	}
	if !occ[0].AllDay {
		t.Error("AllDay: want true for FullDay event")
	}
}

func TestOccurrencesFor_NonRecurringOutsideWindow(t *testing.T) {
	calKR := syntheticCalKR(t)
	const singleICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"UID:before-1\r\nSUMMARY:Before Window\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR"
	armored := encryptICS(t, calKR, singleICS)

	// Event is in May; window is June.
	baseStart := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	winStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	winEnd := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	ev := proton.CalendarEvent{
		StartTime: baseStart.Unix(),
		EndTime:   baseStart.Add(time.Hour).Unix(),
		SharedEvents: []proton.CalendarEventPart{
			{Type: proton.CalendarEventTypeEncrypted, Data: armored},
		},
	}

	occ, _, err := occurrencesFor(ev, calKR, winStart, winEnd, 500)
	if err != nil {
		t.Fatalf("occurrencesFor: %v", err)
	}
	if len(occ) != 0 {
		t.Errorf("expected 0 occurrences for event outside window, got %d", len(occ))
	}
}

func TestParseWindow_Valid(t *testing.T) {
	start, end, perr := parseWindow("2026-06-01T00:00:00Z", "2026-06-30T23:59:59Z")
	if perr != nil {
		t.Fatalf("parseWindow: %v", perr)
	}
	if start.Year() != 2026 || start.Month() != 6 || start.Day() != 1 {
		t.Errorf("start: unexpected %v", start)
	}
	if end.Year() != 2026 || end.Month() != 6 || end.Day() != 30 {
		t.Errorf("end: unexpected %v", end)
	}
}

func TestParseWindow_MissingStart(t *testing.T) {
	_, _, perr := parseWindow("", "2026-06-30T23:59:59Z")
	if perr == nil {
		t.Fatal("expected validation error for missing start")
	}
	if perr.Code != "proton/validation" {
		t.Errorf("code: want proton/validation, got %s", perr.Code)
	}
}

func TestParseWindow_MissingEnd(t *testing.T) {
	_, _, perr := parseWindow("2026-06-01T00:00:00Z", "")
	if perr == nil {
		t.Fatal("expected validation error for missing end")
	}
	if perr.Code != "proton/validation" {
		t.Errorf("code: want proton/validation, got %s", perr.Code)
	}
}

func TestParseWindow_BadStartFormat(t *testing.T) {
	_, _, perr := parseWindow("not-a-date", "2026-06-30T23:59:59Z")
	if perr == nil {
		t.Fatal("expected validation error for bad start format")
	}
	if perr.Code != "proton/validation" {
		t.Errorf("code: want proton/validation, got %s", perr.Code)
	}
}

func TestParseWindow_BadEndFormat(t *testing.T) {
	_, _, perr := parseWindow("2026-06-01T00:00:00Z", "not-a-date")
	if perr == nil {
		t.Fatal("expected validation error for bad end format")
	}
	if perr.Code != "proton/validation" {
		t.Errorf("code: want proton/validation, got %s", perr.Code)
	}
}

func TestParseWindow_EndBeforeStart(t *testing.T) {
	_, _, perr := parseWindow("2026-06-30T00:00:00Z", "2026-06-01T00:00:00Z")
	if perr == nil {
		t.Fatal("expected validation error for end before start")
	}
	if perr.Code != "proton/validation" {
		t.Errorf("code: want proton/validation, got %s", perr.Code)
	}
}

func TestCalendarWindowFilter(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)
	v := calendarWindowFilter(start, end)

	wantStart := strconv.FormatInt(start.Unix(), 10)
	wantEnd := strconv.FormatInt(end.Unix(), 10)
	if got := v.Get("Start"); got != wantStart {
		t.Errorf("Start: want %s, got %s", wantStart, got)
	}
	if got := v.Get("End"); got != wantEnd {
		t.Errorf("End: want %s, got %s", wantEnd, got)
	}
}

func TestToAttendeeDTOs_Nil(t *testing.T) {
	if out := toAttendeeDTOs(nil); out != nil {
		t.Errorf("expected nil for nil input, got %v", out)
	}
}

func TestToAttendeeDTOs_MapsFields(t *testing.T) {
	in := []calendar.Attendee{
		{Email: "a@example.test", Status: "ACCEPTED"},
		{Email: "b@example.test", Status: ""},
	}
	out := toAttendeeDTOs(in)
	if len(out) != 2 {
		t.Fatalf("want 2 attendees, got %d", len(out))
	}
	if out[0].Email != "a@example.test" || out[0].Status != "ACCEPTED" {
		t.Errorf("attendee[0]: got %+v", out[0])
	}
	if out[1].Email != "b@example.test" || out[1].Status != "" {
		t.Errorf("attendee[1]: got %+v", out[1])
	}
}

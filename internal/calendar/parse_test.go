package calendar_test

import (
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/calendar"
)

// fullICS uses a multi-day BYDAY so the test regresses the Prop.Text comma-split
// bug: a value of "FREQ=WEEKLY;BYDAY=MO,WE" must survive intact.
const fullICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" +
	"BEGIN:VEVENT\r\nUID:evt-1\r\nDTSTAMP:20260601T090000Z\r\n" +
	"DTSTART:20260601T090000Z\r\nSUMMARY:Standup\r\nLOCATION:Room 1\r\n" +
	"DESCRIPTION:Daily sync\r\nORGANIZER:mailto:lead@example.test\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=MO,WE\r\n" +
	"ATTENDEE;PARTSTAT=ACCEPTED:mailto:a@example.test\r\n" +
	"ATTENDEE;PARTSTAT=DECLINED:mailto:b@example.test\r\nEND:VEVENT\r\nEND:VCALENDAR"

func TestParseICS(t *testing.T) {
	f, err := calendar.ParseICS(fullICS)
	if err != nil {
		t.Fatalf("ParseICS: %v", err)
	}
	if f.UID != "evt-1" {
		t.Errorf("uid = %q", f.UID)
	}
	if f.Summary != "Standup" {
		t.Errorf("summary = %q", f.Summary)
	}
	if f.Location != "Room 1" {
		t.Errorf("location = %q", f.Location)
	}
	if f.Description != "Daily sync" {
		t.Errorf("description = %q", f.Description)
	}
	if f.RecurrenceRule != "FREQ=WEEKLY;BYDAY=MO,WE" {
		t.Errorf("rrule = %q (comma-split regression?)", f.RecurrenceRule)
	}
	if f.Organizer != "lead@example.test" {
		t.Errorf("organizer = %q", f.Organizer)
	}
	if len(f.Attendees) != 2 {
		t.Fatalf("attendees = %d, want 2", len(f.Attendees))
	}
	if f.Attendees[0].Email != "a@example.test" || f.Attendees[0].Status != "ACCEPTED" {
		t.Errorf("attendee[0] = %+v", f.Attendees[0])
	}
	if f.Attendees[1].Email != "b@example.test" || f.Attendees[1].Status != "DECLINED" {
		t.Errorf("attendee[1] = %+v", f.Attendees[1])
	}
}

// TestParseICS_BareVEvent covers ensureVCalendar: a card with no VCALENDAR
// wrapper must still parse.
func TestParseICS_BareVEvent(t *testing.T) {
	bare := "BEGIN:VEVENT\r\nUID:evt-2\r\nDTSTAMP:20260601T090000Z\r\n" +
		"DTSTART:20260601T090000Z\r\nSUMMARY:Bare\r\nEND:VEVENT"
	f, err := calendar.ParseICS(bare)
	if err != nil {
		t.Fatalf("ParseICS bare: %v", err)
	}
	if f.Summary != "Bare" {
		t.Errorf("summary = %q", f.Summary)
	}
}

// TestParseICS_AttendeeNoPartstat confirms an attendee line without a PARTSTAT
// param parses to an empty Status rather than erroring.
func TestParseICS_AttendeeNoPartstat(t *testing.T) {
	ics := "BEGIN:VEVENT\r\nUID:evt-3\r\nDTSTAMP:20260601T090000Z\r\n" +
		"DTSTART:20260601T090000Z\r\nSUMMARY:NoStatus\r\n" +
		"ATTENDEE:mailto:c@example.test\r\nEND:VEVENT"
	f, err := calendar.ParseICS(ics)
	if err != nil {
		t.Fatalf("ParseICS: %v", err)
	}
	if len(f.Attendees) != 1 {
		t.Fatalf("attendees = %d, want 1", len(f.Attendees))
	}
	if f.Attendees[0].Email != "c@example.test" || f.Attendees[0].Status != "" {
		t.Errorf("attendee[0] = %+v", f.Attendees[0])
	}
}

func TestParseICS_Empty(t *testing.T) {
	_, err := calendar.ParseICS("")
	if err == nil {
		t.Fatal("expected error parsing empty ICS")
	}
}

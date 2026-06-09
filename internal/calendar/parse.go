package calendar

import (
	"fmt"
	"strings"

	"github.com/emersion/go-ical"
)

// Attendee is a calendar event participant from the decrypted ICS.
type Attendee struct {
	Email  string
	Status string // ICS PARTSTAT, e.g. ACCEPTED, DECLINED, TENTATIVE, NEEDS-ACTION
}

// ICSFields are the text fields parsed out of a decrypted event's ICS body.
// Times are intentionally absent: they come from the unencrypted API row.
type ICSFields struct {
	UID            string
	Summary        string
	Description    string
	Location       string
	Organizer      string
	RecurrenceRule string
	Attendees      []Attendee
}

// ParseICS parses a decrypted iCalendar string into its text fields. It reads
// the first VEVENT; events with no VEVENT yield an error.
func ParseICS(ics string) (ICSFields, error) {
	cal, err := ical.NewDecoder(strings.NewReader(ensureVCalendar(ics))).Decode()
	if err != nil {
		return ICSFields{}, fmt.Errorf("decode ics: %w", err)
	}
	events := cal.Events()
	if len(events) == 0 {
		return ICSFields{}, fmt.Errorf("ics has no VEVENT")
	}
	ev := events[0]

	f := ICSFields{
		UID:         text(ev.Props, ical.PropUID),
		Summary:     text(ev.Props, ical.PropSummary),
		Description: text(ev.Props, ical.PropDescription),
		Location:    text(ev.Props, ical.PropLocation),
		// ORGANIZER (CAL-ADDRESS) and RRULE (RECUR) are not TEXT values: Prop.Text
		// runs TextList, which comma-splits and would truncate an RRULE such as
		// BYDAY=MO,WE. Read the raw value instead.
		Organizer:      mailto(raw(ev.Props, ical.PropOrganizer)),
		RecurrenceRule: raw(ev.Props, ical.PropRecurrenceRule),
	}
	for _, p := range ev.Props.Values(ical.PropAttendee) {
		f.Attendees = append(f.Attendees, Attendee{
			Email:  mailto(p.Value),
			Status: p.Params.Get("PARTSTAT"),
		})
	}
	return f, nil
}

// ensureVCalendar wraps a bare VEVENT card in a minimal VCALENDAR envelope so
// the decoder (which requires a VCALENDAR root) accepts cards Proton may return
// without the wrapper. A card that already has the wrapper passes through. The
// actual Proton card shape is confirmed in the Task 9 live tracer.
func ensureVCalendar(ics string) string {
	if strings.Contains(ics, "BEGIN:VCALENDAR") {
		return ics
	}
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//protonmail-mcp//EN\r\n" + ics + "\r\nEND:VCALENDAR"
}

// text returns the named TEXT property's unescaped value, or "" if absent or
// unreadable.
func text(props ical.Props, name string) string {
	s, err := props.Text(name)
	if err != nil {
		return ""
	}
	return s
}

// raw returns the named property's unparsed value, or "" if absent. Used for
// non-TEXT values (RRULE, ORGANIZER) that Prop.Text would mangle.
func raw(props ical.Props, name string) string {
	if p := props.Get(name); p != nil {
		return p.Value
	}
	return ""
}

// mailto strips a leading "mailto:" (case-insensitive) from an ICS CAL-ADDRESS.
func mailto(s string) string {
	if len(s) >= 7 && strings.EqualFold(s[:7], "mailto:") {
		return s[7:]
	}
	return s
}

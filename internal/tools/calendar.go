package tools

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"time"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/millsymills-com/protonmail-mcp/internal/calendar"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxOccurrences = 500

type calendarDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
	Type        string `json:"type"`
	Active      bool   `json:"active"`
	LostAccess  bool   `json:"lost_access,omitempty"`
}

type attendeeDTO struct {
	Email  string `json:"email"`
	Status string `json:"status,omitempty"`
}

type eventOccurrenceDTO struct {
	CalendarID string        `json:"calendar_id"`
	EventID    string        `json:"event_id"`
	UID        string        `json:"uid,omitempty"`
	Summary    string        `json:"summary,omitempty"`
	Location   string        `json:"location,omitempty"`
	Start      string        `json:"start"` // RFC3339 UTC
	End        string        `json:"end"`   // RFC3339 UTC
	AllDay     bool          `json:"all_day,omitempty"`
	Recurring  bool          `json:"is_recurring_instance,omitempty"`
	Attendees  []attendeeDTO `json:"attendees,omitempty"`
}

type listCalendarsIn struct{}
type listCalendarsOut struct {
	Calendars []calendarDTO `json:"calendars"`
}

type listEventsIn struct {
	Start      string `json:"start" jsonschema:"window start, RFC3339"`
	End        string `json:"end" jsonschema:"window end, RFC3339"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"restrict to one calendar; all active calendars when omitted"`
}
type listEventsOut struct {
	Events    []eventOccurrenceDTO `json:"events"`
	Truncated bool                 `json:"truncated,omitempty"`
	Skipped   int                  `json:"skipped,omitempty"`
}

func registerCalendar(server *mcp.Server, d Deps) {
	addTool(server, d, &mcp.Tool{
		Name:        "proton_list_calendars",
		Description: "Lists the user's Proton calendars (id, name, color, type, active/lost-access status). Read-only.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, listCalendars)

	addTool(server, d, &mcp.Tool{
		Name:        "proton_list_events",
		Description: "Lists calendar events in a time window (RFC3339 start/end). Recurring events are expanded to concrete occurrences. calendar_id is optional; all active calendars are searched when omitted. Read-only.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, listEvents)

	addTool(server, d, &mcp.Tool{
		Name:        "proton_get_event",
		Description: "Returns one calendar event's full decrypted detail (summary, description, location, recurrence rule, attendees, organizer). Returns the master event; use proton_list_events for occurrences. Read-only.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, getEvent)
}

func listCalendars(ctx context.Context, d Deps, _ listCalendarsIn) (listCalendarsOut, *proterr.Error) {
	c, perr := client(ctx, d)
	if perr != nil {
		return listCalendarsOut{}, perr
	}
	cals, err := c.GetCalendars(ctx)
	if err != nil {
		return listCalendarsOut{}, proterr.Map(err)
	}
	out := make([]calendarDTO, len(cals))
	for i, cal := range cals {
		out[i] = toCalendarDTO(cal)
	}
	return listCalendarsOut{Calendars: out}, nil
}

func listEvents(ctx context.Context, d Deps, in listEventsIn) (listEventsOut, *proterr.Error) {
	start, end, perr := parseWindow(in.Start, in.End)
	if perr != nil {
		return listEventsOut{}, perr
	}
	c, perr := client(ctx, d)
	if perr != nil {
		return listEventsOut{}, perr
	}

	calIDs, perr := targetCalendars(ctx, c, in.CalendarID)
	if perr != nil {
		return listEventsOut{}, perr
	}

	var all []eventOccurrenceDTO
	truncated := false
	skipped := 0
outer:
	for _, calID := range calIDs {
		calKR, err := d.Session.CalendarKeyring(ctx, calID)
		if err != nil {
			return listEventsOut{}, proterr.Map(err)
		}
		filter := calendarWindowFilter(start, end)
		events, err := c.GetAllCalendarEvents(ctx, calID, filter)
		if err != nil {
			return listEventsOut{}, proterr.Map(err)
		}
		for _, ev := range events {
			occ, trunc, err := occurrencesFor(ev, calKR, start, end, maxOccurrences-len(all))
			if err != nil {
				// A single corrupt/undecryptable event must not blank the whole
				// listing; count it and move on. Keyring and transport failures
				// above stay fatal — those are calendar-level, not one bad event.
				skipped++
				continue
			}
			truncated = truncated || trunc
			all = append(all, occ...)
			if len(all) >= maxOccurrences {
				truncated = true
				all = all[:maxOccurrences]
				break outer
			}
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Start < all[j].Start })
	return listEventsOut{Events: all, Truncated: truncated, Skipped: skipped}, nil
}

// occurrencesFor decrypts ev, expands its RRULE within [start,end], and
// projects each occurrence into a DTO. Event duration (from the API row) is
// preserved per occurrence.
func occurrencesFor(ev proton.CalendarEvent, calKR *crypto.KeyRing, start, end time.Time, remaining int) ([]eventOccurrenceDTO, bool, error) {
	ics, err := calendar.DecryptSharedICS(ev, calKR)
	if err != nil {
		return nil, false, err
	}
	fields, err := calendar.ParseICS(ics)
	if err != nil {
		return nil, false, err
	}
	base := time.Unix(ev.StartTime, 0).UTC()
	duration := time.Unix(ev.EndTime, 0).Sub(time.Unix(ev.StartTime, 0))
	starts, trunc, err := calendar.Expand(fields.RecurrenceRule, base, start, end, remaining)
	if err != nil {
		return nil, false, err
	}
	out := make([]eventOccurrenceDTO, 0, len(starts))
	for _, s := range starts {
		out = append(out, eventOccurrenceDTO{
			CalendarID: ev.CalendarID,
			EventID:    ev.ID,
			UID:        ev.UID,
			Summary:    fields.Summary,
			Location:   fields.Location,
			Start:      s.UTC().Format(time.RFC3339),
			End:        s.Add(duration).UTC().Format(time.RFC3339),
			AllDay:     bool(ev.FullDay),
			Recurring:  fields.RecurrenceRule != "",
			Attendees:  toAttendeeDTOs(fields.Attendees),
		})
	}
	return out, trunc, nil
}

type eventDetailDTO struct {
	eventOccurrenceDTO
	Description    string `json:"description,omitempty"`
	Organizer      string `json:"organizer,omitempty"`
	RecurrenceRule string `json:"recurrence_rule,omitempty"`
}

type getEventIn struct {
	CalendarID string `json:"calendar_id" jsonschema:"the calendar holding the event"`
	EventID    string `json:"event_id" jsonschema:"the event to fetch"`
}
type getEventOut struct {
	Event eventDetailDTO `json:"event"`
}

func getEvent(ctx context.Context, d Deps, in getEventIn) (getEventOut, *proterr.Error) {
	if perr := required("calendar_id", in.CalendarID); perr != nil {
		return getEventOut{}, perr
	}
	if perr := required("event_id", in.EventID); perr != nil {
		return getEventOut{}, perr
	}
	c, perr := client(ctx, d)
	if perr != nil {
		return getEventOut{}, perr
	}
	calKR, err := d.Session.CalendarKeyring(ctx, in.CalendarID)
	if err != nil {
		return getEventOut{}, proterr.Map(err)
	}
	ev, err := c.GetCalendarEvent(ctx, in.CalendarID, in.EventID)
	if err != nil {
		return getEventOut{}, proterr.Map(err)
	}
	ics, err := calendar.DecryptSharedICS(ev, calKR)
	if err != nil {
		return getEventOut{}, proterr.Map(err)
	}
	fields, err := calendar.ParseICS(ics)
	if err != nil {
		return getEventOut{}, proterr.Map(err)
	}
	base := time.Unix(ev.StartTime, 0).UTC()
	endT := time.Unix(ev.EndTime, 0).UTC()
	return getEventOut{Event: eventDetailDTO{
		eventOccurrenceDTO: eventOccurrenceDTO{
			CalendarID: ev.CalendarID,
			EventID:    ev.ID,
			UID:        ev.UID,
			Summary:    fields.Summary,
			Location:   fields.Location,
			Start:      base.Format(time.RFC3339),
			End:        endT.Format(time.RFC3339),
			AllDay:     bool(ev.FullDay),
			Recurring:  fields.RecurrenceRule != "",
			Attendees:  toAttendeeDTOs(fields.Attendees),
		},
		Description:    fields.Description,
		Organizer:      fields.Organizer,
		RecurrenceRule: fields.RecurrenceRule,
	}}, nil
}

func toAttendeeDTOs(in []calendar.Attendee) []attendeeDTO {
	if len(in) == 0 {
		return nil
	}
	out := make([]attendeeDTO, len(in))
	for i, a := range in {
		out[i] = attendeeDTO{Email: a.Email, Status: a.Status}
	}
	return out
}

// parseWindow validates the RFC3339 window bounds.
func parseWindow(startStr, endStr string) (time.Time, time.Time, *proterr.Error) {
	if perr := required("start", startStr); perr != nil {
		return time.Time{}, time.Time{}, perr
	}
	if perr := required("end", endStr); perr != nil {
		return time.Time{}, time.Time{}, perr
	}
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return time.Time{}, time.Time{}, &proterr.Error{Code: "proton/validation", Message: "start must be RFC3339"}
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return time.Time{}, time.Time{}, &proterr.Error{Code: "proton/validation", Message: "end must be RFC3339"}
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, &proterr.Error{Code: "proton/validation", Message: "end must be after start"}
	}
	return start, end, nil
}

// targetCalendars returns the requested calendar ID, or every active calendar
// when none is given. Calendars with lost access are skipped.
func targetCalendars(ctx context.Context, c *proton.Client, calendarID string) ([]string, *proterr.Error) {
	if calendarID != "" {
		return []string{calendarID}, nil
	}
	cals, err := c.GetCalendars(ctx)
	if err != nil {
		return nil, proterr.Map(err)
	}
	var ids []string
	for _, cal := range cals {
		if cal.Flags&proton.CalendarFlagActive != 0 && cal.Flags&proton.CalendarFlagLostAccess == 0 {
			ids = append(ids, cal.ID)
		}
	}
	return ids, nil
}

// calendarWindowFilter builds the events query filter for a time window. The
// upstream endpoint accepts Start/End unix-second bounds.
func calendarWindowFilter(start, end time.Time) url.Values {
	v := url.Values{}
	v.Set("Start", strconv.FormatInt(start.Unix(), 10))
	v.Set("End", strconv.FormatInt(end.Unix(), 10))
	return v
}

func toCalendarDTO(c proton.Calendar) calendarDTO {
	typ := "normal"
	if c.Type == proton.CalendarTypeSubscribed {
		typ = "subscribed"
	}
	return calendarDTO{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		Color:       c.Color,
		Type:        typ,
		Active:      c.Flags&proton.CalendarFlagActive != 0,
		LostAccess:  c.Flags&proton.CalendarFlagLostAccess != 0,
	}
}

//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/millsymills-com/protonmail-mcp/internal/calendar"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
)

func registerCalendarFlows() {
	Register("calendar_list_events_happy", recordCalendarListEvents)
}

// recordCalendarListEvents records the live list_events read path (#196) against
// a real, keyring-unlockable account whose calendar holds at least one event
// (ideally one recurring). It issues the same calls the proton_list_events
// handler makes — GetCalendars, then GetAllCalendarEvents with a unix-second
// Start/End window filter — so the cassette captures the real time-window
// query-param names (unconfirmed item 1), then decrypts the first event at
// record time and logs the VCALENDAR wrapper shape (unconfirmed item 2) for the
// operator to confirm bare VEVENT vs full BEGIN:VCALENDAR…END.
//
// Preconditions the operator must satisfy before recording (cannot be
// unattended): an interactive `protonmail-mcp login` on an account whose session
// unlocks the mailbox keyring (self-heals post-#195), and a calendar containing
// events — add a recurring event in the Proton Calendar UI first, since this
// server has no calendar-write tool.
func recordCalendarListEvents(ctx context.Context) error {
	return recordRawTool(ctx, "calendar_list_events_happy", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
			c, err := s.Client(ctx)
			if err != nil {
				return fmt.Errorf("client: %w", err)
			}
			cals, err := c.GetCalendars(ctx)
			if err != nil {
				return fmt.Errorf("get calendars: %w", err)
			}
			// A fixed wide window keeps re-records byte-stable: the trace pins the
			// param NAMES, not the bounds, so any window that captures the events
			// works and a constant one avoids spurious cassette drift.
			filter := url.Values{}
			filter.Set("Start", strconv.FormatInt(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), 10))
			filter.Set("End", strconv.FormatInt(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), 10))

			for _, cal := range cals {
				events, err := c.GetAllCalendarEvents(ctx, cal.ID, filter)
				if err != nil {
					return fmt.Errorf("get events for calendar %s: %w", cal.ID, err)
				}
				if len(events) == 0 {
					continue
				}
				calKR, err := s.CalendarKeyring(ctx, cal.ID)
				if err != nil {
					return fmt.Errorf("calendar keyring %s: %w", cal.ID, err)
				}
				ics, err := calendar.DecryptSharedICS(events[0], calKR)
				if err != nil {
					return fmt.Errorf("decrypt first event: %w", err)
				}
				logWrapperShape(ics)
				return nil
			}
			return fmt.Errorf("no calendar with events found; add a calendar event first (see #196)")
		})
}

// logWrapperShape reports whether Proton's decrypted card is a full
// BEGIN:VCALENDAR…END envelope or a bare VEVENT — the open question in #196.
// The operator reconciles calEventsICS in the synthetic list_events cassette
// test to whichever shape this reports.
func logWrapperShape(ics string) {
	shape := "bare VEVENT"
	if strings.HasPrefix(strings.TrimSpace(ics), "BEGIN:VCALENDAR") {
		shape = "wrapped BEGIN:VCALENDAR…END"
	}
	slog.Info("calendar live trace: decrypted card shape observed",
		"shape", shape,
		"has_rrule", strings.Contains(ics, "RRULE:"))
}

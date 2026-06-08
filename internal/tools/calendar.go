package tools

import (
	"context"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type calendarDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
	Type        string `json:"type"`
	Active      bool   `json:"active"`
	LostAccess  bool   `json:"lost_access,omitempty"`
}

type listCalendarsIn struct{}
type listCalendarsOut struct {
	Calendars []calendarDTO `json:"calendars"`
}

func registerCalendar(server *mcp.Server, d Deps) {
	addTool(server, d, &mcp.Tool{
		Name:        "proton_list_calendars",
		Description: "Lists the user's Proton calendars (id, name, color, type, active/lost-access status). Read-only.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, listCalendars)
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

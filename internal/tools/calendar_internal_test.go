package tools

import (
	"reflect"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
)

func TestToCalendarDTO(t *testing.T) {
	tests := []struct {
		name string
		in   proton.Calendar
		want calendarDTO
	}{
		{
			name: "no-flags",
			in:   proton.Calendar{ID: "c1", Name: "Personal", Type: proton.CalendarTypeNormal, Flags: 0},
			want: calendarDTO{ID: "c1", Name: "Personal", Type: "normal", Active: false, LostAccess: false},
		},
		{
			name: "active",
			in:   proton.Calendar{ID: "c2", Name: "Work", Flags: proton.CalendarFlagActive},
			want: calendarDTO{ID: "c2", Name: "Work", Type: "normal", Active: true, LostAccess: false},
		},
		{
			name: "active-and-lost-access",
			in: proton.Calendar{
				ID:    "c3",
				Name:  "Shared",
				Flags: proton.CalendarFlagActive | proton.CalendarFlagLostAccess,
			},
			want: calendarDTO{ID: "c3", Name: "Shared", Type: "normal", Active: true, LostAccess: true},
		},
		{
			name: "subscribed-type",
			in:   proton.Calendar{ID: "c4", Name: "Holidays", Type: proton.CalendarTypeSubscribed},
			want: calendarDTO{ID: "c4", Name: "Holidays", Type: "subscribed"},
		},
		{
			name: "unknown-type-falls-back-to-normal",
			in:   proton.Calendar{ID: "c5", Name: "Mystery", Type: proton.CalendarType(99)},
			want: calendarDTO{ID: "c5", Name: "Mystery", Type: "normal"},
		},
		{
			name: "description-populated",
			in:   proton.Calendar{ID: "c6", Name: "Feed", Description: "A subscribed feed", Color: "#10b981"},
			want: calendarDTO{ID: "c6", Name: "Feed", Description: "A subscribed feed", Color: "#10b981", Type: "normal"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toCalendarDTO(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("toCalendarDTO mismatch\n got: %+v\nwant: %+v", got, tc.want)
			}
		})
	}
}

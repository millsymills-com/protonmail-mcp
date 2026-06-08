package calendar_test

import (
	"testing"
	"time"

	"github.com/millsymills-com/protonmail-mcp/internal/calendar"
)

func TestExpand_Weekly(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC) // a Monday
	winStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	winEnd := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	occ, truncated, err := calendar.Expand("FREQ=WEEKLY;BYDAY=MO", start, winStart, winEnd, 100)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if truncated {
		t.Error("did not expect truncation")
	}
	if len(occ) != 5 { // Mondays Jun 1, 8, 15, 22, 29 fall in [Jun 1, Jun 30]
		t.Fatalf("occurrences = %d, want 5", len(occ))
	}
	if !occ[0].Equal(start) {
		t.Errorf("first occurrence = %v, want %v", occ[0], start)
	}
}

func TestExpand_NoRule(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	win := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	occ, _, err := calendar.Expand("", start, start.Add(-time.Hour), win, 100)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(occ) != 1 || !occ[0].Equal(start) {
		t.Fatalf("non-recurring expansion = %v, want [%v]", occ, start)
	}
}

func TestExpand_Cap(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	winEnd := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	occ, truncated, err := calendar.Expand("FREQ=DAILY", start, start.Add(-time.Hour), winEnd, 10)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if !truncated {
		t.Error("expected truncation")
	}
	if len(occ) != 10 {
		t.Fatalf("occurrences = %d, want cap 10", len(occ))
	}
}

func TestExpand_LimitZero(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	occ, truncated, err := calendar.Expand("FREQ=WEEKLY;BYDAY=MO", start, start.Add(-time.Hour), end, 0)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(occ) != 0 || !truncated {
		t.Fatalf("limit 0 = (%d occ, truncated=%v), want (0, true)", len(occ), truncated)
	}
}

func TestExpand_DtstartBeforeWindow(t *testing.T) {
	start := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC) // Monday, before window
	winStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	winEnd := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	occ, truncated, err := calendar.Expand("FREQ=WEEKLY;BYDAY=MO", start, winStart, winEnd, 100)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if truncated {
		t.Error("unexpected truncation")
	}
	if len(occ) != 5 {
		t.Fatalf("occurrences = %d, want 5 (Jun Mondays)", len(occ))
	}
	if !occ[0].Equal(time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("first occurrence = %v, want Jun 1 09:00", occ[0])
	}
}

func TestExpand_BadRule(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	if _, _, err := calendar.Expand("FREQ=NOPE", start, start, start.Add(48*time.Hour), 10); err == nil {
		t.Fatal("expected error for malformed rule")
	}
}

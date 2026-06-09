package calendar

import (
	"fmt"
	"time"

	"github.com/teambition/rrule-go"
)

// Expand returns the event start times that fall within [winStart, winEnd].
// rule is a bare RRULE value (no "RRULE:" prefix); when empty the event is
// non-recurring and the single start time is returned if it falls in the
// window. Expansion stops at limit occurrences, returning truncated=true when
// the rule would have produced more.
func Expand(rule string, start, winStart, winEnd time.Time, limit int) ([]time.Time, bool, error) {
	if rule == "" {
		if !start.Before(winStart) && !start.After(winEnd) {
			return []time.Time{start}, false, nil
		}
		return nil, false, nil
	}

	opt, err := rrule.StrToROption("RRULE:" + rule)
	if err != nil {
		return nil, false, fmt.Errorf("parse rrule: %w", err)
	}
	opt.Dtstart = start
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return nil, false, fmt.Errorf("build rrule: %w", err)
	}

	// Iterate rather than r.Between so collection stops at limit+1 instead of
	// expanding the whole window: a high-frequency rule or a small limit would
	// otherwise allocate every in-window occurrence just to discard most.
	next := r.Iterator()
	var occ []time.Time
	for {
		v, ok := next()
		if !ok || v.After(winEnd) {
			return occ, false, nil
		}
		if v.Before(winStart) {
			continue
		}
		if len(occ) >= limit {
			return occ, true, nil
		}
		occ = append(occ, v)
	}
}

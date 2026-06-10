package tools_test

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// liveCalendarCassette is the live-recorded list_events trace (#196). It is
// absent until an operator records it against a real keyring-unlockable account
// whose calendar holds events:
//
//	make record SCENARIO=calendar_list_events_happy
//
// Until then this test skips. Once the cassette lands it pins the live-confirmed
// time-window query-param names (Start/End) that the synthetic list_events_happy
// cassette could only assume from upstream docs — unconfirmed item 1 in #196.
// (Unconfirmed item 2, the decrypted VCALENDAR wrapper shape, cannot be pinned
// offline without the account's private key; the recorder logs the observed
// shape at record time and the operator reconciles calEventsICS to match.)
const liveCalendarCassette = "testdata/cassettes/calendar_list_events_happy.yaml"

func TestCalendarListEventsLiveWindowParams(t *testing.T) {
	data, err := os.ReadFile(liveCalendarCassette)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("live cassette not recorded yet (see #196); run: make record SCENARIO=calendar_list_events_happy")
	}
	if err != nil {
		t.Fatalf("read live cassette: %v", err)
	}

	var cas struct {
		Interactions []struct {
			Request struct {
				URL string `yaml:"url"`
			} `yaml:"request"`
		} `yaml:"interactions"`
	}
	if err := yaml.Unmarshal(data, &cas); err != nil {
		t.Fatalf("parse live cassette: %v", err)
	}

	var verified int
	for _, in := range cas.Interactions {
		u, err := url.Parse(in.Request.URL)
		if err != nil || !strings.Contains(u.Path, "/calendar/") || !strings.HasSuffix(u.Path, "/events") {
			continue
		}
		verified++
		q := u.Query()
		if !q.Has("Start") || !q.Has("End") {
			t.Errorf("live events request %s missing Start/End window params; got query %q", u.Path, u.RawQuery)
		}
	}
	if verified == 0 {
		t.Fatalf("live cassette %s has no /calendar/.../events request to verify", liveCalendarCassette)
	}
}

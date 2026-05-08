package testharness

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
)

// Fixture is one recorded request/response pair. Used by WithFixture to
// replay canned responses for endpoints whose dev-server semantics diverge
// from production (custom domain verification state machine, catchall, etc).
type Fixture struct {
	Request struct {
		Method string         `json:"method"`
		Path   string         `json:"path"`
		Body   map[string]any `json:"body,omitempty"`
	} `json:"request"`
	Response struct {
		Status int            `json:"status"`
		Body   map[string]any `json:"body"`
	} `json:"response"`
}

// LoadFixtures parses each path and returns the loaded fixtures.
func LoadFixtures(paths ...string) ([]Fixture, error) {
	out := make([]Fixture, 0, len(paths))
	for _, p := range paths {
		//nolint:gosec // G304: paths are caller-supplied test fixture files, not user input
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var f Fixture
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// WithFixture installs an interceptor that matches each incoming request to
// the first fixture whose method+path equals the request's, then serves its
// canned response. A request with no match falls through to the dev server.
func WithFixture(fixtures ...Fixture) Option {
	return WithInterceptor(func(r *http.Request) *http.Response {
		for _, f := range fixtures {
			if !strings.EqualFold(f.Request.Method, r.Method) {
				continue
			}
			if f.Request.Path != r.URL.Path {
				continue
			}
			body, _ := json.Marshal(f.Response.Body)
			return &http.Response{
				StatusCode: f.Response.Status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(body)),
			}
		}
		return nil
	})
}

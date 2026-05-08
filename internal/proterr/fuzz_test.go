package proterr_test

import (
	"net/http"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
)

func FuzzProterrMapping(f *testing.F) {
	// Seed the corpus with status codes we know about plus boundary values.
	for _, s := range []int{0, 200, 400, 401, 402, 404, 409, 422, 429, 500, 502, 503, 999, -1} {
		f.Add(s, []byte("{}"))
	}
	f.Add(401, []byte(`{"Code":2024,"Error":"human verification required"}`))
	f.Add(429, []byte(`{"Code":2001,"Error":"rate limited"}`))

	f.Fuzz(func(t *testing.T, status int, body []byte) {
		// Must never panic regardless of input.
		//nolint:errcheck // panic-safety: proterr.Map should never panic
		_ = proterr.Map(&proton.APIError{Status: status, Message: string(body)})
		//nolint:errcheck // panic-safety: proterr.Map should never panic
		_ = proterr.Map(&proterr.HTTPError{Status: status, Headers: http.Header{}})
	})
}

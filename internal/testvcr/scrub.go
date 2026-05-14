package testvcr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
)

var sensitiveJSONKeys = map[string]bool{
	"AccessToken":     true,
	"RefreshToken":    true,
	"UID":             true,
	"KeySalt":         true,
	"PrivateKey":      true,
	"Signature":       true,
	"Token":           true,
	"SrpSession":      true,
	"ServerProof":     true,
	"ClientProof":     true,
	"ClientEphemeral": true,
	"TwoFactorCode":   true,
}

var redactedHeaders = []string{"Authorization", "X-Pm-Uid", "Cookie", "Set-Cookie"}

func saveHook(i *cassette.Interaction) error {
	for _, h := range redactedHeaders {
		if i.Request.Headers.Get(h) != "" {
			i.Request.Headers.Set(h, "REDACTED")
		}
		if i.Response.Headers.Get(h) != "" {
			i.Response.Headers.Set(h, "REDACTED")
		}
	}
	reqBody, err := newBodyScrubber().scrub(i.Request.Body, i.Request.Headers.Get("Content-Type"))
	if err != nil {
		return fmt.Errorf("scrub request body: %w", err)
	}
	i.Request.Body = reqBody
	respBody, err := newBodyScrubber().scrub(i.Response.Body, i.Response.Headers.Get("Content-Type"))
	if err != nil {
		return fmt.Errorf("scrub response body: %w", err)
	}
	i.Response.Body = respBody
	return nil
}

type bodyScrubber struct {
	counters        map[string]int
	email           string
	domain          string
	throwawayDomain string
}

func newBodyScrubber() *bodyScrubber {
	return &bodyScrubber{
		counters:        map[string]int{},
		email:           strings.TrimSpace(os.Getenv("RECORD_EMAIL")),
		domain:          strings.TrimSpace(os.Getenv("RECORD_DOMAIN")),
		throwawayDomain: strings.TrimSpace(os.Getenv("RECORD_THROWAWAY_DOMAIN")),
	}
}

func (s *bodyScrubber) scrub(body, contentType string) (string, error) {
	if body == "" {
		return body, nil
	}
	if strings.Contains(contentType, "application/json") ||
		strings.HasPrefix(strings.TrimSpace(body), "{") {
		var v any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			// Preserve identifier scrub even when body is mislabelled as JSON.
			return s.rewriteIdentifiers(body), nil
		}
		s.walk(v)
		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return "", err
		}
		out := strings.TrimRight(buf.String(), "\n")
		return s.rewriteIdentifiers(out), nil
	}
	return s.rewriteIdentifiers(body), nil
}

func (s *bodyScrubber) walk(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			if sensitiveJSONKeys[k] {
				if _, ok := vv.(string); ok {
					// Counter tracks per-key occurrences; placeholder format: REDACTED_<KEY>_<N>.
					s.counters[k]++
					t[k] = fmt.Sprintf("REDACTED_%s_%d", strings.ToUpper(k), s.counters[k])
					continue
				}
			}
			s.walk(vv)
		}
	case []any:
		for _, item := range t {
			s.walk(item)
		}
	}
}

func (s *bodyScrubber) rewriteIdentifiers(in string) string {
	out := in
	if s.email != "" {
		out = strings.ReplaceAll(out, s.email, "user@example.test")
	}
	if s.throwawayDomain != "" {
		out = strings.ReplaceAll(out, s.throwawayDomain, "throwaway.example.test")
	}
	if s.domain != "" {
		out = strings.ReplaceAll(out, s.domain, "example.test")
	}
	return out
}

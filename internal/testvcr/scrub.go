package testvcr

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/yaml.v3"
)

// sensitiveJSONKeys are compared against incoming JSON keys via
// strings.EqualFold so casing variations like "UID" vs "Uid" and
// "accessToken" vs "AccessToken" are all caught. Proton returns both
// "UID" and "Uid" in some payloads (e.g. /auth/v4/refresh) and a
// case-sensitive lookup missed the lowercase form.
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
	"RecoverySecret":  true,
	"Fingerprint":     true,
}

func isSensitiveJSONKey(k string) bool {
	for sk := range sensitiveJSONKeys {
		if strings.EqualFold(sk, k) {
			return true
		}
	}
	return false
}

// base64ValuedJSONKey reports whether a sensitive key holds a base64-encoded
// value that consumer code will try to base64-decode at use. Their REDACTED
// placeholder needs to round-trip through base64 (encode → decode) cleanly
// or the consumer errors before any guard like WithSkipVerifyProofs runs.
var base64ValuedKeys = map[string]bool{
	"ServerProof":     true,
	"ClientProof":     true,
	"ClientEphemeral": true,
	"Signature":       true,
}

func base64ValuedJSONKey(k string) bool {
	for sk := range base64ValuedKeys {
		if strings.EqualFold(sk, k) {
			return true
		}
	}
	return false
}

var redactedHeaders = []string{"Authorization", "X-Pm-Uid", "Cookie", "Set-Cookie"}

// frozenDate replaces Date headers so the recording timestamp doesn't
// identify when a cassette was captured. A constant valid RFC1123 value
// keeps parsers happy without leaking anything.
const frozenDate = "Mon, 01 Jan 2001 00:00:00 GMT"

// Accepted residuals (not scrubbed by this package):
//
//   - Opaque Proton-internal IDs (User.ID, Keys[*].ID, Address.ID, DomainID).
//     They round-trip between request URL paths and response bodies, so
//     REDACTED_* would break URL-based replay; deterministic fixture
//     substitution would require URL-aware rewriting and is deferred.
//     The IDs are account-correlatable but cannot be used as credentials.
//   - Response headers other than those in redactedHeaders + Date. Most
//     are Proton infrastructure metadata (CSP, HSTS, cache hints) that
//     leak no user-specific data.

// Rescrub loads an on-disk cassette, runs saveHook against every interaction,
// and writes the result back. Useful when scrub rules tighten and existing
// cassettes need a re-pass without re-recording from the live API. Env vars
// (RECORD_EMAIL etc.) drive identifier rewrites the same way as a live record.
func Rescrub(path string) error {
	// cassette.Load appends ".yaml"; accept either form by stripping it.
	name := strings.TrimSuffix(path, ".yaml")
	c, err := cassette.Load(name)
	if err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	// Load() doesn't set MarshalFunc — Save() panics without one. Match the
	// recorder default (gopkg.in/yaml.v3 Marshal).
	c.MarshalFunc = yaml.Marshal
	for _, i := range c.Interactions {
		if err := saveHook(i); err != nil {
			return fmt.Errorf("scrub interaction %d: %w", i.ID, err)
		}
	}
	if err := c.Save(); err != nil {
		return fmt.Errorf("save %s: %w", path, err)
	}
	return nil
}

func saveHook(i *cassette.Interaction) error {
	for _, h := range redactedHeaders {
		if i.Request.Headers.Get(h) != "" {
			i.Request.Headers.Set(h, "REDACTED")
		}
		if i.Response.Headers.Get(h) != "" {
			i.Response.Headers.Set(h, "REDACTED")
		}
	}
	if i.Request.Headers.Get("Date") != "" {
		i.Request.Headers.Set("Date", frozenDate)
	}
	if i.Response.Headers.Get("Date") != "" {
		i.Response.Headers.Set("Date", frozenDate)
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
			if str, ok := vv.(string); ok {
				if replaced, did := s.replacePGPArmor(k, str); did {
					t[k] = replaced
					continue
				}
				if s.matchesLocalPart(str) {
					t[k] = "user"
					continue
				}
			}
			if isSensitiveJSONKey(k) {
				if str, ok := vv.(string); ok {
					// Empty strings carry no secret and stay as-is — replacing
					// them with a REDACTED_* placeholder would diverge from
					// the consumer's request body (which sends "") and break
					// cassette matching.
					if str == "" {
						continue
					}
					// Counter tracks per-key occurrences (canonical upper-case
					// so "UID" and "Uid" share one REDACTED_UID_<N> sequence).
					canonical := strings.ToUpper(k)
					s.counters[canonical]++
					placeholder := fmt.Sprintf("REDACTED_%s_%d", canonical, s.counters[canonical])
					if base64ValuedJSONKey(k) {
						// The consumer code base64-decodes these values before
						// using them (e.g. proton.Manager decodes ServerProof
						// during refresh-on-401). A raw "REDACTED_*" string
						// fails base64 validation; encode the placeholder so
						// the decoded bytes still serve as a stable identifier
						// without leaking the original.
						placeholder = base64.StdEncoding.EncodeToString([]byte(placeholder))
					}
					t[k] = placeholder
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

// replacePGPArmor returns (substitute, true) when v is a PGP-armored block.
// PRIVATE/PUBLIC KEY BLOCKs swap to the in-repo fixture pair so
// proton.Key.UnmarshalJSON can still parse them on cassette load; MESSAGE
// and SIGNATURE blocks (e.g. RecoverySecretSignature) get REDACTED_PGP_*
// since they don't round-trip through a crypto parser.
//
// The "Modulus" key is exempt because Proton's SRP modulus is a global
// prime — the same signed value for every account, embedded as a public
// fixture in go-srp itself — and replacing it with a placeholder would
// break go-srp's signature verification at cassette replay.
func (s *bodyScrubber) replacePGPArmor(k, v string) (string, bool) {
	if !strings.HasPrefix(v, "-----BEGIN PGP ") {
		return "", false
	}
	switch {
	case strings.HasPrefix(v, "-----BEGIN PGP PRIVATE KEY BLOCK"):
		return fixturePrivateKey, true
	case strings.HasPrefix(v, "-----BEGIN PGP PUBLIC KEY BLOCK"):
		return fixturePublicKey, true
	case k == "Modulus":
		return v, true
	default:
		canonical := strings.ToUpper(k)
		s.counters["PGP_"+canonical]++
		return fmt.Sprintf("REDACTED_PGP_%s_%d", canonical, s.counters["PGP_"+canonical]), true
	}
}

// matchesLocalPart reports whether v exactly equals the local part of
// RECORD_EMAIL. Proton's Name/DisplayName fields hold the email local part
// standalone, so the full-string ReplaceAll in rewriteIdentifiers misses
// them. Exact-match (not substring) avoids collateral on short local parts.
func (s *bodyScrubber) matchesLocalPart(v string) bool {
	if s.email == "" {
		return false
	}
	at := strings.IndexByte(s.email, '@')
	if at <= 0 || at >= len(s.email)-1 {
		return false
	}
	return v == s.email[:at]
}

// protonAddressTLDs covers the Proton-issued address suffixes a single
// account can hold simultaneously. proton_list_addresses returns every alias
// across all four, so scrubbing only the configured RECORD_EMAIL TLD leaves
// the sibling addresses intact.
var protonAddressTLDs = []string{"@protonmail.com", "@protonmail.ch", "@proton.me", "@pm.me"}

func (s *bodyScrubber) rewriteIdentifiers(in string) string {
	out := in
	if s.email != "" {
		out = strings.ReplaceAll(out, s.email, "user@example.test")
		if at := strings.IndexByte(s.email, '@'); at > 0 && at < len(s.email)-1 {
			local := s.email[:at]
			for _, tld := range protonAddressTLDs {
				out = strings.ReplaceAll(out, local+tld, "user@example.test")
			}
		}
	}
	if s.throwawayDomain != "" {
		out = strings.ReplaceAll(out, s.throwawayDomain, "throwaway.example.test")
	}
	if s.domain != "" {
		out = strings.ReplaceAll(out, s.domain, "example.test")
	}
	return out
}

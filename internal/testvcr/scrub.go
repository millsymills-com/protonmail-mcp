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

// sensitiveKey describes how a matched JSON key's value is redacted. base64
// marks keys whose values consumer code base64-decodes at use (e.g.
// proton.Manager decodes ServerProof during refresh-on-401), so their REDACTED
// placeholder must itself round-trip through base64 cleanly — otherwise the
// consumer errors before any guard like WithSkipVerifyProofs runs.
type sensitiveKey struct{ base64 bool }

// sensitiveJSONKeys are compared against incoming JSON keys via
// strings.EqualFold so casing variations like "UID" vs "Uid" and
// "accessToken" vs "AccessToken" are all caught. Proton returns both
// "UID" and "Uid" in some payloads (e.g. /auth/v4/refresh) and a
// case-sensitive lookup missed the lowercase form.
var sensitiveJSONKeys = map[string]sensitiveKey{
	"AccessToken":     {},
	"RefreshToken":    {},
	"UID":             {},
	"KeySalt":         {},
	"PrivateKey":      {},
	"Signature":       {base64: true},
	"Token":           {},
	"SrpSession":      {},
	"ServerProof":     {base64: true},
	"ClientProof":     {base64: true},
	"ClientEphemeral": {base64: true},
	"TwoFactorCode":   {},
	"RecoverySecret":  {},
	"Fingerprint":     {},
}

// lookupSensitiveJSONKey case-folds k against sensitiveJSONKeys, returning the
// entry and whether it matched. A separate ok return is required because a
// present non-base64 key and an absent key share the zero sensitiveKey value.
func lookupSensitiveJSONKey(k string) (sensitiveKey, bool) {
	for sk, meta := range sensitiveJSONKeys {
		if strings.EqualFold(sk, k) {
			return meta, true
		}
	}
	return sensitiveKey{}, false
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
	localParts      []string
}

// newBodyScrubber reads the RECORD_* env the same way a live recording does.
// localParts collects every address local part the account holds: the local
// part of RECORD_EMAIL plus any comma-separated entries in RECORD_LOCAL_PARTS.
// Proton accounts commonly own sibling aliases on a different local part (not
// just a different TLD), and a Name/DisplayName or address carrying one of
// those local parts would otherwise survive scrubbing.
func newBodyScrubber() *bodyScrubber {
	email := strings.TrimSpace(os.Getenv("RECORD_EMAIL"))
	var localParts []string
	if at := strings.IndexByte(email, '@'); at > 0 {
		localParts = append(localParts, email[:at])
	}
	for _, lp := range strings.Split(os.Getenv("RECORD_LOCAL_PARTS"), ",") {
		if lp = strings.TrimSpace(lp); lp != "" {
			localParts = append(localParts, lp)
		}
	}
	return &bodyScrubber{
		counters:        map[string]int{},
		email:           email,
		domain:          strings.TrimSpace(os.Getenv("RECORD_DOMAIN")),
		throwawayDomain: strings.TrimSpace(os.Getenv("RECORD_THROWAWAY_DOMAIN")),
		localParts:      localParts,
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
			if s.scrubHeaderField(t, k, vv) {
				continue
			}
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
			if meta, ok := lookupSensitiveJSONKey(k); ok {
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
					if meta.base64 {
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
	case strings.EqualFold(k, "Modulus") && strings.HasPrefix(v, "-----BEGIN PGP SIGNED MESSAGE"):
		return v, true
	default:
		canonical := strings.ToUpper(k)
		s.counters["PGP_"+canonical]++
		return fmt.Sprintf("REDACTED_PGP_%s_%d", canonical, s.counters["PGP_"+canonical]), true
	}
}

// matchesLocalPart reports whether v exactly equals one of the account's
// address local parts. Proton's Name/DisplayName fields hold a local part
// standalone, so the full-string ReplaceAll in rewriteIdentifiers misses
// them. Exact-match (not substring) avoids collateral on short local parts.
func (s *bodyScrubber) matchesLocalPart(v string) bool {
	for _, lp := range s.localParts {
		if v == lp {
			return true
		}
	}
	return false
}

// rfc2822SafeHeaders lists RFC2822 header names (lower-cased) whose values are
// structural MIME/format metadata carrying no per-message or per-account PII.
// The scrubber keeps these and REDACTS every other header value. A denylist
// would silently leak any sender-controlled header it failed to enumerate —
// routing trail, List-Unsubscribe tracking tokens, X-Originating-IP, abuse
// contacts, third-party addresses — so the safe default is redact-unless-listed.
var rfc2822SafeHeaders = map[string]bool{
	"mime-version":              true,
	"content-type":              true,
	"content-transfer-encoding": true,
	"content-disposition":       true,
	"content-id":                true,
	"content-description":       true,
	"content-language":          true,
}

// scrubHeaderField redacts the three shapes Proton uses to surface a message's
// header-derived content: a "Header" string holding a full RFC2822 block, a
// "ParsedHeaders" object keyed by header name, and a bare "Subject" string on
// the message body object (a sibling of Header/ParsedHeaders, not nested in
// them). The Subject case is scoped to message objects so it leaves same-named
// fields on unrelated payloads (e.g. MailSettings.AutoResponder.Subject) alone.
// Returns true when it handled (and thus consumed) the entry, so walk skips its
// generic processing.
func (s *bodyScrubber) scrubHeaderField(t map[string]any, k string, vv any) bool {
	switch {
	case strings.EqualFold(k, "Header"):
		if str, ok := vv.(string); ok {
			t[k] = scrubRFC2822Headers(str)
			return true
		}
	case strings.EqualFold(k, "ParsedHeaders"):
		if m, ok := vv.(map[string]any); ok {
			for hk := range m {
				if !rfc2822SafeHeaders[strings.ToLower(hk)] {
					m[hk] = "REDACTED"
				}
			}
			return true
		}
	case strings.EqualFold(k, "Subject"):
		if _, ok := vv.(string); ok && isMessageObject(t) {
			t[k] = "REDACTED"
			return true
		}
	}
	return false
}

// isMessageObject reports whether t is a Proton message or message-list entry.
// Every Message and MessageMetadata carries a ConversationID; objects that
// merely happen to have a Subject (MailSettings.AutoResponder) do not, so this
// keeps the Subject redaction from clobbering unrelated settings values.
func isMessageObject(t map[string]any) bool {
	for k := range t {
		if strings.EqualFold(k, "ConversationID") {
			return true
		}
	}
	return false
}

// scrubRFC2822Headers redacts the value of every header whose name is not in
// rfc2822SafeHeaders, preserving the header name and the per-line terminator
// (so a CRLF block stays CRLF) to keep the block syntactically intact. Folded
// continuation lines (leading whitespace, e.g. a multi-line DKIM-Signature) of
// a redacted header are dropped.
func scrubRFC2822Headers(raw string) string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	redacting := false
	for _, line := range lines {
		cr := ""
		if strings.HasSuffix(line, "\r") {
			cr = "\r"
			line = line[:len(line)-1]
		}
		if line != "" && (line[0] == ' ' || line[0] == '\t') {
			if !redacting {
				out = append(out, line+cr)
			}
			continue
		}
		redacting = false
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			name := strings.ToLower(strings.TrimSpace(line[:idx]))
			if !rfc2822SafeHeaders[name] {
				out = append(out, line[:idx]+": REDACTED"+cr)
				redacting = true
				continue
			}
		}
		out = append(out, line+cr)
	}
	return strings.Join(out, "\n")
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
	}
	for _, lp := range s.localParts {
		for _, tld := range protonAddressTLDs {
			out = strings.ReplaceAll(out, lp+tld, "user@example.test")
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

package testvcr

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
)

func TestScrubHeaderRedaction(t *testing.T) {
	i := &cassette.Interaction{
		Request: cassette.Request{
			Headers: http.Header{
				"Authorization": []string{"Bearer secret"},
				"X-Pm-Uid":      []string{"abc123"},
				"Cookie":        []string{"sess=xyz"},
				"User-Agent":    []string{"protonmail-mcp/test"},
			},
		},
		Response: cassette.Response{Headers: http.Header{"Set-Cookie": []string{"sess=zzz"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	want := http.Header{
		"Authorization": []string{"REDACTED"},
		"X-Pm-Uid":      []string{"REDACTED"},
		"Cookie":        []string{"REDACTED"},
		"User-Agent":    []string{"protonmail-mcp/test"},
	}
	if !reflect.DeepEqual(i.Request.Headers, want) {
		t.Fatalf("request headers = %#v, want %#v", i.Request.Headers, want)
	}
	if got := i.Response.Headers.Get("Set-Cookie"); got != "REDACTED" {
		t.Fatalf("Set-Cookie = %q, want REDACTED", got)
	}
}

func TestScrubJSONBodyReplacesSensitiveKeys(t *testing.T) {
	body := `{"AccessToken":"eyJraWQi","RefreshToken":"rt-1","User":{"Email":"me@protonmail.com"}}`
	i := &cassette.Interaction{
		Request:  cassette.Request{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	t.Setenv("RECORD_EMAIL", "me@protonmail.com")
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(i.Response.Body), &got); err != nil {
		t.Fatal(err)
	}
	if got["AccessToken"] != "REDACTED_ACCESSTOKEN_1" {
		t.Fatalf("AccessToken not scrubbed: %v", got["AccessToken"])
	}
	if got["RefreshToken"] != "REDACTED_REFRESHTOKEN_1" {
		t.Fatalf("RefreshToken not scrubbed: %v", got["RefreshToken"])
	}
	user := got["User"].(map[string]any)
	if user["Email"] != "user@example.test" {
		t.Fatalf("email not rewritten: %v", user["Email"])
	}
}

// TestScrubReplacesPGPKeysWithFixture pins the scrubber's PGP substitution
// behaviour. Real Proton key payloads embed the account UID inside the
// armored packet (so the raw email leaks through) and the matching
// PrivateKey is encrypted-but-still-sensitive material; replacing the pair
// with a generated fixture both removes the leak and gives
// proton.Key.UnmarshalJSON valid armored data to parse on cassette load.
func TestScrubReplacesPGPKeysWithFixture(t *testing.T) {
	body := `{"PrivateKey":"-----BEGIN PGP PRIVATE KEY BLOCK-----\nAAAA\n-----END PGP PRIVATE KEY BLOCK-----","PublicKey":"-----BEGIN PGP PUBLIC KEY BLOCK-----\nBBBB\n-----END PGP PUBLIC KEY BLOCK-----"}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	got := i.Response.Body
	if !strings.Contains(got, "-----BEGIN PGP PRIVATE KEY BLOCK-----") {
		t.Fatalf("private key armor missing after scrub: %s", got)
	}
	if !strings.Contains(got, "-----BEGIN PGP PUBLIC KEY BLOCK-----") {
		t.Fatalf("public key armor missing after scrub: %s", got)
	}
	if !strings.Contains(got, "Comment: https://gopenpgp.org") {
		t.Fatalf("expected gopenpgp fixture marker, got: %s", got)
	}
}

func TestScrubRewritesDomain(t *testing.T) {
	t.Setenv("RECORD_DOMAIN", "myalias.dev")
	body := `{"Domain":"myalias.dev","Subdomain":"mail.myalias.dev"}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	if got := i.Response.Body; got != `{"Domain":"example.test","Subdomain":"mail.example.test"}` {
		t.Fatalf("domain not rewritten: %s", got)
	}
}

func TestScrubRewritesThrowawayDomain(t *testing.T) {
	t.Setenv("RECORD_THROWAWAY_DOMAIN", "throwaway.dev")
	body := `{"DomainName":"throwaway.dev","Status":"active"}`
	ct := http.Header{"Content-Type": []string{"application/json"}}
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: ct},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	want := `{"DomainName":"throwaway.example.test","Status":"active"}`
	if got := i.Response.Body; got != want {
		t.Fatalf("throwaway domain not rewritten: got %s, want %s", got, want)
	}
}

// TestScrubFreezesDateHeader pins the Date-header freeze: a real recording
// timestamp would otherwise mark exactly when the cassette was captured.
// The replacement must stay RFC1123-parseable so downstream code that calls
// time.Parse on it doesn't break at replay time.
func TestScrubFreezesDateHeader(t *testing.T) {
	i := &cassette.Interaction{
		Request:  cassette.Request{Headers: http.Header{"Date": []string{"Mon, 18 May 2026 17:51:09 GMT"}}},
		Response: cassette.Response{Headers: http.Header{"Date": []string{"Mon, 18 May 2026 17:51:10 GMT"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	want := "Mon, 01 Jan 2001 00:00:00 GMT"
	if got := i.Request.Headers.Get("Date"); got != want {
		t.Fatalf("request Date = %q, want %q", got, want)
	}
	if got := i.Response.Headers.Get("Date"); got != want {
		t.Fatalf("response Date = %q, want %q", got, want)
	}
	if _, err := time.Parse(time.RFC1123, want); err != nil {
		t.Fatalf("frozen Date isn't RFC1123-parseable: %v", err)
	}
}

// TestScrubRedactsRecoverySecretAndFingerprint pins the post-2026-05-18
// additions to sensitiveJSONKeys. Real Proton /core/v4/users responses leak
// a base64 RecoverySecret and the account's PGP key Fingerprint inline —
// both need REDACTED_*_N placeholders before commit.
func TestScrubRedactsRecoverySecretAndFingerprint(t *testing.T) {
	body := `{"RecoverySecret":"g8RbbkMK/qLTE0R7vh1Q0mh5lrUL51dWcbIluRbJg0w=","Fingerprint":"e1a4a657f6898d2204d606e235fac8ab7e0011e6"}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(i.Response.Body), &got); err != nil {
		t.Fatal(err)
	}
	if got["RecoverySecret"] != "REDACTED_RECOVERYSECRET_1" {
		t.Fatalf("RecoverySecret = %v, want REDACTED_RECOVERYSECRET_1", got["RecoverySecret"])
	}
	if got["Fingerprint"] != "REDACTED_FINGERPRINT_1" {
		t.Fatalf("Fingerprint = %v, want REDACTED_FINGERPRINT_1", got["Fingerprint"])
	}
}

// TestScrubReplacesPGPSignatureAndMessage covers armor variants that aren't
// keys — RecoverySecretSignature (PGP SIGNATURE) and any PGP MESSAGE — by
// asserting they collapse to a REDACTED_PGP_* placeholder rather than
// shipping the real Proton-tagged armor.
func TestScrubReplacesPGPSignatureAndMessage(t *testing.T) {
	body := `{"RecoverySecretSignature":"-----BEGIN PGP SIGNATURE-----\nVersion: ProtonMail\n\nwsB...\n-----END PGP SIGNATURE-----","Token":"-----BEGIN PGP MESSAGE-----\nVersion: ProtonMail\n\nwcB...\n-----END PGP MESSAGE-----"}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(i.Response.Body), &got); err != nil {
		t.Fatal(err)
	}
	sig, _ := got["RecoverySecretSignature"].(string)
	if !strings.HasPrefix(sig, "REDACTED_PGP_") {
		t.Fatalf("RecoverySecretSignature not collapsed: %q", sig)
	}
	tok, _ := got["Token"].(string)
	if !strings.HasPrefix(tok, "REDACTED_PGP_") {
		t.Fatalf("Token not collapsed: %q", tok)
	}
	if strings.Contains(i.Response.Body, "Version: ProtonMail") {
		t.Fatalf("real Proton armor survived: %s", i.Response.Body)
	}
}

// TestScrubRewritesSiblingProtonAddresses covers Proton accounts that hold
// addresses on multiple TLDs (e.g. RECORD_EMAIL=foo@pm.me but the account
// also has foo@protonmail.com). proton_list_addresses returns every alias,
// and a substring rewrite of just RECORD_EMAIL misses the siblings.
func TestScrubRewritesSiblingProtonAddresses(t *testing.T) {
	t.Setenv("RECORD_EMAIL", "overm1nd@pm.me")
	body := `{"Addresses":[{"Email":"overm1nd@pm.me"},{"Email":"overm1nd@protonmail.com"},{"Email":"overm1nd@proton.me"},{"Email":"overm1nd@protonmail.ch"}]}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	for _, tld := range []string{"@pm.me", "@protonmail.com", "@proton.me", "@protonmail.ch"} {
		if strings.Contains(i.Response.Body, "overm1nd"+tld) {
			t.Fatalf("sibling address overm1nd%s survived: %s", tld, i.Response.Body)
		}
	}
}

// TestScrubRewritesEmailLocalPart covers Name/DisplayName which hold the
// email local part standalone (not the full address). rewriteIdentifiers'
// substring replace on the full email misses these — the walk-time
// exact-match check is what catches them.
func TestScrubRewritesEmailLocalPart(t *testing.T) {
	t.Setenv("RECORD_EMAIL", "overm1nd@pm.me")
	body := `{"Name":"overm1nd","DisplayName":"overm1nd","Unrelated":"overm1nd-prime"}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(i.Response.Body), &got); err != nil {
		t.Fatal(err)
	}
	if got["Name"] != "user" {
		t.Fatalf("Name not rewritten: %v", got["Name"])
	}
	if got["DisplayName"] != "user" {
		t.Fatalf("DisplayName not rewritten: %v", got["DisplayName"])
	}
	if got["Unrelated"] != "overm1nd-prime" {
		t.Fatalf("substring false-positive: %v", got["Unrelated"])
	}
}

// TestScrubCaseFoldsSensitiveKeys guards against the leak found 2026-05-17
// where Proton's /auth/v4/refresh response returned both "UID" and "Uid" but
// only the upper-case form was redacted. Case-insensitive lookup is the fix.
func TestScrubCaseFoldsSensitiveKeys(t *testing.T) {
	body := `{"UID":"u1","Uid":"u2","accessToken":"at1","refreshtoken":"rt1"}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(i.Response.Body), &got); err != nil {
		t.Fatal(err)
	}
	for key, val := range got {
		if s, ok := val.(string); ok && !strings.HasPrefix(s, "REDACTED_") {
			t.Fatalf("key %q value %q was not scrubbed", key, s)
		}
	}
}

// TestScrubBase64EncodesProofPlaceholders pins the base64-valued branch of
// sensitiveJSONKeys. ServerProof/ClientProof/ClientEphemeral/Signature hold
// values consumer code base64-decodes at use (e.g. proton.Manager decodes
// ServerProof during refresh-on-401), so a raw "REDACTED_*" placeholder would
// fail that decode before any WithSkipVerifyProofs guard runs. Each placeholder
// must therefore round-trip through base64. Without this test a dropped base64
// flag would pass silently.
func TestScrubBase64EncodesProofPlaceholders(t *testing.T) {
	body := `{"ServerProof":"c3A=","ClientProof":"Y3A=","ClientEphemeral":"Y2U=","Signature":"c2c="}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(i.Response.Body), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"ServerProof":     "REDACTED_SERVERPROOF_1",
		"ClientProof":     "REDACTED_CLIENTPROOF_1",
		"ClientEphemeral": "REDACTED_CLIENTEPHEMERAL_1",
		"Signature":       "REDACTED_SIGNATURE_1",
	}
	for k, decoded := range want {
		s, ok := got[k].(string)
		if !ok {
			t.Fatalf("%s missing or non-string: %v", k, got[k])
		}
		dec, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			t.Fatalf("%s placeholder %q is not valid base64: %v", k, s, err)
		}
		if string(dec) != decoded {
			t.Fatalf("%s decoded = %q, want %q", k, dec, decoded)
		}
	}
}

// TestScrubRewritesSiblingLocalParts covers RECORD_LOCAL_PARTS: an account
// holds aliases on a different local part (not just a different TLD), e.g. the
// "mills@" sibling that leaked in the get_message_happy cassette. Both the
// standalone local part (Name field) and the full sibling address must scrub.
func TestScrubRewritesSiblingLocalParts(t *testing.T) {
	t.Setenv("RECORD_EMAIL", "overm1nd@pm.me")
	t.Setenv("RECORD_LOCAL_PARTS", "mills, overm1nd")
	body := `{"Name":"mills","Addresses":[{"Email":"mills@proton.me"},{"Email":"mills@pm.me"}]}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(i.Response.Body, "mills") {
		t.Fatalf("sibling local part survived: %s", i.Response.Body)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(i.Response.Body), &got); err != nil {
		t.Fatal(err)
	}
	if got["Name"] != "user" {
		t.Fatalf("Name not rewritten: %v", got["Name"])
	}
}

// TestScrubRedactsRawRFC2822Header covers the "Header" string field that
// proton_get_message returns: a raw RFC2822 block whose From/To/Subject/
// Message-Id/DKIM-Signature carry per-message PII. Header names are preserved;
// every value whose name is not on the structural allowlist (Content-Type
// here) is redacted, including a folded multi-line DKIM-Signature.
func TestScrubRedactsRawRFC2822Header(t *testing.T) {
	raw := "From: \"Andrew Mills\" <notifications@github.com>\n" +
		"To: protonmail-mcp@noreply.github.com\n" +
		"Subject: [millsymills-com/protonmail-mcp] PR run failed (69de588)\n" +
		"Message-Id: <abc.123@github.com>\n" +
		"DKIM-Signature: v=1; a=rsa-sha256;\n\tb=longsig==\n" +
		"Content-Type: text/plain\n"
	jsonBody, err := json.Marshal(map[string]any{"Header": raw})
	if err != nil {
		t.Fatal(err)
	}
	i := &cassette.Interaction{
		Response: cassette.Response{Body: string(jsonBody), Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"notifications@github.com", "69de588", "abc.123", "longsig"} {
		if strings.Contains(i.Response.Body, leak) {
			t.Fatalf("leak %q survived header scrub: %s", leak, i.Response.Body)
		}
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(i.Response.Body), &got); err != nil {
		t.Fatal(err)
	}
	header, _ := got["Header"].(string)
	if !strings.Contains(header, "Content-Type: text/plain") {
		t.Fatalf("non-sensitive header dropped: %q", header)
	}
	if !strings.Contains(header, "Subject: REDACTED") {
		t.Fatalf("Subject not redacted: %q", header)
	}
}

// TestScrubRedactsParsedHeaders covers the "ParsedHeaders" object shape: every
// key not on the structural allowlist is redacted (e.g. X-Pm-Spam's encrypted
// blob, Message-Id), while an allowlisted key (Content-Type) stays.
func TestScrubRedactsParsedHeaders(t *testing.T) {
	body := `{"ParsedHeaders":{"X-Pm-Spam":"encrypted-blob","Message-Id":"<x@y>","Content-Type":"text/plain"}}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(i.Response.Body), &got); err != nil {
		t.Fatal(err)
	}
	ph := got["ParsedHeaders"].(map[string]any)
	if ph["X-Pm-Spam"] != "REDACTED" {
		t.Fatalf("X-Pm-Spam not redacted: %v", ph["X-Pm-Spam"])
	}
	if ph["Message-Id"] != "REDACTED" {
		t.Fatalf("Message-Id not redacted: %v", ph["Message-Id"])
	}
	if ph["Content-Type"] != "text/plain" {
		t.Fatalf("harmless header altered: %v", ph["Content-Type"])
	}
}

// TestScrubRawHeaderRedactsSenderControlled guards the allowlist against the
// denylist gap it replaced: sender-controlled headers (List-Unsubscribe
// tracking token, third-party Sender, X-Originating-IP) that no denylist
// enumerated must still be redacted because they are not on the structural
// allowlist. Uses CRLF line endings like real Proton cassettes, and asserts
// the terminator is preserved on kept lines and a folded continuation of a
// redacted header is dropped.
func TestScrubRawHeaderRedactsSenderControlled(t *testing.T) {
	raw := "List-Unsubscribe: <https://track.example.com/u?token=SECRET123>\r\n" +
		"Sender: \"Ernie Ball\" <newsletter@ernieball.com>\r\n" +
		"X-Originating-IP: 199.167.224.156\r\n" +
		"DKIM-Signature: v=1; a=rsa-sha256;\r\n\tb=foldedsig==\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html\r\n"
	jsonBody, err := json.Marshal(map[string]any{"Header": raw})
	if err != nil {
		t.Fatal(err)
	}
	i := &cassette.Interaction{
		Response: cassette.Response{Body: string(jsonBody), Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"SECRET123", "newsletter@ernieball.com", "199.167.224.156", "foldedsig"} {
		if strings.Contains(i.Response.Body, leak) {
			t.Fatalf("leak %q survived allowlist scrub: %s", leak, i.Response.Body)
		}
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(i.Response.Body), &got); err != nil {
		t.Fatal(err)
	}
	header, _ := got["Header"].(string)
	for _, kept := range []string{"MIME-Version: 1.0\r\n", "Content-Type: text/html\r\n"} {
		if !strings.Contains(header, kept) {
			t.Fatalf("structural header dropped or CRLF lost; want %q in %q", kept, header)
		}
	}
	if !strings.Contains(header, "List-Unsubscribe: REDACTED\r\n") {
		t.Fatalf("List-Unsubscribe not redacted with CRLF preserved: %q", header)
	}
}

// TestScrubParsedHeadersRedactsSenderControlled covers the same allowlist gap
// in the "ParsedHeaders" object shape, including an array-valued header
// (Authentication-Results) as Proton emits — a non-allowlisted key is replaced
// regardless of whether its value is a string or array.
func TestScrubParsedHeadersRedactsSenderControlled(t *testing.T) {
	body := `{"ParsedHeaders":{"List-Unsubscribe":"<https://track.example.com/u?token=SECRET456>",` +
		`"Sender":"newsletter@ernieball.com","Authentication-Results":["dkim=pass d=ernieball.com","spf=pass"],` +
		`"X-Originating-IP":"199.167.224.156","Content-Type":"text/html","MIME-Version":"1.0"}}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"SECRET456", "newsletter@ernieball.com", "199.167.224.156", "dkim=pass"} {
		if strings.Contains(i.Response.Body, leak) {
			t.Fatalf("leak %q survived parsed-header scrub: %s", leak, i.Response.Body)
		}
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(i.Response.Body), &got); err != nil {
		t.Fatal(err)
	}
	ph := got["ParsedHeaders"].(map[string]any)
	for _, redacted := range []string{"List-Unsubscribe", "Sender", "Authentication-Results", "X-Originating-IP"} {
		if ph[redacted] != "REDACTED" {
			t.Fatalf("%s not redacted: %v", redacted, ph[redacted])
		}
	}
	if ph["Content-Type"] != "text/html" || ph["MIME-Version"] != "1.0" {
		t.Fatalf("structural header altered: Content-Type=%v MIME-Version=%v", ph["Content-Type"], ph["MIME-Version"])
	}
}

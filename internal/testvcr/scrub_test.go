package testvcr

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

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

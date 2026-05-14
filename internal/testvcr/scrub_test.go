package testvcr

import (
	"encoding/json"
	"net/http"
	"reflect"
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

func TestScrubLeavesPublicPGPKeyAlone(t *testing.T) {
	body := `{"PublicKey":"-----BEGIN PGP PUBLIC KEY BLOCK-----\nAAAA\n-----END PGP PUBLIC KEY BLOCK-----"}`
	i := &cassette.Interaction{
		Response: cassette.Response{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}},
	}
	if err := saveHook(i); err != nil {
		t.Fatal(err)
	}
	if i.Response.Body != body {
		t.Fatalf("public key block was modified: %s", i.Response.Body)
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

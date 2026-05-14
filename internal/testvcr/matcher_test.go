package testvcr

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
)

func req(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	r, err := http.NewRequest(method, url, io.NopCloser(bytes.NewBufferString(body)))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestMatcherMethodAndPath(t *testing.T) {
	r := req(t, "GET", "https://mail.proton.me/api/core/v4/users", "")
	c := cassette.Request{Method: "GET", URL: "https://mail.proton.me/api/core/v4/users"}
	if !BodyAwareMatcher(r, c) {
		t.Fatal("expected match")
	}
	c.Method = "POST"
	if BodyAwareMatcher(r, c) {
		t.Fatal("expected mismatch on method")
	}
}

func TestMatcherCanonicalisesJSONBody(t *testing.T) {
	a := `{"Username":"alice","ClientProof":"random123"}`
	b := `{"ClientProof":"random123","Username":"alice"}`
	r := req(t, "POST", "https://example.test/api/auth", a)
	c := cassette.Request{Method: "POST", URL: "https://example.test/api/auth", Body: b}
	if !BodyAwareMatcher(r, c) {
		t.Fatal("expected match after key reorder")
	}
}

func TestMatcherSRPIgnoresClientProofValue(t *testing.T) {
	a := `{"Username":"alice","ClientProof":"differentvalue","ClientEphemeral":"e1"}`
	b := `{"Username":"alice","ClientProof":"REDACTED_CLIENTPROOF_1",` +
		`"ClientEphemeral":"REDACTED_CLIENTEPHEMERAL_1"}`
	r := req(t, "POST", "https://example.test/api/auth", a)
	c := cassette.Request{Method: "POST", URL: "https://example.test/api/auth", Body: b}
	if !BodyAwareMatcher(r, c) {
		t.Fatal("SRP matcher should ignore proof value, match on presence + Username")
	}
}

func TestMatcherPathTolerantToOpaqueIDs(t *testing.T) {
	r := req(t, "GET", "https://example.test/api/core/v4/addresses/abcdef1234/keys", "")
	c := cassette.Request{
		Method: "GET",
		URL:    "https://example.test/api/core/v4/addresses/zyxw98765/keys",
	}
	if !BodyAwareMatcher(r, c) {
		t.Fatal("expected ID-tolerant path match")
	}
}

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

func TestBodyAwareMatcher(t *testing.T) {
	tests := []struct {
		name                       string
		reqMethod, reqURL, reqBody string
		cMethod, cURL, cBody       string
		want                       bool
	}{
		{
			name:      "method+path-match",
			reqMethod: "GET", reqURL: "https://mail.proton.me/api/core/v4/users",
			cMethod: "GET", cURL: "https://mail.proton.me/api/core/v4/users",
			want: true,
		},
		{
			name:      "method-mismatch",
			reqMethod: "GET", reqURL: "https://mail.proton.me/api/core/v4/users",
			cMethod: "POST", cURL: "https://mail.proton.me/api/core/v4/users",
			want: false,
		},
		{
			name:      "json-key-reorder-canonicalised",
			reqMethod: "POST", reqURL: "https://example.test/api/auth",
			reqBody: `{"Username":"alice","ClientProof":"random123"}`,
			cMethod: "POST", cURL: "https://example.test/api/auth",
			cBody: `{"ClientProof":"random123","Username":"alice"}`,
			want:  true,
		},
		{
			name:      "srp-ignores-clientproof-value",
			reqMethod: "POST", reqURL: "https://example.test/api/auth",
			reqBody: `{"Username":"alice","ClientProof":"differentvalue","ClientEphemeral":"e1"}`,
			cMethod: "POST", cURL: "https://example.test/api/auth",
			cBody: `{"Username":"alice","ClientProof":"REDACTED_CLIENTPROOF_1",` +
				`"ClientEphemeral":"REDACTED_CLIENTEPHEMERAL_1"}`,
			want: true,
		},
		{
			name:      "path-tolerant-to-opaque-ids",
			reqMethod: "GET", reqURL: "https://example.test/api/core/v4/addresses/abcdef1234/keys",
			cMethod: "GET", cURL: "https://example.test/api/core/v4/addresses/zyxw98765/keys",
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := req(t, tc.reqMethod, tc.reqURL, tc.reqBody)
			c := cassette.Request{Method: tc.cMethod, URL: tc.cURL, Body: tc.cBody}
			if got := BodyAwareMatcher(r, c); got != tc.want {
				t.Fatalf("BodyAwareMatcher = %v, want %v", got, tc.want)
			}
		})
	}
}

package session

import (
	"net/http"
	"net/http/httptest"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/go-resty/resty/v2"
)

// TestAuth2FACaptureAdoptsPostTokens pins the fix for #86: when Proton's
// /auth/v4/2fa response carries rotated tokens, merge() returns an Auth
// that overlays them on the pre-2FA values.
func TestAuth2FACaptureAdoptsPostTokens(t *testing.T) {
	body := []byte(`{"Code":1000,"UID":"uid-post","AccessToken":"acc-post","RefreshToken":"ref-post","Scope":"full"}`)
	resp := newRestyResponse(t, http.MethodPost, "https://api.example/auth/v4/2fa", http.StatusOK, body)

	cap := newAuth2FACapture()
	if err := cap.hook(nil, resp); err != nil {
		t.Fatalf("hook: %v", err)
	}

	base := proton.Auth{UID: "uid-pre", AccessToken: "acc-pre", RefreshToken: "ref-pre", Scope: "twofactor"}
	got := cap.merge(base)
	if got == nil {
		t.Fatal("merge returned nil; want post-2FA Auth")
	}
	if got.UID != "uid-post" || got.AccessToken != "acc-post" || got.RefreshToken != "ref-post" || got.Scope != "full" {
		t.Fatalf("merge = %+v", *got)
	}
}

// Proton may decline to rotate one or more fields. merge must fall back to
// the pre-2FA value for any field the response omitted, not blow it out.
func TestAuth2FACapturePreservesUnsetFields(t *testing.T) {
	body := []byte(`{"Code":1000,"RefreshToken":"ref-post"}`)
	resp := newRestyResponse(t, http.MethodPost, "https://api.example/auth/v4/2fa", http.StatusOK, body)

	cap := newAuth2FACapture()
	_ = cap.hook(nil, resp)

	base := proton.Auth{UID: "uid-pre", AccessToken: "acc-pre", RefreshToken: "ref-pre"}
	got := cap.merge(base)
	if got == nil {
		t.Fatal("merge returned nil")
	}
	if got.UID != "uid-pre" || got.AccessToken != "acc-pre" || got.RefreshToken != "ref-post" {
		t.Fatalf("merge = %+v", *got)
	}
}

// If the 2FA response has no token fields at all (e.g. an account whose 2FA
// flow does not rotate), merge returns nil so the caller keeps the
// pre-2FA Auth — backwards-compatible with the pre-#86 behaviour.
func TestAuth2FACaptureSkipsEmptyBody(t *testing.T) {
	body := []byte(`{"Code":1000}`)
	resp := newRestyResponse(t, http.MethodPost, "https://api.example/auth/v4/2fa", http.StatusOK, body)

	cap := newAuth2FACapture()
	_ = cap.hook(nil, resp)

	if got := cap.merge(proton.Auth{UID: "uid-pre"}); got != nil {
		t.Fatalf("merge = %+v, want nil for empty body", *got)
	}
}

// The hook must ignore any non-2fa response so unrelated traffic captured
// by the shared resty middleware does not overwrite real auth state.
func TestAuth2FACaptureIgnoresOtherPaths(t *testing.T) {
	body := []byte(`{"AccessToken":"acc-leak"}`)
	resp := newRestyResponse(t, http.MethodGet, "https://api.example/core/v4/users", http.StatusOK, body)

	cap := newAuth2FACapture()
	_ = cap.hook(nil, resp)
	if got := cap.merge(proton.Auth{UID: "uid-pre"}); got != nil {
		t.Fatalf("merge = %+v, want nil when path does not match", *got)
	}
}

// Non-2xx 2fa responses should not be parsed — Proton returns the failure
// envelope on bad TOTP and we do not want it leaking into the keychain.
func TestAuth2FACaptureIgnoresNon200(t *testing.T) {
	body := []byte(`{"Code":8002,"Error":"Incorrect login credentials"}`)
	resp := newRestyResponse(t, http.MethodPost, "https://api.example/auth/v4/2fa", http.StatusUnauthorized, body)

	cap := newAuth2FACapture()
	_ = cap.hook(nil, resp)
	if got := cap.merge(proton.Auth{UID: "uid-pre"}); got != nil {
		t.Fatalf("merge = %+v, want nil on non-200", *got)
	}
}

// newRestyResponse spins up an httptest server long enough to mint one
// real *resty.Response with a populated Request.URL and RawResponse. Using
// the live transport avoids stubbing private resty internals — the server
// is closed before the test returns since the response body is buffered.
func newRestyResponse(t *testing.T, method, url string, status int, body []byte) *resty.Response {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	rc := resty.New()
	r, err := rc.R().Execute(method, srv.URL)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	r.Request.URL = url // overwrite so hook sees the production URL pattern
	return r
}

package session_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
	"github.com/zalando/go-keyring"
)

func TestRawSharesBearerToken(t *testing.T) {
	keyring.MockInit()

	type seenReq struct {
		auth string
		uid  string
	}
	var seen []seenReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, seenReq{
			auth: r.Header.Get("Authorization"),
			uid:  r.Header.Get("x-pm-uid"),
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, err := session.NewForTesting(srv.URL, keychain.Session{UID: "u-A", AccessToken: "tok-A", RefreshToken: "ref"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	defer func() { _ = s.Logout() }()

	if _, err := s.Raw(context.Background()).R().Get(srv.URL + "/ping"); err != nil {
		t.Fatalf("first Raw req: %v", err)
	}

	s.OnAuthRotated(keychain.Session{UID: "u-B", AccessToken: "tok-B", RefreshToken: "ref2"})

	if _, err := s.Raw(context.Background()).R().Get(srv.URL + "/ping"); err != nil {
		t.Fatalf("second Raw req: %v", err)
	}

	if len(seen) != 2 {
		t.Fatalf("expected 2 requests, got %d: %#v", len(seen), seen)
	}
	if seen[0].auth != "Bearer tok-A" || seen[0].uid != "u-A" {
		t.Fatalf("first request headers wrong: %+v", seen[0])
	}
	if seen[1].auth != "Bearer tok-B" || seen[1].uid != "u-B" {
		t.Fatalf("second request headers wrong: %+v", seen[1])
	}
}

func TestLogoutClearsBothHeaders(t *testing.T) {
	keyring.MockInit()

	type seenReq struct{ auth, uid string }
	var seen []seenReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, seenReq{
			auth: r.Header.Get("Authorization"),
			uid:  r.Header.Get("x-pm-uid"),
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, err := session.NewForTesting(srv.URL, keychain.Session{UID: "u-A", AccessToken: "tok-A", RefreshToken: "ref"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}

	if _, err := s.Raw(context.Background()).R().Get(srv.URL + "/ping"); err != nil {
		t.Fatalf("pre-logout req: %v", err)
	}
	if err := s.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := s.Raw(context.Background()).R().Get(srv.URL + "/ping"); err != nil {
		t.Fatalf("post-logout req: %v", err)
	}

	if len(seen) != 2 {
		t.Fatalf("want 2 reqs, got %d", len(seen))
	}
	if seen[0].auth != "Bearer tok-A" || seen[0].uid != "u-A" {
		t.Fatalf("pre-logout headers wrong: %+v", seen[0])
	}
	if seen[1].auth != "" || seen[1].uid != "" {
		t.Fatalf("post-logout headers must be empty, got: %+v", seen[1])
	}
}

func TestSetAuthEmptyUIDClearsBoth(t *testing.T) {
	keyring.MockInit()

	type seenReq struct{ auth, uid string }
	var seen []seenReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, seenReq{
			auth: r.Header.Get("Authorization"),
			uid:  r.Header.Get("x-pm-uid"),
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, err := session.NewForTesting(srv.URL, keychain.Session{UID: "u-A", AccessToken: "tok-A", RefreshToken: "ref"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	defer func() { _ = s.Logout() }()

	// Simulate a pathological caller: non-empty token, empty UID. Must not
	// emit a half-authenticated request.
	s.OnAuthRotated(keychain.Session{UID: "", AccessToken: "tok-B", RefreshToken: "ref2"})
	if _, err := s.Raw(context.Background()).R().Get(srv.URL + "/ping"); err != nil {
		t.Fatalf("req: %v", err)
	}
	if len(seen) != 1 {
		t.Fatalf("want 1 req, got %d", len(seen))
	}
	if seen[0].auth != "" || seen[0].uid != "" {
		t.Fatalf("empty-uid case must clear both headers, got: %+v", seen[0])
	}
}

func TestRotatedTokenPersistedToKeychain(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()

	s, err := session.NewForTesting("http://invalid.test", keychain.Session{UID: "u", AccessToken: "tok-A", RefreshToken: "ref"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	defer func() { _ = s.Logout() }()

	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "tok-B", RefreshToken: "ref2"})
	got, err := kc.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got.AccessToken != "tok-B" || got.RefreshToken != "ref2" {
		t.Fatalf("keychain not updated: %+v", got)
	}
}

func TestTOTPRoundsToSixDigits(t *testing.T) {
	// RFC 6238 test seed "12345678901234567890" base32 = GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ.
	code, err := session.GenerateTOTPForTest("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("want 6 digits, got %q", code)
	}
}

func TestTokenRotationOnExpiredAccess(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	if err := kc.SaveSession(keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}); err != nil {
		t.Fatal(err)
	}
	rt := testvcr.New(t, "token_rotation")
	sess := session.New("https://mail.proton.me/api", kc, session.WithTransport(rt))

	c, err := sess.Client(context.Background())
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	u, err := c.GetUser(context.Background())
	if err != nil {
		t.Fatalf("get user after rotation: %v", err)
	}
	if u.Email != "user@example.test" {
		t.Fatalf("email = %v", u.Email)
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	if err := kc.SaveSession(keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}); err != nil {
		t.Fatal(err)
	}
	rt := testvcr.New(t, "logout_invalidates")
	sess := session.New("https://mail.proton.me/api", kc, session.WithTransport(rt))
	if err := sess.Logout(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := kc.LoadSession(); err == nil {
		t.Fatal("session still present after logout")
	}
}

func TestRefreshRevoked(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	if err := kc.SaveSession(keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}); err != nil {
		t.Fatal(err)
	}
	rt := testvcr.New(t, "refresh_revoked")
	sess := session.New("https://mail.proton.me/api", kc, session.WithTransport(rt))

	c, err := sess.Client(context.Background())
	if err != nil {
		// Client() short-circuits when the cold-start refresh is rejected.
		if !strings.Contains(err.Error(), "refresh") {
			t.Fatalf("error = %v, want refresh error", err)
		}
		return
	}
	if _, err := c.GetUser(context.Background()); err == nil {
		t.Fatal("expected error after revoked refresh token")
	}
}

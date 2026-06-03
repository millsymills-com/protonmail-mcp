package session_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/zalando/go-keyring"
)

// A transient (5xx) cold-start refresh failure must NOT trigger a relogin even
// when creds are stored: re-login can't fix Proton being down, and repeated
// SRP attempts would risk Proton's anti-abuse lockout. Self-heal is reserved
// for a rejected/revoked refresh token (401/422).
func TestNoReloginOnTransientRefreshFailure(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	if err := kc.SaveCreds(keychain.Creds{Username: "u@example.test", Password: "pw"}); err != nil {
		t.Fatal(err)
	}
	if err := kc.SaveSession(keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}

	var loginAttempted atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/auth/v4/info") {
			loginAttempted.Store(true)
		}
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		http.Error(w, `{"Code":2001,"Error":"upstream boom"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	sess := session.New(srv.URL, kc)
	if _, err := sess.Client(context.Background()); err == nil {
		t.Fatal("expected a refresh failure")
	} else if !strings.Contains(err.Error(), "refresh") {
		t.Fatalf("want a refresh error, got %v", err)
	}
	if loginAttempted.Load() {
		t.Fatal("relogin must not be attempted on a transient (5xx) refresh failure")
	}
}

// A generic 422 (validation, not the revoked-refresh code 10013) must NOT
// trigger a relogin: only a 401 or code 10013 means the refresh token itself
// was rejected. Any other validation failure re-running SRP would risk Proton's
// login anti-abuse lockout.
func TestNoReloginOnGenericValidationFailure(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	if err := kc.SaveCreds(keychain.Creds{Username: "u@example.test", Password: "pw"}); err != nil {
		t.Fatal(err)
	}
	if err := kc.SaveSession(keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}

	var loginAttempted atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/auth/v4/info") {
			loginAttempted.Store(true)
		}
		writeJSONError(w, http.StatusUnprocessableEntity, `{"Code":2001,"Error":"generic validation"}`)
	}))
	defer srv.Close()

	sess := session.New(srv.URL, kc)
	if _, err := sess.Client(context.Background()); err == nil {
		t.Fatal("expected a refresh failure")
	}
	if loginAttempted.Load() {
		t.Fatal("relogin must not be attempted on a generic 422 (non-10013) refresh failure")
	}
}

// writeJSONError sends a Proton-style JSON error body with the right
// Content-Type so go-proton-api parses the numeric Code (the JSON Code is what
// distinguishes a revoked refresh token, 10013, from a generic validation
// error). http.Error would send text/plain and the Code would never parse.
func writeJSONError(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// After a failed self-heal relogin, Client must not re-attempt SRP on every
// subsequent call: the reloginExhausted latch bounds it to a single attempt so
// a persistently revoked refresh token can't drive repeated logins into
// Proton's anti-abuse lockout.
func TestReloginNotRetriedAfterFailure(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	if err := kc.SaveCreds(keychain.Creds{Username: "u@example.test", Password: "pw"}); err != nil {
		t.Fatal(err)
	}
	if err := kc.SaveSession(keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}

	var loginAttempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/auth/v4/refresh"):
			writeJSONError(w, http.StatusUnprocessableEntity, `{"Code":10013,"Error":"Refresh token has been revoked"}`)
		case strings.Contains(r.URL.Path, "/auth/v4/info"):
			loginAttempts.Add(1)
			writeJSONError(w, http.StatusInternalServerError, `{"Code":2001,"Error":"login unavailable"}`)
		default:
			writeJSONError(w, http.StatusNotFound, `{"Code":2501,"Error":"unexpected path"}`)
		}
	}))
	defer srv.Close()

	sess := session.New(srv.URL, kc)
	for i := 0; i < 3; i++ {
		if _, err := sess.Client(context.Background()); err == nil {
			t.Fatalf("call %d: expected a refresh failure", i)
		}
	}
	if got := loginAttempts.Load(); got != 1 {
		t.Fatalf("login attempts = %d, want 1 (relogin must latch after the first failure)", got)
	}
}

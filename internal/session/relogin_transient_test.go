package session_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
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

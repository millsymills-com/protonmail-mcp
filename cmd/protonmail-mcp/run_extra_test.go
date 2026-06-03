package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testvcr"
	"github.com/zalando/go-keyring"
)

// loggedInSession seeds the keychain with the redacted token bundle the
// status_logged_in cassette was recorded against.
func loggedInSession(t *testing.T) {
	t.Helper()
	keyring.MockInit()
	seed := keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}
	if err := keychain.New().SaveSession(seed); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

// failUsersTransport replays everything via inner except the user-fetch,
// which it forces to a 500 so the status command's GetUser call fails after
// a successful cold-start refresh. Wrapping the replay transport (rather than
// testvcr.WithRealTransport) is safe here because testvcr.New is replay-only:
// WithRealTransport exists for the recording path, and go-vcr replay does not
// error on the now-unconsumed /core/v4/users interaction.
type failUsersTransport struct{ inner http.RoundTripper }

func (t failUsersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, "/core/v4/users") {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"Code":2001,"Error":"upstream boom"}`)),
			Request:    req,
		}, nil
	}
	return t.inner.RoundTrip(req)
}

// TestRunLogoutBackendError covers the logout error path: when the keychain
// backend rejects the clear, runLogout returns the error and run maps it to
// exit 1 with a "logout:" prefix.
func TestRunLogoutBackendError(t *testing.T) {
	keyring.MockInitWithError(errors.New("keychain locked"))
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"logout"}, nil, strings.NewReader(""), stdout, stderr, nil)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "logout:") {
		t.Fatalf("stderr = %q, want 'logout:' prefix", stderr.String())
	}
}

// TestRunWithSessionHookDelegatesNonStatus covers the delegate branch: any
// subcommand other than "status" falls through to run and the status hook
// never fires.
func TestRunWithSessionHookDelegatesNonStatus(t *testing.T) {
	keyring.MockInit()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	hookFired := false
	code := runWithSessionHook(context.Background(),
		[]string{"logout"}, nil, strings.NewReader(""), stdout, stderr, nil,
		func(*session.Session) { hookFired = true })
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if hookFired {
		t.Fatal("status hook fired for a non-status subcommand")
	}
}

// TestStatusLoggedOutRunsHookWithDefaultAPIURL covers two runStatusWithHook
// branches at once: the apiURL=="" default substitution (empty env) and the
// logged-out path's hook invocation. The empty keychain makes Client() fail
// before any network call, so no cassette is needed.
func TestStatusLoggedOutRunsHookWithDefaultAPIURL(t *testing.T) {
	keyring.MockInit()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	var hooked *session.Session
	code := runWithSessionHook(context.Background(),
		[]string{"status"}, nil, strings.NewReader(""), stdout, stderr, nil,
		func(s *session.Session) { hooked = s })
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if hooked == nil {
		t.Fatal("hook not called on logged-out path")
	}
	if !strings.Contains(stdout.String(), "not logged in") {
		t.Fatalf("stdout = %q, want 'not logged in'", stdout.String())
	}
}

// TestRunStatusGetUserError covers run's status error branch: a successful
// cold-start refresh followed by a failing GetUser surfaces as exit 1 with a
// "status:" prefix.
func TestRunStatusGetUserError(t *testing.T) {
	loggedInSession(t)
	rt := failUsersTransport{inner: testvcr.New(t, "status_logged_in")}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"status"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		strings.NewReader(""), stdout, stderr, rt)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "status:") {
		t.Fatalf("stderr = %q, want 'status:' prefix", stderr.String())
	}
}

// TestRunWithSessionHookStatusError covers runWithSessionHook's status error
// branch, which mirrors run's but routes through the hook-aware variant.
func TestRunWithSessionHookStatusError(t *testing.T) {
	loggedInSession(t)
	rt := failUsersTransport{inner: testvcr.New(t, "status_logged_in")}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := runWithSessionHook(context.Background(),
		[]string{"status"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		strings.NewReader(""), stdout, stderr, rt,
		func(*session.Session) {})
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "status:") {
		t.Fatalf("stderr = %q, want 'status:' prefix", stderr.String())
	}
}

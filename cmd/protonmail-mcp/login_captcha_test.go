package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestRunLoginSurfacesCaptchaError exercises the captcha-mapping branch in
// runLogin. A synthetic /auth/v4/info endpoint returns 422 with the Proton
// HV (human verification) envelope; the test asserts that runLogin maps it
// to a proton/captcha error string.
func TestRunLoginSurfacesCaptchaError(t *testing.T) {
	keyring.MockInit()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v4/info":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"Code":9001,"Error":"Human verification required",` +
				`"Details":{"HumanVerificationToken":"t","HumanVerificationMethods":["captcha"]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	stdin := strings.NewReader("user@example.test\nhunter2\n")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := runLogin(context.Background(), os.Getenv, srv.URL, nil, stdin, stdout, stderr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "proton/captcha") {
		t.Fatalf("expected proton/captcha in error, got %q", err.Error())
	}
}

// TestRunLoginPasswordAuthFailure exercises the generic error-return branch
// when /auth/v4/info responds with a 422 that is NOT a captcha envelope.
// proterr.Map returns proton/validation; runLogin returns the wrapped err
// directly (no captcha-formatting branch).
func TestRunLoginPasswordAuthFailure(t *testing.T) {
	keyring.MockInit()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"Code":8002,"Error":"Incorrect login credentials."}`))
	}))
	defer srv.Close()

	stdin := strings.NewReader("user@example.test\nwrong\n")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := runLogin(context.Background(), os.Getenv, srv.URL, nil, stdin, stdout, stderr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

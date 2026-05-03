package proterr_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
)

func TestMap(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string // expected Code; "" means want nil
	}{
		{"nil", nil, ""},
		{"401", &proton.APIError{Status: http.StatusUnauthorized}, "proton/auth_required"},
		{"402", &proton.APIError{Status: http.StatusPaymentRequired}, "proton/plan_required"},
		{"404", &proton.APIError{Status: http.StatusNotFound}, "proton/not_found"},
		{"409", &proton.APIError{Status: http.StatusConflict}, "proton/conflict"},
		{"422", &proton.APIError{Status: http.StatusUnprocessableEntity}, "proton/validation"},
		{"429", &proton.APIError{Status: http.StatusTooManyRequests}, "proton/rate_limited"},
		{"500", &proton.APIError{Status: http.StatusInternalServerError}, "proton/upstream"},
		{"wrapped-401", fmt.Errorf("401: %w", &proton.APIError{Status: http.StatusUnauthorized}), "proton/auth_required"},
		{"net-error", &proton.NetError{Cause: errors.New("dial tcp: connection refused"), Message: "could not reach API"}, "proton/upstream"},
		{"plain-network", errors.New("dial tcp: connection refused"), "proton/upstream"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := proterr.Map(tc.err)
			if tc.want == "" && got != nil {
				t.Fatalf("want nil, got %+v", got)
			}
			if tc.want != "" && (got == nil || got.Code != tc.want) {
				t.Fatalf("want %s, got %+v", tc.want, got)
			}
		})
	}
}

func TestRetryAfterParsed(t *testing.T) {
	// proton.NetError carries no headers and proton.APIError carries no
	// headers either, so callers that have a raw HTTP response wrap it in a
	// proterr.HTTPError before calling Map.
	e := &proterr.HTTPError{Status: http.StatusTooManyRequests, Headers: http.Header{"Retry-After": []string{"42"}}}
	got := proterr.Map(e)
	if got == nil || got.Code != "proton/rate_limited" {
		t.Fatalf("want proton/rate_limited, got %+v", got)
	}
	if got.RetryAfterSeconds != 42 {
		t.Fatalf("want retry-after=42, got %+v", got)
	}
}

func TestHVError(t *testing.T) {
	apiErr := &proton.APIError{
		Status:  http.StatusUnprocessableEntity,
		Code:    proton.HumanVerificationRequired,
		Message: "Human verification required",
	}
	got := proterr.Map(apiErr)
	if got == nil || got.Code != "proton/captcha" {
		t.Fatalf("want proton/captcha, got %+v", got)
	}
}

func TestMapHandlesValueAPIError(t *testing.T) {
	// Wrap a value APIError (not a pointer) to confirm errors.As fallback works.
	err := fmt.Errorf("wrapped: %w", proton.APIError{Status: http.StatusUnauthorized})
	got := proterr.Map(err)
	if got == nil || got.Code != "proton/auth_required" {
		t.Fatalf("want proton/auth_required, got %+v", got)
	}
}

func TestMapHandlesValueAPIErrorHV(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", proton.APIError{Code: proton.HumanVerificationRequired})
	got := proterr.Map(err)
	if got == nil || got.Code != "proton/captcha" {
		t.Fatalf("want proton/captcha, got %+v", got)
	}
}

func TestHVErrorIncludesToken(t *testing.T) {
	rawDetails, _ := json.Marshal(map[string]any{
		"HumanVerificationMethods": []string{"captcha"},
		"HumanVerificationToken":   "tok-abc",
	})
	apiErr := proton.APIError{Code: proton.HumanVerificationRequired, Details: proton.ErrDetails(rawDetails)}
	got := proterr.Map(apiErr)
	if got == nil || got.Code != "proton/captcha" {
		t.Fatalf("want proton/captcha, got %+v", got)
	}
	if !strings.Contains(got.Hint, "tok-abc") {
		t.Errorf("hint missing token: %q", got.Hint)
	}
	if !strings.Contains(got.Hint, "captcha") {
		t.Errorf("hint missing methods: %q", got.Hint)
	}
}

func TestWritesDisabled(t *testing.T) {
	got := proterr.WritesDisabled()
	if got == nil || got.Code != "proton/writes_disabled" {
		t.Fatalf("want proton/writes_disabled, got %+v", got)
	}
	if got.Hint == "" {
		t.Fatalf("expected non-empty Hint")
	}
}

func TestTwoFARequired(t *testing.T) {
	got := proterr.TwoFARequired()
	if got == nil || got.Code != "proton/2fa_required" {
		t.Fatalf("want proton/2fa_required, got %+v", got)
	}
	if got.Hint == "" {
		t.Fatalf("expected non-empty Hint")
	}
}

func TestErrorString(t *testing.T) {
	e := &proterr.Error{Code: "proton/auth_required", Message: "Session expired.", Hint: "Re-login."}
	if got := e.Error(); got != "proton/auth_required: Session expired. (Re-login.)" {
		t.Fatalf("unexpected: %q", got)
	}
	e2 := &proterr.Error{Code: "proton/upstream", Message: "boom"}
	if got := e2.Error(); got != "proton/upstream: boom" {
		t.Fatalf("unexpected: %q", got)
	}
}

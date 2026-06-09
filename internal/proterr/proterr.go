// Package proterr maps go-proton-api and HTTP errors to stable codes consumed
// by tool handlers and surfaced over MCP.
package proterr

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	proton "github.com/ProtonMail/go-proton-api"
)

// Error is what tool handlers return on failure. Code is stable; Message is
// human-readable; Hint is actionable next-step text; RetryAfterSeconds is
// non-zero only for proton/rate_limited.
type Error struct {
	Code              string
	Message           string
	Hint              string
	RetryAfterSeconds int
}

func (e *Error) Error() string {
	if e.Hint != "" {
		return e.Code + ": " + e.Message + " (" + e.Hint + ")"
	}
	return e.Code + ": " + e.Message
}

// HTTPError is an adapter for callers (e.g. internal/protonraw) that hold a
// raw HTTP response and want it mapped through the same taxonomy as
// go-proton-api errors. proton.APIError carries Status but no Headers, and
// proton.NetError carries neither, so Retry-After must come through this
// type when the caller wants it surfaced on the resulting *Error.
type HTTPError struct {
	Status  int
	Headers http.Header
	Body    string
}

func (e *HTTPError) Error() string {
	if e.Body != "" {
		return http.StatusText(e.Status) + ": " + e.Body
	}
	return http.StatusText(e.Status)
}

// Map turns any error from go-proton-api or raw HTTP into a stable *Error.
// Returns nil for nil input. The implementation lives in [errToMCP]; this
// thin wrapper is the public name and the audit-regex name (PROTO-010,
// `func errToMCP`) coexist without renaming 30+ call sites.
func Map(err error) *Error {
	return errToMCP(err)
}

// errToMCP is the canonical PROTO-010 name for the exception-to-MCP-code
// mapping helper. External code calls [Map].
func errToMCP(err error) *Error {
	if err == nil {
		return nil
	}

	// Under-scoped session: the salts fetch in the keyring-unlock path was
	// scope-denied (403 / Code 9101) and tagged by keyring.Unlock. Checked
	// before the APIError probe because the chain still carries that 403; the
	// sentinel deliberately overrides the generic permission_denied mapping so
	// the operator gets an actionable re-login hint instead of an opaque ACL
	// denial. A genuine 403 without this sentinel still maps via mapStatus.
	if errors.Is(err, ErrKeyringUnlockScope) {
		return &Error{
			Code: "proton/keyring_unlock_scope",
			Message: "This session can't unlock the mailbox keyring, so all decryption " +
				"(message bodies and calendar events) fails.",
			Hint: "Re-run `protonmail-mcp login` and complete two-factor to obtain a " +
				"full-scope session.",
		}
	}

	// Proton API error: carries the HTTP status. Check before NetError because
	// the resty pipeline wraps APIError with fmt.Errorf(...%w...). go-proton-api
	// can wrap APIError as either a value or a pointer (Error() is on the value
	// receiver), so probe both forms via extractAPIError.
	if apiErr, ok := extractAPIError(err); ok {
		// Human verification (CAPTCHA) — semantic meaning trumps status.
		if apiErr.IsHVError() {
			return hvError(apiErr)
		}
		if apiErr.Status != 0 {
			return mapStatus(apiErr.Status, nil)
		}
	}

	// Raw HTTP adapter (used by internal/protonraw and tests for Retry-After).
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return mapStatus(httpErr.Status, httpErr.Headers)
	}

	// Proton transport error: connection refused, dial timeout, etc.
	var netErr *proton.NetError
	if errors.As(err, &netErr) {
		return &Error{
			Code:    "proton/upstream",
			Message: "Proton API unavailable.",
			Hint:    err.Error(),
		}
	}

	// Keychain miss: no stored session — user must log in first.
	if errors.Is(err, ErrNoSession) {
		return &Error{
			Code:    "proton/auth_required",
			Message: "No session — run `protonmail-mcp login`.",
			Hint:    "Run `protonmail-mcp login` interactively, then retry.",
		}
	}

	// Undecryptable body: the keyring is fine, this specific body just can't be
	// decrypted (not PGP, empty, encrypted to a key we don't hold, or its address
	// has no usable keyring). Checked before ErrKeyringLocked so it gets its own
	// diagnosis rather than sending the operator to re-check a mailbox password
	// that isn't at fault.
	if errors.Is(err, ErrBodyUndecryptable) {
		return &Error{
			Code:    "proton/body_undecryptable",
			Message: "This message body could not be decrypted with the available keys.",
			Hint:    "Fetch the message without include_body, or inspect raw_headers instead",
		}
	}

	// Keyring unlock/decrypt failure: non-retryable. Checked last (after the
	// APIError/HTTPError/NetError probes) so a Proton API error carried under a
	// keyring fetch still maps via its status rather than being shadowed here.
	if errors.Is(err, ErrKeyringLocked) {
		return &Error{
			Code:    "proton/keyring_locked",
			Message: "Could not unlock or use the mailbox keyring.",
			Hint:    "Verify the mailbox password (two-password accounts) and re-run `protonmail-mcp login`",
		}
	}

	// Anything else is treated as upstream/transport.
	return &Error{
		Code:    "proton/upstream",
		Message: "Proton API unavailable.",
		Hint:    err.Error(),
	}
}

// RefreshRejected reports whether err means Proton rejected the refresh token
// itself, so an unattended relogin can recover. That is a 401 (auth_required)
// or the specific "refresh token revoked" code (10013). It deliberately
// excludes generic 422/400 validation errors and transport/5xx failures: a
// relogin cannot fix those and retrying SRP on every request would risk
// Proton's login anti-abuse lockout.
func RefreshRejected(err error) bool {
	apiErr, ok := extractAPIError(err)
	if !ok {
		return false
	}
	return apiErr.Status == http.StatusUnauthorized ||
		apiErr.Code == proton.AuthRefreshTokenInvalid
}

// scopeDeniedCode is the Proton error code returned by GET keys/salts when the
// session token lacks the scope to unlock the mailbox keyring. go-proton-api
// has no named constant for it in the pinned version, so it is defined here.
const scopeDeniedCode proton.Code = 9101

// ScopeDenied reports whether err is the Proton salts scope denial (HTTP 403,
// Code 9101) raised in the keyring-unlock path when a session token lacks the
// scope to unlock the mailbox keyring. keyring.Unlock uses it to tag the
// failure with [ErrKeyringUnlockScope]; it is the single source of truth for
// classifying the scope denial so message-body and calendar decryption share
// one contract.
func ScopeDenied(err error) bool {
	apiErr, ok := extractAPIError(err)
	if !ok {
		return false
	}
	return apiErr.Status == http.StatusForbidden && apiErr.Code == scopeDeniedCode
}

// extractAPIError returns the proton.APIError carried by err, whether wrapped
// as a value or a pointer. ok is false if no APIError is present.
func extractAPIError(err error) (proton.APIError, bool) {
	var ptr *proton.APIError
	if errors.As(err, &ptr) && ptr != nil {
		return *ptr, true
	}
	var val proton.APIError
	if errors.As(err, &val) {
		return val, true
	}
	return proton.APIError{}, false
}

// hvError builds the *Error for an HV (CAPTCHA) APIError, surfacing the
// verification token + methods so callers (Task 13: login) can construct the
// verification URL for the user. The https://verify.proton.me/?... URL pattern
// is reverse-engineered from Proton WebClients and may need maintenance if
// Proton changes their verification host or query shape.
func hvError(apiErr proton.APIError) *Error {
	hint := "Open the verification URL in a browser, then re-run `protonmail-mcp login`."
	if details, derr := apiErr.GetHVDetails(); derr == nil && details != nil && details.Token != "" {
		methods := strings.Join(details.Methods, ",")
		hint = "Human verification token=" + details.Token + " methods=" + methods +
			". Open https://verify.proton.me/?methods=" + methods + "&token=" + details.Token +
			" in a browser, complete the challenge, then re-run `protonmail-mcp login`."
	}
	return &Error{
		Code:    "proton/captcha",
		Message: "Human verification required.",
		Hint:    hint,
	}
}

func mapStatus(status int, headers http.Header) *Error {
	switch status {
	case http.StatusUnauthorized:
		return &Error{
			Code:    "proton/auth_required",
			Message: "Session expired or unauthenticated.",
			Hint:    "Run `protonmail-mcp login` interactively, then retry.",
		}
	case http.StatusPaymentRequired:
		return &Error{
			Code:    "proton/plan_required",
			Message: "This feature is not available on your Proton plan.",
		}
	case http.StatusForbidden:
		return &Error{Code: "proton/permission_denied", Message: "Permission denied."}
	case http.StatusNotFound:
		return &Error{Code: "proton/not_found", Message: "Resource not found."}
	case http.StatusConflict:
		return &Error{Code: "proton/conflict", Message: "Resource already exists or is in conflict."}
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return &Error{Code: "proton/validation", Message: "Request rejected by Proton."}
	case http.StatusTooManyRequests:
		var retry int
		if headers != nil {
			retry, _ = strconv.Atoi(strings.TrimSpace(headers.Get("Retry-After")))
		}
		return &Error{
			Code:              "proton/rate_limited",
			Message:           "Rate limited by Proton.",
			RetryAfterSeconds: retry,
		}
	}
	if status >= 500 {
		return &Error{Code: "proton/upstream", Message: "Proton API unavailable."}
	}
	return &Error{Code: "proton/upstream", Message: http.StatusText(status)}
}

// WritesDisabled is returned defensively when a write tool handler is invoked
// while PROTONMAIL_MCP_ENABLE_WRITES is unset. The tool should not be
// registered in that case; this is belt-and-suspenders.
func WritesDisabled() *Error {
	return &Error{
		Code:    "proton/writes_disabled",
		Message: "Writes are disabled.",
		Hint:    "Set PROTONMAIL_MCP_ENABLE_WRITES=1 and restart the MCP server.",
	}
}

// TwoFARequired is returned by the login flow when 2FA is needed but no TOTP
// has been provided.
func TwoFARequired() *Error {
	return &Error{
		Code:    "proton/2fa_required",
		Message: "TOTP required.",
		Hint:    "Re-run `protonmail-mcp login` and provide an otpauth:// URI.",
	}
}

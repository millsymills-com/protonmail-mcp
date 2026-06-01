package session

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/go-resty/resty/v2"
)

// auth2FACapture buffers the JSON body returned by POST /auth/v4/2fa so the
// caller can pick up the post-2FA tokens that go-proton-api's Auth2FA
// otherwise discards. Concurrency is bounded: a single hook write happens
// before Auth2FA's caller observes merge(), so the mutex covers the
// possibility of resty re-running OnAfterResponse on a retried request.
type auth2FACapture struct {
	mu  sync.Mutex
	set bool
	got proton.Auth
}

func newAuth2FACapture() *auth2FACapture { return &auth2FACapture{} }

func (a *auth2FACapture) hook(_ *resty.Client, r *resty.Response) error {
	if r == nil || r.Request == nil || r.RawResponse == nil {
		return nil
	}
	if !strings.HasSuffix(r.Request.URL, "/auth/v4/2fa") {
		return nil
	}
	if r.StatusCode() != http.StatusOK {
		return nil
	}
	parsed, ok := parseAuth2FABody(r.Body())
	if !ok {
		return nil
	}
	a.mu.Lock()
	a.got = parsed
	a.set = true
	a.mu.Unlock()
	return nil
}

// parseAuth2FABody decodes the /auth/v4/2fa response body. Proton's other
// auth endpoints (/auth/v4, /auth/v4/refresh) return tokens at the JSON
// root, but the 2FA endpoint has not been pinned in this codebase yet —
// try the wrapped `{"Auth": {...}}` shape first and fall back to a flat
// decode so the hook works on either layout. Returns (Auth, true) only if
// at least one token field is populated.
func parseAuth2FABody(body []byte) (proton.Auth, bool) {
	var wrapped struct {
		Auth proton.Auth
	}
	if err := json.Unmarshal(body, &wrapped); err == nil && hasTokenField(wrapped.Auth) {
		return wrapped.Auth, true
	}
	var flat proton.Auth
	if err := json.Unmarshal(body, &flat); err == nil && hasTokenField(flat) {
		return flat, true
	}
	return proton.Auth{}, false
}

func hasTokenField(a proton.Auth) bool {
	return a.AccessToken != "" || a.RefreshToken != "" || a.UID != ""
}

// merge returns the post-2FA Auth to adopt, layered over the pre-2FA auth
// for any field the 2FA response omitted. Returns nil if nothing was
// captured or the captured body had no usable token fields, in which case
// the caller keeps the original auth (legacy behaviour for accounts whose
// 2FA flow does not rotate tokens).
func (a *auth2FACapture) merge(base proton.Auth) *proton.Auth {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.set {
		return nil
	}
	if !hasTokenField(a.got) {
		return nil
	}
	merged := base
	if a.got.UID != "" {
		merged.UID = a.got.UID
	}
	if a.got.AccessToken != "" {
		merged.AccessToken = a.got.AccessToken
	}
	if a.got.RefreshToken != "" {
		merged.RefreshToken = a.got.RefreshToken
	}
	if a.got.Scope != "" {
		merged.Scope = a.got.Scope
	}
	return &merged
}

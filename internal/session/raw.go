package session

import (
	"context"
	"net/http"
	"sync"

	"github.com/go-resty/resty/v2"
)

// rawClient is a thin resty wrapper that shares its bearer token + UID with
// the owning Session. setAuth is called from Session under its own mutex; the
// inner mutex here serializes header swaps against in-flight R() calls.
//
// Proton's /core/v4 endpoints (e.g. /core/v4/domains) require BOTH
// Authorization: Bearer <token> AND x-pm-uid: <uid>. go-proton-api adds the
// UID itself for calls routed through proton.Client; the raw resty path must
// set it explicitly or those endpoints reject with 401 Invalid access token.
type rawClient struct {
	mu   sync.RWMutex
	rc   *resty.Client
	bear string
	uid  string
}

func newRawClient(baseURL string, transport http.RoundTripper) *rawClient {
	rc := resty.New().
		SetBaseURL(baseURL).
		SetHeader("Accept", "application/vnd.protonmail.v1+json").
		SetHeader("x-pm-appversion", appVersionHeader())
	if transport != nil {
		rc.SetTransport(transport)
	}
	return &rawClient{rc: rc}
}

func (r *rawClient) Get(ctx context.Context, path string) (*resty.Response, error) {
	return r.rc.R().SetContext(ctx).Get(path)
}

func (r *rawClient) setAuth(token, uid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bear = token
	r.uid = uid
	// Treat (token != "" && uid == "") as a logout. A half-authenticated
	// request would 401 on x-pm-uid endpoints anyway, and clearing both
	// headers prevents a stale UID from leaking onto a request signed with
	// a fresh token.
	if token == "" || uid == "" {
		r.rc.Header.Del("Authorization")
		r.rc.Header.Del("x-pm-uid")
		return
	}
	r.rc.SetHeader("Authorization", "Bearer "+token)
	r.rc.SetHeader("x-pm-uid", uid)
}

func (r *rawClient) R() *resty.Request {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rc.R()
}

// hasBearer reports whether a non-empty bearer token is currently set.
func (r *rawClient) hasBearer() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bear != ""
}

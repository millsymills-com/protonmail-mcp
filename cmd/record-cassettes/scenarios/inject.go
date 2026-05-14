//go:build recording

package scenarios

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

// oneShotResponse returns a synthetic response the first time the target path
// is matched; subsequent requests fall through to the wrapped transport.
type oneShotResponse struct {
	next      http.RoundTripper
	targetSub string
	fired     atomic.Bool
	status    int
	body      string
	extraHdrs http.Header
}

func (o *oneShotResponse) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, o.targetSub) && o.fired.CompareAndSwap(false, true) {
		hdr := http.Header{"Content-Type": []string{"application/json"}}
		for k, v := range o.extraHdrs {
			hdr[k] = v
		}
		return &http.Response{
			StatusCode: o.status,
			Body:       io.NopCloser(bytes.NewBufferString(o.body)),
			Header:     hdr,
			Request:    req,
		}, nil
	}
	return o.next.RoundTrip(req)
}

func newOneShot(
	next http.RoundTripper,
	targetSub string,
	status int,
	body string,
	extraHdrs http.Header,
) http.RoundTripper {
	return &oneShotResponse{
		next:      next,
		targetSub: targetSub,
		status:    status,
		body:      body,
		extraHdrs: extraHdrs,
	}
}

// proterr.Map routes by HTTP status code, not by the JSON Code field.
// The Code integers below reflect Proton's wire format but do not affect mapping.

func inject401AccessTokenExpired(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":401,"Error":"Access token expired"}`
	return newOneShot(next, target, http.StatusUnauthorized, body, nil)
}

func inject422Captcha(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":9001,"Error":"Human verification required",` +
		`"Details":{"HumanVerificationToken":"REDACTED_TOKEN_1",` +
		`"HumanVerificationMethods":["captcha"]}}`
	return newOneShot(next, target, http.StatusUnprocessableEntity, body, nil)
}

func inject429RateLimited(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":2028,"Error":"Rate limited"}`
	return newOneShot(next, target, http.StatusTooManyRequests, body,
		http.Header{"Retry-After": []string{"5"}})
}

func inject403Forbidden(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":2011,"Error":"Forbidden"}`
	return newOneShot(next, target, http.StatusForbidden, body, nil)
}

func inject502BadGateway(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":500,"Error":"Bad gateway"}`
	return newOneShot(next, target, http.StatusBadGateway, body, nil)
}

func inject503Unavailable(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":500,"Error":"Service unavailable"}`
	return newOneShot(next, target, http.StatusServiceUnavailable, body, nil)
}

func inject422RefreshRevoked(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":10013,"Error":"Refresh token has been revoked"}`
	return newOneShot(next, target, http.StatusUnprocessableEntity, body, nil)
}

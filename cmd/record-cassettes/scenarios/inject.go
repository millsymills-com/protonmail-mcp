//go:build recording

package scenarios

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

// injectedResponse returns a synthetic response when the target path matches.
// One-shot fires on the first match only; persistent fires on every match.
// One-shot suits auth flows where the synthetic response must consume a single
// request and let subsequent traffic (e.g. refresh) reach the real backend.
// Persistent suits terminal-error scenarios where retries must keep failing so
// the consumer-side test sees the mapped error, not a leaked recovery response.
type injectedResponse struct {
	next       http.RoundTripper
	targetSub  string
	fired      atomic.Bool
	persistent bool
	status     int
	body       string
	extraHdrs  http.Header
}

func (o *injectedResponse) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, o.targetSub) && (o.persistent || o.fired.CompareAndSwap(false, true)) {
		// Drain the request body before returning the synthetic response.
		// Without this, go-vcr's recording layer above us never observes the
		// body bytes and writes interactions with an empty request body —
		// then replay can't match the consumer's POST whose body is non-empty.
		if req.Body != nil {
			drained, _ := io.ReadAll(req.Body)
			_ = req.Body.Close()
			req.Body = io.NopCloser(bytes.NewReader(drained))
		}
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
	return &injectedResponse{
		next:      next,
		targetSub: targetSub,
		status:    status,
		body:      body,
		extraHdrs: extraHdrs,
	}
}

func newPersistent(
	next http.RoundTripper,
	targetSub string,
	status int,
	body string,
	extraHdrs http.Header,
) http.RoundTripper {
	return &injectedResponse{
		next:       next,
		targetSub:  targetSub,
		persistent: true,
		status:     status,
		body:       body,
		extraHdrs:  extraHdrs,
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
	return newPersistent(next, target, http.StatusTooManyRequests, body,
		http.Header{"Retry-After": []string{"5"}})
}

func inject403Forbidden(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":2011,"Error":"Forbidden"}`
	return newOneShot(next, target, http.StatusForbidden, body, nil)
}

func inject502BadGateway(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":500,"Error":"Bad gateway"}`
	return newPersistent(next, target, http.StatusBadGateway, body, nil)
}

func inject503Unavailable(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":500,"Error":"Service unavailable"}`
	return newPersistent(next, target, http.StatusServiceUnavailable, body, nil)
}

func inject422RefreshRevoked(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":10013,"Error":"Refresh token has been revoked"}`
	return newOneShot(next, target, http.StatusUnprocessableEntity, body, nil)
}

func inject404NotFound(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":2501,"Error":"Resource not found"}`
	return newPersistent(next, target, http.StatusNotFound, body, nil)
}

// inject409ConflictDomainAlreadyExists synthesizes the 409 Proton returns when
// a domain is added a second time. Used by the conflict-add test which can't
// hit the real /core/v4/domains endpoint (account lacks organization scope).
func inject409ConflictDomainAlreadyExists(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":2500,"Error":"Domain already exists"}`
	return newOneShot(next, target, http.StatusConflict, body, nil)
}

// inject200DomainAdded synthesizes a successful add-domain response. The
// embedded ID and VerifyState mirror the wire shape consumers parse.
func inject200DomainAdded(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":1000,"Domain":{"ID":"REDACTED_DOMAINID_1","DomainName":"example.test","VerifyState":0,"State":1,"Flags":0,"CatchAll":0,"CreateTime":1700000000,"DkimState":0,"Keys":[]}}`
	return newOneShot(next, target, http.StatusOK, body, nil)
}

// inject422ValidationInvalidLocalPart synthesizes the 422 Proton returns for
// an invalid address local part (e.g. one starting with "--"). Used by the
// validation error test which can't hit the real /core/v4/addresses/setup
// endpoint (needs a verified custom domain).
func inject422ValidationInvalidLocalPart(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":2061,"Error":"Local part is invalid"}`
	return newOneShot(next, target, http.StatusUnprocessableEntity, body, nil)
}

// dateHeader supplies a valid RFC1123-format Date for synthetic responses.
// Some proton.Client code paths (e.g. address enable/disable) parse the Date
// header strictly and fail with `parsing time "": cannot parse ""` when the
// header is missing; setting a fixed historical value keeps cassettes
// deterministic and replay-stable.
var dateHeader = http.Header{"Date": []string{"Mon, 01 Jan 2001 00:00:00 GMT"}}

func withDate(h http.Header) http.Header {
	if h == nil {
		return dateHeader
	}
	for k, v := range dateHeader {
		h[k] = v
	}
	return h
}

// inject200AddressCreated synthesizes the Address envelope Proton returns
// from POST /core/v4/addresses/setup. The ID and Email values are the
// REDACTED placeholders consumer tests assert against.
func inject200AddressCreated(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":1000,"Address":{"ID":"REDACTED_ADDRESSID_1","Email":"record-test@example.test","Send":1,"Receive":1,"Status":1,"Type":4,"Order":1,"DisplayName":"Record Test","Signature":"","Priority":1,"DomainID":"REDACTED_DOMAINID_1","HasKeys":1,"Keys":[]}}`
	return newPersistent(next, target, http.StatusOK, body, withDate(nil))
}

// inject200Ok synthesizes a bare success envelope. Used for endpoints whose
// successful response carries no payload (e.g. address enable/disable/delete).
func inject200Ok(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":1000}`
	return newPersistent(next, target, http.StatusOK, body, withDate(nil))
}

// inject200CustomDomainsList synthesizes GET /core/v4/domains with a single
// non-verified domain. Consumer tests that depend on `len(domains) > 0`
// require at least one entry; recording an empty list would cause the test
// to t.Skip and erode coverage.
func inject200CustomDomainsList(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":1000,"Domains":[{"ID":"REDACTED_DOMAINID_1","DomainName":"example.test","VerifyState":0,"State":1,"Flags":0,"CatchAll":0,"CreateTime":1700000000,"DkimState":0,"Keys":[]}]}`
	return newPersistent(next, target, http.StatusOK, body, withDate(nil))
}

// inject200CustomDomainSingle synthesizes GET/POST/PUT /core/v4/domains/<id>
// returning a single domain envelope.
func inject200CustomDomainSingle(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":1000,"Domain":{"ID":"REDACTED_DOMAINID_1","DomainName":"example.test","VerifyState":0,"State":1,"Flags":0,"CatchAll":0,"CreateTime":1700000000,"DkimState":0,"Keys":[]}}`
	return newPersistent(next, target, http.StatusOK, body, withDate(nil))
}

// inject200AuthRefresh synthesizes the /auth/v4/refresh success response with
// placeholder tokens that survive the scrubber unchanged. Used for scenarios
// where the cassette needs a recorded refresh interaction but recording
// against the real API isn't reliable (e.g. token_rotation, where Proton
// rejects rapid-fire refreshes on the same session with 400 Invalid refresh
// token).
func inject200AuthRefresh(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"AccessToken":"REDACTED_ACCESSTOKEN_1","Code":1000,"ExpiresIn":86400,` +
		`"RefreshToken":"REDACTED_REFRESHTOKEN_1","Scope":"full self parent user loggedin paid nondelinquent mail verified settings",` +
		`"Scopes":["full","self","parent","user","loggedin","paid","nondelinquent","mail","verified","settings"],` +
		`"TokenType":"Bearer","UID":"REDACTED_UID_1","Uid":"REDACTED_UID_1"}`
	return newPersistent(next, target, http.StatusOK, body, withDate(nil))
}

// inject200User synthesizes GET /core/v4/users with a minimal User envelope.
// Only the fields TestTokenRotationOnExpiredAccess and its peers assert on
// (Email = user@example.test) are populated.
func inject200User(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":1000,"User":{"ID":"REDACTED_USERID_1","Name":"user","DisplayName":"user","Email":"user@example.test","Currency":"USD","Credit":0,"MaxSpace":0,"UsedSpace":0,"Subscribed":0,"Services":0,"MnemonicStatus":0,"Role":0,"Private":1,"Delinquent":0,"Keys":[]}}`
	return newPersistent(next, target, http.StatusOK, body, withDate(nil))
}

// inject200DomainAddressesCatchallOn synthesizes
// GET /core/v4/domains/{id}/addresses with one address marked CatchAll=true.
func inject200DomainAddressesCatchallOn(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":1000,"Addresses":[{"ID":"REDACTED_ADDRESSID_1","Email":"catch@example.test","DomainID":"REDACTED_DOMAINID_1","CatchAll":true,"Status":1,"Receive":1,"Send":1,"HasKeys":1,"Type":4,"Order":1,"Priority":1}]}`
	return newPersistent(next, target, http.StatusOK, body, withDate(nil))
}

// inject200DomainAddressesCatchallOff synthesizes
// GET /core/v4/domains/{id}/addresses with one address whose CatchAll=false.
func inject200DomainAddressesCatchallOff(next http.RoundTripper, target string) http.RoundTripper {
	body := `{"Code":1000,"Addresses":[{"ID":"REDACTED_ADDRESSID_1","Email":"plain@example.test","DomainID":"REDACTED_DOMAINID_1","CatchAll":false,"Status":1,"Receive":1,"Send":1,"HasKeys":1,"Type":4,"Order":1,"Priority":1}]}`
	return newPersistent(next, target, http.StatusOK, body, withDate(nil))
}

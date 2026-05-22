package protonraw_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"

	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
	"github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
)

// stubTransport returns a fixed response (or error) for every request.
type stubTransport struct {
	status int
	body   string
	err    error
}

func (s stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: s.status,
		Body:       http.NoBody,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    req,
	}, nil
}

// stubDoer wraps a resty.Client to satisfy protonraw.Doer.
type stubDoer struct{ c *resty.Client }

func (s stubDoer) R() *resty.Request { return s.c.R() }

func newStubDoer(tr http.RoundTripper) stubDoer {
	c := resty.New().SetTransport(tr).SetBaseURL("https://mail.proton.me/api")
	return stubDoer{c: c}
}

// TestProtonrawErrorPathsHTTPError exercises the typed-error branch in
// decode() for every protonraw helper. Each one should surface
// proterr.HTTPError so proterr.Map can route by status.
func TestProtonrawErrorPathsHTTPError(t *testing.T) {
	d := newStubDoer(stubTransport{status: http.StatusForbidden})
	cases := []struct {
		name string
		fn   func() error
	}{
		{"ListCustomDomains", func() error {
			_, err := protonraw.ListCustomDomains(context.Background(), d)
			return err
		}},
		{"GetCustomDomain", func() error {
			_, err := protonraw.GetCustomDomain(context.Background(), d, "REDACTED_DOMAINID_1")
			return err
		}},
		{"AddCustomDomain", func() error {
			_, err := protonraw.AddCustomDomain(context.Background(), d, "example.test")
			return err
		}},
		{"VerifyCustomDomain", func() error {
			_, err := protonraw.VerifyCustomDomain(context.Background(), d, "REDACTED_DOMAINID_1")
			return err
		}},
		{"RemoveCustomDomain", func() error {
			return protonraw.RemoveCustomDomain(context.Background(), d, "REDACTED_DOMAINID_1")
		}},
		{"ListDomainAddresses", func() error {
			_, err := protonraw.ListDomainAddresses(context.Background(), d, "REDACTED_DOMAINID_1")
			return err
		}},
		{"UpdateCatchAll", func() error {
			addr := "REDACTED_ADDRESSID_1"
			return protonraw.UpdateCatchAll(context.Background(), d, "REDACTED_DOMAINID_1", &addr)
		}},
		{"CreateAddress", func() error {
			_, err := protonraw.CreateAddress(context.Background(), d, protonraw.CreateAddressRequest{
				DomainID: "REDACTED_DOMAINID_1", LocalPart: "alias",
			})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var httpErr *proterr.HTTPError
			if !errors.As(err, &httpErr) {
				t.Fatalf("expected *proterr.HTTPError, got %T: %v", err, err)
			}
			if httpErr.Status != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", httpErr.Status)
			}
		})
	}
}

// TestProtonrawValidatesPathID covers the validatePathID guard on every
// helper that takes an ID. Empty + control-character IDs must short-circuit
// before any HTTP traffic.
func TestProtonrawValidatesPathID(t *testing.T) {
	d := newStubDoer(stubTransport{err: errors.New("transport must not be hit")})
	cases := []struct {
		name string
		fn   func(id string) error
	}{
		{"GetCustomDomain", func(id string) error {
			_, err := protonraw.GetCustomDomain(context.Background(), d, id)
			return err
		}},
		{"VerifyCustomDomain", func(id string) error {
			_, err := protonraw.VerifyCustomDomain(context.Background(), d, id)
			return err
		}},
		{"RemoveCustomDomain", func(id string) error {
			return protonraw.RemoveCustomDomain(context.Background(), d, id)
		}},
		{"ListDomainAddresses", func(id string) error {
			_, err := protonraw.ListDomainAddresses(context.Background(), d, id)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/empty", func(t *testing.T) {
			err := tc.fn("")
			if err == nil || !strings.Contains(err.Error(), "required") {
				t.Fatalf("expected 'required' validation, got %v", err)
			}
		})
		t.Run(tc.name+"/slash", func(t *testing.T) {
			err := tc.fn("bad/id")
			if err == nil || !strings.Contains(err.Error(), "invalid") {
				t.Fatalf("expected 'invalid' validation, got %v", err)
			}
		})
	}
}

// TestUpdateCatchAllValidatesDomainID covers the same guard on the catchall
// PUT endpoint, which takes a non-ID body param.
func TestUpdateCatchAllValidatesDomainID(t *testing.T) {
	d := newStubDoer(stubTransport{err: errors.New("transport must not be hit")})
	if err := protonraw.UpdateCatchAll(context.Background(), d, "", nil); err == nil {
		t.Fatal("expected validation error for empty domain_id")
	}
	if err := protonraw.UpdateCatchAll(context.Background(), d, "bad?id", nil); err == nil {
		t.Fatal("expected validation error for bad domain_id")
	}
}

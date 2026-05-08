package protonraw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
)

type fuzzDoer struct{ rc *resty.Client }

func (f *fuzzDoer) R() *resty.Request { return f.rc.R() }

func newFuzzClient(baseURL string) *fuzzDoer {
	return &fuzzDoer{rc: resty.New().SetBaseURL(baseURL).SetHeader("Authorization", "Bearer test")}
}

func FuzzProtonrawDecodeAddress(f *testing.F) {
	f.Add([]byte(`{"Code":1000,"Address":{"ID":"1","Email":"a@b","Send":1}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))
	f.Add([]byte(`null`))

	f.Fuzz(func(t *testing.T, body []byte) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		defer srv.Close()
		c := newFuzzClient(srv.URL)
		_, _ = CreateAddress(context.Background(), c, CreateAddressRequest{
			DomainID: "d", LocalPart: "p",
		})
	})
}

func FuzzProtonrawDecodeDomain(f *testing.F) {
	f.Add([]byte(`{"Code":1000,"Domains":[]}`))
	f.Add([]byte(`{"Code":1000,"Domain":{"ID":"d1","DomainName":"x"}}`))
	f.Add([]byte(`{}`))

	f.Fuzz(func(t *testing.T, body []byte) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		defer srv.Close()
		c := newFuzzClient(srv.URL)
		_, _ = ListCustomDomains(context.Background(), c)
		_, _ = GetCustomDomain(context.Background(), c, "id")
	})
}

func FuzzProtonrawDecodeCatchall(f *testing.F) {
	f.Add([]byte(`{"Code":1000,"Addresses":[]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"Addresses":[{"ID":"a","Email":"a@b","CatchAll":true}]}`))

	f.Fuzz(func(t *testing.T, body []byte) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		defer srv.Close()
		c := newFuzzClient(srv.URL)
		_, _ = ListDomainAddresses(context.Background(), c, "domainID")
	})
}

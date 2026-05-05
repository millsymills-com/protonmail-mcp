package protonraw_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
)

type fakeDoer struct{ rc *resty.Client }

func (f *fakeDoer) R() *resty.Request { return f.rc.R() }

func newFakeDoer(baseURL string) *fakeDoer {
	return &fakeDoer{rc: resty.New().SetBaseURL(baseURL).SetHeader("Authorization", "Bearer test")}
}

func TestListCustomDomains(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/v4/domains" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Code": 1000,
			"Domains": []protonraw.CustomDomain{
				{ID: "d1", DomainName: "example.com", State: 1},
			},
		})
	}))
	defer srv.Close()

	got, err := protonraw.ListCustomDomains(context.Background(), newFakeDoer(srv.URL))
	if err != nil {
		t.Fatalf("ListCustomDomains: %v", err)
	}
	if len(got) != 1 || got[0].DomainName != "example.com" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestGetCustomDomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/v4/domains/d1" || r.Method != http.MethodGet {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Code":   1000,
			"Domain": protonraw.CustomDomain{ID: "d1", DomainName: "example.com", State: 1},
		})
	}))
	defer srv.Close()

	got, err := protonraw.GetCustomDomain(context.Background(), newFakeDoer(srv.URL), "d1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.ID != "d1" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestAddCustomDomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/v4/domains" || r.Method != http.MethodPost {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["Name"] != "example.com" {
			t.Errorf("body Name=%q want example.com", body["Name"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Code":   1000,
			"Domain": protonraw.CustomDomain{ID: "d1", DomainName: "example.com"},
		})
	}))
	defer srv.Close()

	got, err := protonraw.AddCustomDomain(context.Background(), newFakeDoer(srv.URL), "example.com")
	if err != nil {
		t.Fatalf("AddCustomDomain: %v", err)
	}
	if got.ID != "d1" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestVerifyCustomDomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/v4/domains/d1/verify" || r.Method != http.MethodPut {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Code":   1000,
			"Domain": protonraw.CustomDomain{ID: "d1", State: 1, VerifyState: 1},
		})
	}))
	defer srv.Close()

	got, err := protonraw.VerifyCustomDomain(context.Background(), newFakeDoer(srv.URL), "d1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.VerifyState != 1 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestRemoveCustomDomain(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/core/v4/domains/d1" || r.Method != http.MethodDelete {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Code": 1000})
	}))
	defer srv.Close()

	if err := protonraw.RemoveCustomDomain(context.Background(), newFakeDoer(srv.URL), "d1"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !called {
		t.Fatal("server not called")
	}
}

func TestCreateAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/v4/addresses/setup" || r.Method != http.MethodPost {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		var body protonraw.CreateAddressRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.DomainID != "d1" || body.LocalPart != "andy" {
			t.Errorf("unexpected body: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Code":    1000,
			"Address": protonraw.CreatedAddress{ID: "a1", Email: "andy@example.com"},
		})
	}))
	defer srv.Close()

	got, err := protonraw.CreateAddress(context.Background(), newFakeDoer(srv.URL), protonraw.CreateAddressRequest{
		DomainID:  "d1",
		LocalPart: "andy",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Email != "andy@example.com" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestListDomainAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/v4/domains/d1/addresses" || r.Method != http.MethodGet {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Code": 1000,
			"Addresses": []protonraw.DomainAddress{
				{ID: "a1", Email: "andy@example.com", DomainID: "d1", CatchAll: true},
				{ID: "a2", Email: "info@example.com", DomainID: "d1", CatchAll: false},
			},
		})
	}))
	defer srv.Close()

	got, err := protonraw.ListDomainAddresses(context.Background(), newFakeDoer(srv.URL), "d1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 || !got[0].CatchAll || got[1].CatchAll {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestUpdateCatchAllEnable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/v4/domains/d1/catchall" || r.Method != http.MethodPut {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["AddressID"] != "a1" {
			t.Errorf("body AddressID=%v want a1", body["AddressID"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Code": 1000})
	}))
	defer srv.Close()

	id := "a1"
	if err := protonraw.UpdateCatchAll(context.Background(), newFakeDoer(srv.URL), "d1", &id); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestUpdateCatchAllDisable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/v4/domains/d1/catchall" || r.Method != http.MethodPut {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		// Disabling must serialize AddressID as JSON null. After Decode, that
		// surfaces as a present key with nil value.
		v, present := body["AddressID"]
		if !present {
			t.Errorf("AddressID key missing; want explicit null")
		}
		if v != nil {
			t.Errorf("AddressID=%v want nil (json null)", v)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Code": 1000})
	}))
	defer srv.Close()

	if err := protonraw.UpdateCatchAll(context.Background(), newFakeDoer(srv.URL), "d1", nil); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestUpdateCatchAllRejectsBadDomainID(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]any{"Code": 1000})
	}))
	defer srv.Close()

	id := "a1"
	for _, bad := range []string{"", "d1/catchall?x=1", "d1#frag", "d1?foo"} {
		if err := protonraw.UpdateCatchAll(context.Background(), newFakeDoer(srv.URL), bad, &id); err == nil {
			t.Errorf("UpdateCatchAll(%q) want err, got nil", bad)
		}
	}
	if called {
		t.Fatal("server hit despite invalid domain_id; guard failed")
	}
}

func TestCustomDomainCRUDRejectsBadID(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	bad := []string{"", "d1/etc", "d1?x=1", "d1#frag", "../addresses/a1"}
	for _, id := range bad {
		if _, err := protonraw.GetCustomDomain(context.Background(), newFakeDoer(srv.URL), id); err == nil {
			t.Errorf("GetCustomDomain(%q) want err, got nil", id)
		}
		if _, err := protonraw.VerifyCustomDomain(context.Background(), newFakeDoer(srv.URL), id); err == nil {
			t.Errorf("VerifyCustomDomain(%q) want err, got nil", id)
		}
		if err := protonraw.RemoveCustomDomain(context.Background(), newFakeDoer(srv.URL), id); err == nil {
			t.Errorf("RemoveCustomDomain(%q) want err, got nil", id)
		}
	}
	if called {
		t.Fatal("server hit despite invalid id; guard failed")
	}
}

func TestListDomainAddressesRejectsBadDomainID(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	if _, err := protonraw.ListDomainAddresses(context.Background(), newFakeDoer(srv.URL), ""); err == nil {
		t.Fatal("empty domain_id must fail")
	}
	if _, err := protonraw.ListDomainAddresses(context.Background(), newFakeDoer(srv.URL), "d1/etc"); err == nil {
		t.Fatal("slash in domain_id must fail")
	}
	if called {
		t.Fatal("server hit despite invalid domain_id; guard failed")
	}
}

func TestErrorResponseSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Code":  2024,
			"Error": "Domain already in use",
		})
	}))
	defer srv.Close()

	_, err := protonraw.AddCustomDomain(context.Background(), newFakeDoer(srv.URL), "example.com")
	if err == nil {
		t.Fatalf("want error for non-1000 code, got nil")
	}
}

func TestNon1000CodeWithoutErrorIsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Proton's quota/permission errors sometimes ship without a string Error.
		_ = json.NewEncoder(w).Encode(map[string]any{"Code": 2011})
	}))
	defer srv.Close()

	_, err := protonraw.AddCustomDomain(context.Background(), newFakeDoer(srv.URL), "example.com")
	if err == nil {
		t.Fatalf("want error for code 2011, got nil")
	}
}

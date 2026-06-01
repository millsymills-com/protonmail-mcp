package protonraw

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-resty/resty/v2"
)

// validatePathID rejects empty input or any character that would let a
// caller smuggle path/query bytes into a URL segment we build by string
// concatenation. Proton IDs are opaque base64-ish strings and never
// contain these characters in practice.
func validatePathID(name, id string) error {
	if id == "" {
		return fmt.Errorf("%s is required", name)
	}
	if strings.ContainsAny(id, "/?#") {
		return fmt.Errorf("invalid %s %q", name, id)
	}
	return nil
}

// DomainAddress is the per-domain projection of Address; the upstream type is
// `Omit<Address, 'SignedKeyList' | 'Keys'>` per WebClients/Address.ts. We only
// pull the fields callers need for catchall reasoning.
type DomainAddress struct {
	ID       string `json:"ID"`
	Email    string `json:"Email"`
	DomainID string `json:"DomainID"`
	CatchAll bool   `json:"CatchAll"`
	Status   int    `json:"Status"`
	Receive  int    `json:"Receive"`
	Send     int    `json:"Send"`
	HasKeys  int    `json:"HasKeys"`
	Type     int    `json:"Type"`
	Order    int    `json:"Order"`
	Priority int    `json:"Priority"`
}

// ListDomainAddresses -> GET /core/v4/domains/{id}/addresses
// Source: WebClients/packages/shared/lib/api/domains.ts: queryDomainAddresses
func ListDomainAddresses(ctx context.Context, d Doer, domainID string) ([]DomainAddress, error) {
	if err := validatePathID("domain_id", domainID); err != nil {
		return nil, err
	}
	var out struct {
		Addresses []DomainAddress `json:"Addresses"`
	}
	if err := do(ctx, d, "list domain addresses "+domainID, &out, func(r *resty.Request) (*resty.Response, error) {
		return r.Get("/core/v4/domains/" + domainID + "/addresses")
	}); err != nil {
		return nil, err
	}
	return out.Addresses, nil
}

// UpdateCatchAll -> PUT /core/v4/domains/{id}/catchall
// Source: WebClients/packages/shared/lib/api/domains.ts: updateCatchAll
//
// Pass a non-nil addressID to enable catchall for that address; pass nil to
// disable. The Proton API serializes a nil AddressID as JSON null, which is
// how the web client signals "off".
func UpdateCatchAll(ctx context.Context, d Doer, domainID string, addressID *string) error {
	if err := validatePathID("domain_id", domainID); err != nil {
		return err
	}
	body := map[string]any{"AddressID": addressID}
	return do(ctx, d, "update catchall "+domainID, nil, func(r *resty.Request) (*resty.Response, error) {
		return r.SetBody(body).Put("/core/v4/domains/" + domainID + "/catchall")
	})
}

package protonraw

import (
	"context"
	"fmt"

	"github.com/go-resty/resty/v2"
)

// CreateAddressRequest is the shape POSTed to /core/v4/addresses/setup.
// Source: WebClients/packages/shared/lib/api/addresses.ts: setupAddress
type CreateAddressRequest struct {
	DomainID    string `json:"DomainID"`
	LocalPart   string `json:"LocalPart"`
	DisplayName string `json:"DisplayName,omitempty"`
	Signature   string `json:"Signature,omitempty"`
}

// CreatedAddress is the response shape — minimal; callers can re-fetch via
// go-proton-api Client.GetAddress for the full Address struct.
type CreatedAddress struct {
	ID    string `json:"ID"`
	Email string `json:"Email"`
}

// CreateAddress -> POST /core/v4/addresses/setup
func CreateAddress(ctx context.Context, d Doer, req CreateAddressRequest) (CreatedAddress, error) {
	var out struct {
		Address CreatedAddress `json:"Address"`
	}
	label := fmt.Sprintf("create address %s@%s", req.LocalPart, req.DomainID)
	if err := do(ctx, d, label, &out, func(r *resty.Request) (*resty.Response, error) {
		return r.SetBody(req).Post("/core/v4/addresses/setup")
	}); err != nil {
		return CreatedAddress{}, err
	}
	return out.Address, nil
}

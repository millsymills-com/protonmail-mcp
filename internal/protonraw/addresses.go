package protonraw

import (
	"context"
	"fmt"
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
	resp, err := d.R().SetContext(ctx).SetBody(req).Post("/core/v4/addresses/setup")
	if err != nil {
		return CreatedAddress{}, fmt.Errorf("create address %s@%s: %w", req.LocalPart, req.DomainID, err)
	}
	if err := decode(resp, &out); err != nil {
		return CreatedAddress{}, fmt.Errorf("create address %s@%s: %w", req.LocalPart, req.DomainID, err)
	}
	return out.Address, nil
}

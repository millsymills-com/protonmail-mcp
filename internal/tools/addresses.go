package tools

import (
	"context"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
	"github.com/millsymills-com/protonmail-mcp/internal/protonraw"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// addressDTO mirrors the relevant fields of proton.Address. Note the upstream
// type does NOT expose Signature or DomainID at the address level (display
// name and signature are global mail settings, addressed via SetDisplayName /
// SetSignature on Client; the domain is implicit in Email).
type addressDTO struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name,omitempty"`
	Status      int      `json:"status"`
	Order       int      `json:"order"`
	Type        int      `json:"type"`
	Send        bool     `json:"send"`
	Receive     bool     `json:"receive"`
	KeyIDs      []string `json:"key_ids,omitempty"`
}

type listAddressesIn struct{}
type listAddressesOut struct {
	Addresses []addressDTO `json:"addresses"`
}

type getAddressIn struct {
	ID string `json:"id" jsonschema:"the Proton address ID"`
}
type getAddressOut struct {
	Address addressDTO `json:"address"`
}

type createAddressIn struct {
	DomainID    string `json:"domain_id" jsonschema:"the Proton custom domain ID (from proton_list_custom_domains)"`
	LocalPart   string `json:"local_part" jsonschema:"the part of the email before the @"`
	DisplayName string `json:"display_name,omitempty" jsonschema:"optional display name"`
	Signature   string `json:"signature,omitempty" jsonschema:"optional HTML signature"`
}
type createAddressOut struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type updateAddressIn struct {
	ID          string  `json:"id"`
	DisplayName *string `json:"display_name,omitempty"`
	Signature   *string `json:"signature,omitempty"`
}
type updateAddressOut struct {
	OK bool `json:"ok"`
}

type setAddressStatusIn struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled" jsonschema:"true to enable, false to disable"`
}
type setAddressStatusOut struct {
	OK bool `json:"ok"`
}

type deleteAddressIn struct {
	ID string `json:"id"`
}
type deleteAddressOut struct {
	OK bool `json:"ok"`
}

func registerAddresses(server *mcp.Server, d Deps) {
	addTool(server, d, &mcp.Tool{
		Name:        "proton_list_addresses",
		Description: "Lists all addresses on the account, including aliases and disabled ones.",
	}, func(ctx context.Context, d Deps, _ listAddressesIn) (listAddressesOut, *proterr.Error) {
		c, perr := client(ctx, d)
		if perr != nil {
			return listAddressesOut{}, perr
		}
		raw, err := c.GetAddresses(ctx)
		if err != nil {
			return listAddressesOut{}, proterr.Map(err)
		}
		out := make([]addressDTO, len(raw))
		for i, a := range raw {
			out[i] = toAddressDTO(a)
		}
		return listAddressesOut{Addresses: out}, nil
	})

	addTool(server, d, &mcp.Tool{
		Name:        "proton_get_address",
		Description: "Returns detail for a single address by ID.",
	}, func(ctx context.Context, d Deps, in getAddressIn) (getAddressOut, *proterr.Error) {
		c, perr := client(ctx, d)
		if perr != nil {
			return getAddressOut{}, perr
		}
		raw, err := c.GetAddress(ctx, in.ID)
		if err != nil {
			return getAddressOut{}, proterr.Map(err)
		}
		return getAddressOut{Address: toAddressDTO(raw)}, nil
	})

	if !WritesEnabled() {
		return
	}

	addTool(server, d, &mcp.Tool{
		Name:        "proton_create_address",
		Description: "Creates a new address (alias) on a custom domain.",
	}, func(ctx context.Context, d Deps, in createAddressIn) (createAddressOut, *proterr.Error) {
		got, err := protonraw.CreateAddress(ctx, d.Session.Raw(ctx), protonraw.CreateAddressRequest{
			DomainID:    in.DomainID,
			LocalPart:   in.LocalPart,
			DisplayName: in.DisplayName,
			Signature:   in.Signature,
		})
		if err != nil {
			return createAddressOut{}, proterr.Map(err)
		}
		return createAddressOut{ID: got.ID, Email: got.Email}, nil
	})

	addTool(server, d, &mcp.Tool{
		Name:        "proton_update_address",
		Description: "Updates display name and/or signature for the account. Note: SetDisplayName/SetSignature in go-proton-api are global mail settings, not per-address; the ID parameter is accepted for forward compatibility.",
	}, func(ctx context.Context, d Deps, in updateAddressIn) (updateAddressOut, *proterr.Error) {
		c, perr := client(ctx, d)
		if perr != nil {
			return updateAddressOut{}, perr
		}
		if in.DisplayName != nil {
			if _, err := c.SetDisplayName(ctx, proton.SetDisplayNameReq{DisplayName: *in.DisplayName}); err != nil {
				return updateAddressOut{}, proterr.Map(err)
			}
		}
		if in.Signature != nil {
			if _, err := c.SetSignature(ctx, proton.SetSignatureReq{Signature: *in.Signature}); err != nil {
				return updateAddressOut{}, proterr.Map(err)
			}
		}
		return updateAddressOut{OK: true}, nil
	})

	addTool(server, d, &mcp.Tool{
		Name:        "proton_set_address_status",
		Description: "Enables or disables an address.",
	}, func(ctx context.Context, d Deps, in setAddressStatusIn) (setAddressStatusOut, *proterr.Error) {
		c, perr := client(ctx, d)
		if perr != nil {
			return setAddressStatusOut{}, perr
		}
		var err error
		if in.Enabled {
			err = c.EnableAddress(ctx, in.ID)
		} else {
			err = c.DisableAddress(ctx, in.ID)
		}
		if err != nil {
			return setAddressStatusOut{}, proterr.Map(err)
		}
		return setAddressStatusOut{OK: true}, nil
	})

	addTool(server, d, &mcp.Tool{
		Name:        "proton_delete_address",
		Description: "Permanently deletes an address. DESTRUCTIVE — cannot be undone.",
	}, func(ctx context.Context, d Deps, in deleteAddressIn) (deleteAddressOut, *proterr.Error) {
		c, perr := client(ctx, d)
		if perr != nil {
			return deleteAddressOut{}, perr
		}
		if err := c.DeleteAddress(ctx, in.ID); err != nil {
			return deleteAddressOut{}, proterr.Map(err)
		}
		return deleteAddressOut{OK: true}, nil
	})
}

func toAddressDTO(a proton.Address) addressDTO {
	keyIDs := make([]string, len(a.Keys))
	for i, k := range a.Keys {
		keyIDs[i] = k.ID
	}
	return addressDTO{
		ID:          a.ID,
		Email:       a.Email,
		DisplayName: a.DisplayName,
		Status:      int(a.Status),
		Order:       a.Order,
		Type:        int(a.Type),
		Send:        bool(a.Send),
		Receive:     bool(a.Receive),
		KeyIDs:      keyIDs,
	}
}

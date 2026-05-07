package tools

import (
	"context"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
	"github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_list_addresses",
		Description: "Lists all addresses on the account, including aliases and disabled ones.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listAddressesIn) (*mcp.CallToolResult, listAddressesOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, listAddressesOut{}, nil
		}
		raw, err := c.GetAddresses(ctx)
		if err != nil {
			return failure(proterr.Map(err)), listAddressesOut{}, nil
		}
		out := make([]addressDTO, len(raw))
		for i, a := range raw {
			out[i] = toAddressDTO(a)
		}
		return nil, listAddressesOut{Addresses: out}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_get_address",
		Description: "Returns detail for a single address by ID.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getAddressIn) (*mcp.CallToolResult, getAddressOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, getAddressOut{}, nil
		}
		raw, err := c.GetAddress(ctx, in.ID)
		if err != nil {
			return failure(proterr.Map(err)), getAddressOut{}, nil
		}
		return nil, getAddressOut{Address: toAddressDTO(raw)}, nil
	})

	if !WritesEnabled() {
		return
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_create_address",
		Description: "Creates a new address (alias) on a custom domain.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in createAddressIn) (*mcp.CallToolResult, createAddressOut, error) {
		raw := d.Session.Raw(ctx)
		got, err := protonraw.CreateAddress(ctx, raw, protonraw.CreateAddressRequest{
			DomainID:    in.DomainID,
			LocalPart:   in.LocalPart,
			DisplayName: in.DisplayName,
			Signature:   in.Signature,
		})
		if err != nil {
			return failure(proterr.Map(err)), createAddressOut{}, nil
		}
		return nil, createAddressOut{ID: got.ID, Email: got.Email}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_update_address",
		Description: "Updates display name and/or signature for the account. Note: SetDisplayName/SetSignature in go-proton-api are global mail settings, not per-address; the ID parameter is accepted for forward compatibility.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in updateAddressIn) (*mcp.CallToolResult, updateAddressOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, updateAddressOut{}, nil
		}
		if in.DisplayName != nil {
			if _, err := c.SetDisplayName(ctx, proton.SetDisplayNameReq{DisplayName: *in.DisplayName}); err != nil {
				return failure(proterr.Map(err)), updateAddressOut{}, nil
			}
		}
		if in.Signature != nil {
			if _, err := c.SetSignature(ctx, proton.SetSignatureReq{Signature: *in.Signature}); err != nil {
				return failure(proterr.Map(err)), updateAddressOut{}, nil
			}
		}
		return nil, updateAddressOut{OK: true}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_set_address_status",
		Description: "Enables or disables an address.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setAddressStatusIn) (*mcp.CallToolResult, setAddressStatusOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, setAddressStatusOut{}, nil
		}
		var err error
		if in.Enabled {
			err = c.EnableAddress(ctx, in.ID)
		} else {
			err = c.DisableAddress(ctx, in.ID)
		}
		if err != nil {
			return failure(proterr.Map(err)), setAddressStatusOut{}, nil
		}
		return nil, setAddressStatusOut{OK: true}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_delete_address",
		Description: "Permanently deletes an address. DESTRUCTIVE — cannot be undone.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deleteAddressIn) (*mcp.CallToolResult, deleteAddressOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, deleteAddressOut{}, nil
		}
		if err := c.DeleteAddress(ctx, in.ID); err != nil {
			return failure(proterr.Map(err)), deleteAddressOut{}, nil
		}
		return nil, deleteAddressOut{OK: true}, nil
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

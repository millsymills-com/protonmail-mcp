package tools

import (
	"context"
	"fmt"

	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
	"github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type domainDTO struct {
	ID          string         `json:"id"`
	DomainName  string         `json:"domain_name"`
	State       int            `json:"state"`
	VerifyState int            `json:"verify_state"`
	MxState     int            `json:"mx_state"`
	SpfState    int            `json:"spf_state"`
	DkimState   int            `json:"dkim_state"`
	DmarcState  int            `json:"dmarc_state"`
	Records     []dnsRecordDTO `json:"required_dns_records,omitempty"`
}

type dnsRecordDTO struct {
	Type     string `json:"type" jsonschema:"DNS record type: TXT, MX, or CNAME"`
	Hostname string `json:"hostname" jsonschema:"the hostname to publish, e.g. @ or mail._domainkey"`
	Value    string `json:"value" jsonschema:"the record value"`
	Purpose  string `json:"purpose" jsonschema:"verify | mx | spf | dkim | dmarc"`
}

type listDomainsIn struct{}
type listDomainsOut struct {
	Domains []domainDTO `json:"domains"`
}

type getDomainIn struct {
	ID string `json:"id"`
}
type getDomainOut struct {
	Domain domainDTO `json:"domain"`
}

type addDomainIn struct {
	DomainName string `json:"domain_name"`
}
type addDomainOut struct {
	Domain domainDTO `json:"domain"`
}

type verifyDomainIn struct {
	ID string `json:"id"`
}
type verifyDomainOut struct {
	Domain domainDTO `json:"domain"`
}

type removeDomainIn struct {
	ID string `json:"id"`
}
type removeDomainOut struct {
	OK bool `json:"ok"`
}

type getCatchallIn struct {
	DomainID string `json:"domain_id" jsonschema:"the Proton custom domain ID (from proton_list_custom_domains)"`
}
type getCatchallOut struct {
	DomainID             string  `json:"domain_id"`
	Enabled              bool    `json:"enabled"`
	DestinationAddressID *string `json:"destination_address_id,omitempty"`
	DestinationEmail     *string `json:"destination_email,omitempty"`
}

type setCatchallIn struct {
	DomainID             string `json:"domain_id" jsonschema:"the Proton custom domain ID"`
	DestinationAddressID string `json:"destination_address_id" jsonschema:"address ID on the same domain that should receive catchall mail"`
}
type setCatchallOut struct {
	OK bool `json:"ok"`
}

type disableCatchallIn struct {
	DomainID string `json:"domain_id" jsonschema:"the Proton custom domain ID"`
}
type disableCatchallOut struct {
	OK bool `json:"ok"`
}

func registerDomains(server *mcp.Server, d Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_list_custom_domains",
		Description: "Lists all custom domains on the account with verification state.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listDomainsIn) (*mcp.CallToolResult, listDomainsOut, error) {
		raws, err := protonraw.ListCustomDomains(ctx, d.Session.Raw(ctx))
		if err != nil {
			return failure(proterr.Map(err)), listDomainsOut{}, nil
		}
		out := make([]domainDTO, len(raws))
		for i, r := range raws {
			out[i] = toDomainDTO(r)
		}
		return nil, listDomainsOut{Domains: out}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_get_custom_domain",
		Description: "Returns detail (including required DNS records) for a custom domain.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getDomainIn) (*mcp.CallToolResult, getDomainOut, error) {
		raw, err := protonraw.GetCustomDomain(ctx, d.Session.Raw(ctx), in.ID)
		if err != nil {
			return failure(proterr.Map(err)), getDomainOut{}, nil
		}
		return nil, getDomainOut{Domain: toDomainDTO(raw)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_get_catchall",
		Description: "Reports whether catchall is enabled on a custom domain and, if so, which address receives unmatched mail.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getCatchallIn) (*mcp.CallToolResult, getCatchallOut, error) {
		if fail := requireField("domain_id", in.DomainID); fail != nil {
			return fail, getCatchallOut{}, nil
		}
		addrs, err := protonraw.ListDomainAddresses(ctx, d.Session.Raw(ctx), in.DomainID)
		if err != nil {
			return failure(proterr.Map(err)), getCatchallOut{}, nil
		}
		out := getCatchallOut{DomainID: in.DomainID}
		for _, a := range addrs {
			if a.CatchAll {
				id, email := a.ID, a.Email
				out.Enabled = true
				out.DestinationAddressID = &id
				out.DestinationEmail = &email
				break
			}
		}
		return nil, out, nil
	})

	if !WritesEnabled() {
		return
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_add_custom_domain",
		Description: "Adds a new custom domain. Returns the required DNS records to publish at your DNS provider.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in addDomainIn) (*mcp.CallToolResult, addDomainOut, error) {
		raw, err := protonraw.AddCustomDomain(ctx, d.Session.Raw(ctx), in.DomainName)
		if err != nil {
			return failure(proterr.Map(err)), addDomainOut{}, nil
		}
		return nil, addDomainOut{Domain: toDomainDTO(raw)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_verify_custom_domain",
		Description: "Asks Proton to re-verify the published DNS records for a custom domain.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in verifyDomainIn) (*mcp.CallToolResult, verifyDomainOut, error) {
		raw, err := protonraw.VerifyCustomDomain(ctx, d.Session.Raw(ctx), in.ID)
		if err != nil {
			return failure(proterr.Map(err)), verifyDomainOut{}, nil
		}
		return nil, verifyDomainOut{Domain: toDomainDTO(raw)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_remove_custom_domain",
		Description: "Removes a custom domain. DESTRUCTIVE — orphans all aliases on the domain.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in removeDomainIn) (*mcp.CallToolResult, removeDomainOut, error) {
		if err := protonraw.RemoveCustomDomain(ctx, d.Session.Raw(ctx), in.ID); err != nil {
			return failure(proterr.Map(err)), removeDomainOut{}, nil
		}
		return nil, removeDomainOut{OK: true}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_set_catchall",
		Description: "Enables catchall on a custom domain and routes unmatched mail to the given address. The address must already exist on that domain.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setCatchallIn) (*mcp.CallToolResult, setCatchallOut, error) {
		if fail := requireField("domain_id", in.DomainID); fail != nil {
			return fail, setCatchallOut{}, nil
		}
		if fail := requireField("destination_address_id", in.DestinationAddressID); fail != nil {
			return fail, setCatchallOut{}, nil
		}
		raw := d.Session.Raw(ctx)
		addrs, err := protonraw.ListDomainAddresses(ctx, raw, in.DomainID)
		if err != nil {
			return failure(proterr.Map(err)), setCatchallOut{}, nil
		}
		found := false
		for _, a := range addrs {
			if a.ID == in.DestinationAddressID {
				found = true
				break
			}
		}
		if !found {
			return failure(&proterr.Error{
				Code:    "proton/validation",
				Message: fmt.Sprintf("address %s does not belong to domain %s", in.DestinationAddressID, in.DomainID),
				Hint:    "call proton_list_addresses or proton_list_custom_domains to find the right address ID",
			}), setCatchallOut{}, nil
		}
		if err := protonraw.UpdateCatchAll(ctx, raw, in.DomainID, &in.DestinationAddressID); err != nil {
			return failure(proterr.Map(err)), setCatchallOut{}, nil
		}
		return nil, setCatchallOut{OK: true}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_disable_catchall",
		Description: "Disables catchall on a custom domain. Mail to unknown local-parts will bounce.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in disableCatchallIn) (*mcp.CallToolResult, disableCatchallOut, error) {
		if fail := requireField("domain_id", in.DomainID); fail != nil {
			return fail, disableCatchallOut{}, nil
		}
		if err := protonraw.UpdateCatchAll(ctx, d.Session.Raw(ctx), in.DomainID, nil); err != nil {
			return failure(proterr.Map(err)), disableCatchallOut{}, nil
		}
		return nil, disableCatchallOut{OK: true}, nil
	})
}

func toDomainDTO(c protonraw.CustomDomain) domainDTO {
	recs := make([]dnsRecordDTO, len(c.Records))
	for i, r := range c.Records {
		recs[i] = dnsRecordDTO{Type: r.Type, Hostname: r.Name, Value: r.Value, Purpose: r.Purpose}
	}
	return domainDTO{
		ID:          c.ID,
		DomainName:  c.DomainName,
		State:       c.State,
		VerifyState: c.VerifyState,
		MxState:     c.MxState,
		SpfState:    c.SpfState,
		DkimState:   c.DkimState,
		DmarcState:  c.DmarcState,
		Records:     recs,
	}
}

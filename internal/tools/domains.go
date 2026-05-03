package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
	"github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
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

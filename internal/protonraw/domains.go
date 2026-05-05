package protonraw

import "context"

// CustomDomain mirrors the Proton API shape for a user-managed custom domain.
// Source: WebClients/packages/shared/lib/interfaces/Domain.ts.
type CustomDomain struct {
	ID          string               `json:"ID"`
	DomainName  string               `json:"DomainName"`
	State       int                  `json:"State"`
	VerifyState int                  `json:"VerifyState"`
	MxState     int                  `json:"MxState"`
	SpfState    int                  `json:"SpfState"`
	DkimState   int                  `json:"DkimState"`
	DmarcState  int                  `json:"DmarcState"`
	Records     []CustomDomainRecord `json:"VerificationRecords,omitempty"`
}

// CustomDomainRecord is one DNS record the user must publish.
type CustomDomainRecord struct {
	Type    string `json:"Type"`     // "TXT", "MX", "CNAME"
	Name    string `json:"Hostname"` // "@", "mail._domainkey", etc.
	Value   string `json:"Value"`
	Purpose string `json:"Purpose"` // "verify", "mx", "spf", "dkim", "dmarc"
}

// ListCustomDomains -> GET /core/v4/domains
// Source: WebClients/packages/shared/lib/api/domains.ts: queryDomains
func ListCustomDomains(ctx context.Context, d Doer) ([]CustomDomain, error) {
	var out struct {
		Domains []CustomDomain `json:"Domains"`
	}
	resp, err := d.R().SetContext(ctx).Get("/core/v4/domains")
	if err != nil {
		return nil, err
	}
	if err := decode(resp, &out); err != nil {
		return nil, err
	}
	return out.Domains, nil
}

// GetCustomDomain -> GET /core/v4/domains/{id}
// Source: WebClients/packages/shared/lib/api/domains.ts: getDomain
func GetCustomDomain(ctx context.Context, d Doer, id string) (CustomDomain, error) {
	if err := validatePathID("id", id); err != nil {
		return CustomDomain{}, err
	}
	var out struct {
		Domain CustomDomain `json:"Domain"`
	}
	resp, err := d.R().SetContext(ctx).Get("/core/v4/domains/" + id)
	if err != nil {
		return CustomDomain{}, err
	}
	if err := decode(resp, &out); err != nil {
		return CustomDomain{}, err
	}
	return out.Domain, nil
}

// AddCustomDomain -> POST /core/v4/domains
// Source: WebClients/packages/shared/lib/api/domains.ts: addDomain
func AddCustomDomain(ctx context.Context, d Doer, domain string) (CustomDomain, error) {
	body := map[string]string{"Name": domain}
	var out struct {
		Domain CustomDomain `json:"Domain"`
	}
	resp, err := d.R().SetContext(ctx).SetBody(body).Post("/core/v4/domains")
	if err != nil {
		return CustomDomain{}, err
	}
	if err := decode(resp, &out); err != nil {
		return CustomDomain{}, err
	}
	return out.Domain, nil
}

// VerifyCustomDomain -> PUT /core/v4/domains/{id}/verify
// Source: WebClients/packages/shared/lib/api/domains.ts: verifyDomain
func VerifyCustomDomain(ctx context.Context, d Doer, id string) (CustomDomain, error) {
	if err := validatePathID("id", id); err != nil {
		return CustomDomain{}, err
	}
	var out struct {
		Domain CustomDomain `json:"Domain"`
	}
	resp, err := d.R().SetContext(ctx).Put("/core/v4/domains/" + id + "/verify")
	if err != nil {
		return CustomDomain{}, err
	}
	if err := decode(resp, &out); err != nil {
		return CustomDomain{}, err
	}
	return out.Domain, nil
}

// RemoveCustomDomain -> DELETE /core/v4/domains/{id}
// Source: WebClients/packages/shared/lib/api/domains.ts: deleteDomain
func RemoveCustomDomain(ctx context.Context, d Doer, id string) error {
	if err := validatePathID("id", id); err != nil {
		return err
	}
	resp, err := d.R().SetContext(ctx).Delete("/core/v4/domains/" + id)
	if err != nil {
		return err
	}
	return decode(resp, nil)
}

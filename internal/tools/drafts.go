package tools

import (
	"context"
	"net/mail"
	"strconv"

	"github.com/ProtonMail/gluon/rfc822"
	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// parseRecipients parses RFC 5322 address strings ("a@x" or "Name <a@x>"). An
// empty input yields a nil slice; a malformed entry is a validation error.
func parseRecipients(in []string) ([]*mail.Address, *proterr.Error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]*mail.Address, 0, len(in))
	for _, s := range in {
		a, err := mail.ParseAddress(s)
		if err != nil {
			return nil, &proterr.Error{Code: "proton/validation", Message: "invalid email address: " + quoteTrunc(s)}
		}
		out = append(out, a)
	}
	return out, nil
}

// resolveMIMEType maps the optional mime_type input to an rfc822 type,
// defaulting to text/plain and rejecting anything other than the two supported
// body formats.
func resolveMIMEType(s string) (rfc822.MIMEType, *proterr.Error) {
	switch s {
	case "", "text/plain":
		return rfc822.TextPlain, nil
	case "text/html":
		return rfc822.TextHTML, nil
	default:
		return "", &proterr.Error{Code: "proton/validation", Message: "mime_type must be text/plain or text/html"}
	}
}

// quoteTrunc renders caller input safely for an error message: quoted (so
// control characters can't leak into MCP text content) and capped so an
// oversized input can't bloat the error.
func quoteTrunc(s string) string {
	if len(s) > 100 {
		s = s[:100] + "…"
	}
	return strconv.Quote(s)
}

// resolveSender returns the address to send from: the one named by addressID,
// or the account's primary sending address (enabled, send-allowed, lowest
// Order) when addressID is empty.
func resolveSender(ctx context.Context, c *proton.Client, addressID string) (proton.Address, *proterr.Error) {
	if addressID != "" {
		a, err := c.GetAddress(ctx, addressID)
		if err != nil {
			return proton.Address{}, proterr.Map(err)
		}
		if a.Status != proton.AddressStatusEnabled || !bool(a.Send) {
			return proton.Address{}, &proterr.Error{Code: "proton/validation", Message: "address " + quoteTrunc(addressID) + " cannot send (disabled or receive-only)"}
		}
		return a, nil
	}
	addrs, err := c.GetAddresses(ctx)
	if err != nil {
		return proton.Address{}, proterr.Map(err)
	}
	var best *proton.Address
	for i := range addrs {
		a := &addrs[i]
		if a.Status != proton.AddressStatusEnabled || !bool(a.Send) {
			continue
		}
		if best == nil || a.Order < best.Order {
			best = a
		}
	}
	if best == nil {
		return proton.Address{}, &proterr.Error{Code: "proton/validation", Message: "no enabled sending address found"}
	}
	return *best, nil
}

type createDraftIn struct {
	FromAddressID string   `json:"from_address_id,omitempty" jsonschema:"sender address ID; defaults to the primary sending address"`
	To            []string `json:"to,omitempty" jsonschema:"recipient email addresses"`
	CC            []string `json:"cc,omitempty"`
	BCC           []string `json:"bcc,omitempty"`
	Subject       string   `json:"subject,omitempty"`
	Body          string   `json:"body,omitempty"`
	MIMEType      string   `json:"mime_type,omitempty" jsonschema:"text/plain (default) or text/html"`
}

type draftOut struct {
	Message messageStubDTO `json:"message"`
}

type updateDraftIn struct {
	ID            string   `json:"id"`
	FromAddressID string   `json:"from_address_id,omitempty" jsonschema:"sender address ID; defaults to the primary sending address"`
	To            []string `json:"to,omitempty" jsonschema:"recipient email addresses"`
	CC            []string `json:"cc,omitempty"`
	BCC           []string `json:"bcc,omitempty"`
	Subject       string   `json:"subject,omitempty"`
	Body          string   `json:"body,omitempty"`
	MIMEType      string   `json:"mime_type,omitempty" jsonschema:"text/plain (default) or text/html"`
}

// draftTemplate validates and parses the pure inputs; Sender is left nil so
// handlers can run this before any network call and attach the resolved
// sender afterwards.
func draftTemplate(to, cc, bcc []string, subject, body, mimeType string) (proton.DraftTemplate, *proterr.Error) {
	toL, perr := parseRecipients(to)
	if perr != nil {
		return proton.DraftTemplate{}, perr
	}
	ccL, perr := parseRecipients(cc)
	if perr != nil {
		return proton.DraftTemplate{}, perr
	}
	bccL, perr := parseRecipients(bcc)
	if perr != nil {
		return proton.DraftTemplate{}, perr
	}
	mt, perr := resolveMIMEType(mimeType)
	if perr != nil {
		return proton.DraftTemplate{}, perr
	}
	return proton.DraftTemplate{
		Subject:  subject,
		ToList:   toL,
		CCList:   ccL,
		BCCList:  bccL,
		Body:     body,
		MIMEType: mt,
	}, nil
}

func senderKeyRing(ctx context.Context, d Deps, addrID string) (*crypto.KeyRing, *proterr.Error) {
	krs, err := d.Session.Keyrings(ctx)
	if err != nil {
		return nil, proterr.Map(err)
	}
	kr, err := krs.AddressKeyRing(addrID)
	if err != nil {
		return nil, proterr.Map(err)
	}
	return kr, nil
}

func registerDrafts(server *mcp.Server, d Deps) {
	if !WritesEnabled() {
		return
	}
	addTool(server, d, &mcp.Tool{
		Name:        "proton_create_draft",
		Description: "Creates a draft message encrypted to the sender's own key. Does NOT send. from_address_id defaults to the primary sending address. Returns the created draft's metadata (use the returned id with proton_update_draft).",
	}, createDraft)
	addTool(server, d, &mcp.Tool{
		Name:        "proton_update_draft",
		Description: "Replaces an existing draft's content by ID: subject, recipients, and body are overwritten with the provided values, and omitted fields are cleared (this is a full replace, not a merge — re-supply every field you want kept). The body is re-encrypted to the sender key; from_address_id defaults to the primary sending address, which may differ from the draft's original sender. Does NOT send.",
	}, updateDraft)
}

func createDraft(ctx context.Context, d Deps, in createDraftIn) (draftOut, *proterr.Error) {
	tmpl, perr := draftTemplate(in.To, in.CC, in.BCC, in.Subject, in.Body, in.MIMEType)
	if perr != nil {
		return draftOut{}, perr
	}
	c, perr := client(ctx, d)
	if perr != nil {
		return draftOut{}, perr
	}
	sender, perr := resolveSender(ctx, c, in.FromAddressID)
	if perr != nil {
		return draftOut{}, perr
	}
	tmpl.Sender = &mail.Address{Name: sender.DisplayName, Address: sender.Email}
	kr, perr := senderKeyRing(ctx, d, sender.ID)
	if perr != nil {
		return draftOut{}, perr
	}
	msg, err := c.CreateDraft(ctx, kr, proton.CreateDraftReq{Message: tmpl})
	if err != nil {
		return draftOut{}, proterr.Map(err)
	}
	return draftOut{Message: toMessageStubDTO(msg.MessageMetadata)}, nil
}

func updateDraft(ctx context.Context, d Deps, in updateDraftIn) (draftOut, *proterr.Error) {
	if perr := required("id", in.ID); perr != nil {
		return draftOut{}, perr
	}
	tmpl, perr := draftTemplate(in.To, in.CC, in.BCC, in.Subject, in.Body, in.MIMEType)
	if perr != nil {
		return draftOut{}, perr
	}
	c, perr := client(ctx, d)
	if perr != nil {
		return draftOut{}, perr
	}
	sender, perr := resolveSender(ctx, c, in.FromAddressID)
	if perr != nil {
		return draftOut{}, perr
	}
	tmpl.Sender = &mail.Address{Name: sender.DisplayName, Address: sender.Email}
	kr, perr := senderKeyRing(ctx, d, sender.ID)
	if perr != nil {
		return draftOut{}, perr
	}
	msg, err := c.UpdateDraft(ctx, in.ID, kr, proton.UpdateDraftReq{Message: tmpl})
	if err != nil {
		return draftOut{}, proterr.Map(err)
	}
	return draftOut{Message: toMessageStubDTO(msg.MessageMetadata)}, nil
}

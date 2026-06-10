package tools

import (
	"context"
	"net/mail"
	"strconv"

	"github.com/ProtonMail/gluon/rfc822"
	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
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
			return proton.Address{}, &proterr.Error{Code: "proton/validation", Message: "address " + addressID + " cannot send (disabled or receive-only)"}
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

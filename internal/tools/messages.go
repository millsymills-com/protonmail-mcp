package tools

import (
	"context"
	"net/mail"
	"strings"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sensitiveHeaders are removed from parsed_headers before returning. Bcc
// reveals recipients hidden by the original sender; X-Originating-IP and
// related vendor extensions reveal sender network metadata that the
// standard Received chain already filters at MTA boundaries. Returning
// these would expose data the recipient never had access to in a
// conventional mail UI. Raw headers are still returned verbatim — callers
// asking for raw_headers have explicitly opted into the full block.
var sensitiveHeaders = map[string]struct{}{
	"bcc":                  {},
	"x-originating-ip":     {},
	"x-original-sender-ip": {},
	"x-original-sender":    {},
	"x-real-ip":            {},
}

// messageStubDTO is the search-result projection. Fields chosen for the
// canonical "did delivery happen?" check: subject + sender + recipients +
// timestamp + flags. Body is intentionally absent — bodies are PGP-encrypted
// and decrypting them needs an unlocked keyring (v1.5).
type messageStubDTO struct {
	ID            string   `json:"id"`
	Subject       string   `json:"subject"`
	From          string   `json:"from,omitempty"`
	To            []string `json:"to,omitempty"`
	CC            []string `json:"cc,omitempty"`
	InternalDate  int64    `json:"internal_date"`
	Unread        bool     `json:"unread"`
	HasAttachment bool     `json:"has_attachment"`
	AddressID     string   `json:"address_id,omitempty"`
	LabelIDs      []string `json:"label_ids,omitempty"`
}

type searchMessagesIn struct {
	Query     string `json:"query,omitempty" jsonschema:"substring match against the Subject (server-side)"`
	LabelID   string `json:"label_id,omitempty" jsonschema:"restrict to a specific label/folder ID"`
	AddressID string `json:"address_id,omitempty" jsonschema:"restrict to messages received at this address"`
	Limit     int    `json:"limit,omitempty" jsonschema:"page size, 1..150 (default 50)"`
	Page      int    `json:"page,omitempty" jsonschema:"0-indexed page (default 0)"`
}
type searchMessagesOut struct {
	Messages []messageStubDTO `json:"messages"`
}

type getMessageIn struct {
	ID             string `json:"id"`
	IncludeHeaders bool   `json:"include_headers,omitempty" jsonschema:"if true, return the full raw RFC822 header block + parsed headers (e.g. Authentication-Results)"`
}
type getMessageOut struct {
	Message       messageStubDTO      `json:"message"`
	RawHeaders    string              `json:"raw_headers,omitempty"`
	ParsedHeaders map[string][]string `json:"parsed_headers,omitempty"`
}

func registerMessages(server *mcp.Server, d Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_search_messages",
		Description: "Searches recent messages by subject substring, label/folder, or recipient address. Returns metadata only (no body). Use proton_get_message with include_headers=true to inspect Authentication-Results for delivery verification.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchMessagesIn) (*mcp.CallToolResult, searchMessagesOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, searchMessagesOut{}, nil
		}
		page := in.Page
		if page < 0 {
			page = 0
		}
		size := in.Limit
		switch {
		case size <= 0:
			size = 50
		case size > 150:
			size = 150
		}
		filter := proton.MessageFilter{
			Subject:   in.Query,
			LabelID:   in.LabelID,
			AddressID: in.AddressID,
			Desc:      proton.Bool(true),
		}
		raws, err := c.GetMessageMetadataPage(ctx, page, size, filter)
		if err != nil {
			return failure(proterr.Map(err)), searchMessagesOut{}, nil
		}
		out := make([]messageStubDTO, len(raws))
		for i, m := range raws {
			out[i] = toMessageStubDTO(m)
		}
		return nil, searchMessagesOut{Messages: out}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_get_message",
		Description: "Returns a single message's metadata. With include_headers=true, also returns the raw RFC822 header block and a parsed-header map (e.g. Authentication-Results, DKIM-Signature, Received) for delivery verification. Sensitive headers (Bcc, X-Originating-IP, etc.) are stripped from parsed_headers, but raw_headers is the complete block — treat raw_headers as containing the BCC list and origination IP and handle accordingly. Body is not returned — PGP decryption requires an unlocked keyring (v1.5).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getMessageIn) (*mcp.CallToolResult, getMessageOut, error) {
		if fail := requireField("id", in.ID); fail != nil {
			return fail, getMessageOut{}, nil
		}
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, getMessageOut{}, nil
		}
		raw, err := c.GetMessage(ctx, in.ID)
		if err != nil {
			return failure(proterr.Map(err)), getMessageOut{}, nil
		}
		out := getMessageOut{Message: toMessageStubDTO(raw.MessageMetadata)}
		if in.IncludeHeaders {
			out.RawHeaders = raw.Header
			out.ParsedHeaders = filterSensitiveHeaders(raw.ParsedHeaders.Values)
		}
		return nil, out, nil
	})
}

func toMessageStubDTO(m proton.MessageMetadata) messageStubDTO {
	return messageStubDTO{
		ID:            m.ID,
		Subject:       m.Subject,
		From:          formatAddress(m.Sender),
		To:            formatAddresses(m.ToList),
		CC:            formatAddresses(m.CCList),
		InternalDate:  m.Time,
		Unread:        bool(m.Unread),
		HasAttachment: m.NumAttachments > 0,
		AddressID:     m.AddressID,
		LabelIDs:      m.LabelIDs,
	}
}

func formatAddress(a *mail.Address) string {
	if a == nil {
		return ""
	}
	if a.Name != "" {
		return a.Name + " <" + a.Address + ">"
	}
	return a.Address
}

func formatAddresses(as []*mail.Address) []string {
	if len(as) == 0 {
		return nil
	}
	out := make([]string, 0, len(as))
	for _, a := range as {
		if s := formatAddress(a); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func filterSensitiveHeaders(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for k, v := range in {
		if _, drop := sensitiveHeaders[strings.ToLower(k)]; drop {
			continue
		}
		out[k] = v
	}
	return out
}

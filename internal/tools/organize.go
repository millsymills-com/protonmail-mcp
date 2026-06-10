package tools

import (
	"context"

	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func boolPtr(b bool) *bool { return &b }

type organizeOut struct {
	OK         bool     `json:"ok"`
	MessageIDs []string `json:"message_ids"`
}

func validateMessageIDs(ids []string) *proterr.Error {
	if len(ids) == 0 {
		return &proterr.Error{Code: "proton/validation", Message: "message_ids must contain at least one ID"}
	}
	return nil
}

func validateLabelAction(action string) *proterr.Error {
	if action != "add" && action != "remove" {
		return &proterr.Error{Code: "proton/validation", Message: `action must be "add" or "remove", got ` + quoteTrunc(action)}
	}
	return nil
}

type labelMessagesIn struct {
	MessageIDs []string `json:"message_ids"`
	LabelID    string   `json:"label_id"`
	Action     string   `json:"action" jsonschema:"add or remove"`
}

type markMessagesIn struct {
	MessageIDs []string `json:"message_ids"`
	Read       bool     `json:"read" jsonschema:"true marks read, false marks unread"`
}

func registerOrganize(server *mcp.Server, d Deps) {
	if !WritesEnabled() {
		return
	}
	addTool(server, d, &mcp.Tool{
		Name:        "proton_label_messages",
		Description: "Adds or removes a label on one or more messages. Moving to Trash is action=add with label_id \"3\" (see proton_list_labels). Reversible. Not atomic across many IDs: requests are chunked (150/batch) and a mid-batch failure leaves earlier batches applied.",
	}, labelMessages)
	addTool(server, d, &mcp.Tool{
		Name:        "proton_mark_messages",
		Description: "Marks one or more messages read (read=true) or unread (read=false). Reversible. Not atomic across many IDs: requests are chunked (150/batch) and a mid-batch failure leaves earlier batches applied.",
	}, markMessages)
	if !DangerousEnabled() {
		return
	}
	addTool(server, d, &mcp.Tool{
		Name:        "proton_delete_messages",
		Description: "PERMANENTLY deletes one or more messages (irreversible expunge — NOT trash). Requires PROTONMAIL_MCP_ENABLE_DANGEROUS in addition to ENABLE_WRITES. To trash recoverably, use proton_label_messages with label_id \"3\" instead. Not atomic across many IDs: requests are chunked (150/batch) and a mid-batch failure leaves earlier batches applied.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, deleteMessages)
}

func labelMessages(ctx context.Context, d Deps, in labelMessagesIn) (organizeOut, *proterr.Error) {
	if perr := validateMessageIDs(in.MessageIDs); perr != nil {
		return organizeOut{}, perr
	}
	if perr := required("label_id", in.LabelID); perr != nil {
		return organizeOut{}, perr
	}
	if perr := validateLabelAction(in.Action); perr != nil {
		return organizeOut{}, perr
	}
	c, perr := client(ctx, d)
	if perr != nil {
		return organizeOut{}, perr
	}
	var err error
	if in.Action == "add" {
		err = c.LabelMessages(ctx, in.MessageIDs, in.LabelID)
	} else {
		err = c.UnlabelMessages(ctx, in.MessageIDs, in.LabelID)
	}
	if err != nil {
		return organizeOut{}, proterr.Map(err)
	}
	return organizeOut{OK: true, MessageIDs: in.MessageIDs}, nil
}

func markMessages(ctx context.Context, d Deps, in markMessagesIn) (organizeOut, *proterr.Error) {
	if perr := validateMessageIDs(in.MessageIDs); perr != nil {
		return organizeOut{}, perr
	}
	c, perr := client(ctx, d)
	if perr != nil {
		return organizeOut{}, perr
	}
	var err error
	if in.Read {
		err = c.MarkMessagesRead(ctx, in.MessageIDs...)
	} else {
		err = c.MarkMessagesUnread(ctx, in.MessageIDs...)
	}
	if err != nil {
		return organizeOut{}, proterr.Map(err)
	}
	return organizeOut{OK: true, MessageIDs: in.MessageIDs}, nil
}

type deleteMessagesIn struct {
	MessageIDs []string `json:"message_ids"`
}

func deleteMessages(ctx context.Context, d Deps, in deleteMessagesIn) (organizeOut, *proterr.Error) {
	if perr := validateMessageIDs(in.MessageIDs); perr != nil {
		return organizeOut{}, perr
	}
	c, perr := client(ctx, d)
	if perr != nil {
		return organizeOut{}, perr
	}
	if err := c.DeleteMessage(ctx, in.MessageIDs...); err != nil {
		return organizeOut{}, proterr.Map(err)
	}
	return organizeOut{OK: true, MessageIDs: in.MessageIDs}, nil
}

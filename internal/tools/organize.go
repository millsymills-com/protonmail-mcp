package tools

import (
	"context"

	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
		return &proterr.Error{Code: "proton/validation", Message: `action must be "add" or "remove"`}
	}
	return nil
}

type labelMessageIn struct {
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
		Name:        "proton_label_message",
		Description: "Adds or removes a label on one or more messages. Moving to Trash is action=add with label_id \"3\" (see proton_list_labels). Reversible.",
	}, labelMessage)
	addTool(server, d, &mcp.Tool{
		Name:        "proton_mark_messages",
		Description: "Marks one or more messages read (read=true) or unread (read=false). Reversible.",
	}, markMessages)
}

func labelMessage(ctx context.Context, d Deps, in labelMessageIn) (organizeOut, *proterr.Error) {
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

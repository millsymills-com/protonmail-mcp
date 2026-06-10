package tools

import (
	"context"
	"strconv"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type labelDTO struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Type  string   `json:"type"`
	Color string   `json:"color,omitempty"`
	Path  []string `json:"path,omitempty"`
}

type listLabelsIn struct{}
type listLabelsOut struct {
	Labels []labelDTO `json:"labels"`
}

func registerLabels(server *mcp.Server, d Deps) {
	addTool(server, d, &mcp.Tool{
		Name:        "proton_list_labels",
		Description: "Lists the account's labels and folders, including system labels (Inbox=0, AllDrafts=1, Sent=7, Trash=3, Spam=4, Archive=6, Starred=10). Use the returned IDs with proton_label_messages; label_id \"3\" moves a message to Trash.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, listLabels)
}

func listLabels(ctx context.Context, d Deps, _ listLabelsIn) (listLabelsOut, *proterr.Error) {
	c, perr := client(ctx, d)
	if perr != nil {
		return listLabelsOut{}, perr
	}
	raw, err := c.GetLabels(ctx, proton.LabelTypeSystem, proton.LabelTypeFolder, proton.LabelTypeLabel)
	if err != nil {
		return listLabelsOut{}, proterr.Map(err)
	}
	out := make([]labelDTO, len(raw))
	for i, l := range raw {
		out[i] = labelDTO{ID: l.ID, Name: l.Name, Type: labelTypeString(l.Type), Color: l.Color, Path: l.Path}
	}
	return listLabelsOut{Labels: out}, nil
}

func labelTypeString(t proton.LabelType) string {
	switch t {
	case proton.LabelTypeLabel:
		return "label"
	case proton.LabelTypeFolder:
		return "folder"
	case proton.LabelTypeSystem:
		return "system"
	default:
		return strconv.Itoa(int(t))
	}
}

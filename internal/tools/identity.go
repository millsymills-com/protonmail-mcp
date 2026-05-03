package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
)

type whoamiInput struct{}
type whoamiOutput struct {
	Email     string `json:"email" jsonschema:"the primary email of the logged-in account"`
	Name      string `json:"name,omitempty" jsonschema:"the user's display name if set"`
	UsedSpace int64  `json:"used_space_bytes" jsonschema:"current storage usage in bytes"`
	MaxSpace  int64  `json:"max_space_bytes" jsonschema:"plan's storage quota in bytes"`
}

type sessionStatusInput struct{}
type sessionStatusOutput struct {
	LoggedIn bool   `json:"logged_in"`
	Email    string `json:"email,omitempty"`
}

func registerIdentity(server *mcp.Server, d Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_whoami",
		Description: "Returns the logged-in Proton account's email, display name, and storage usage.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ whoamiInput) (*mcp.CallToolResult, whoamiOutput, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, whoamiOutput{}, nil
		}
		u, err := c.GetUser(ctx)
		if err != nil {
			return failure(proterr.Map(err)), whoamiOutput{}, nil
		}
		return nil, whoamiOutput{
			Email:     u.Email,
			Name:      u.DisplayName,
			UsedSpace: int64(u.UsedSpace),
			MaxSpace:  int64(u.MaxSpace),
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_session_status",
		Description: "Reports whether a session is currently authenticated.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ sessionStatusInput) (*mcp.CallToolResult, sessionStatusOutput, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return nil, sessionStatusOutput{LoggedIn: false}, nil
		}
		u, err := c.GetUser(ctx)
		if err != nil {
			return nil, sessionStatusOutput{LoggedIn: false}, nil
		}
		return nil, sessionStatusOutput{LoggedIn: true, Email: u.Email}, nil
	})
}

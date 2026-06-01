package tools

import (
	"context"

	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type whoamiInput struct{}
type whoamiOutput struct {
	Email           string `json:"email" jsonschema:"the primary email of the logged-in account"`
	Name            string `json:"name,omitempty" jsonschema:"the user's display name if set"`
	UsedSpace       int64  `json:"used_space_bytes" jsonschema:"current storage usage in bytes"`
	MaxSpace        int64  `json:"max_space_bytes" jsonschema:"plan's storage quota in bytes"`
	PersistDegraded bool   `json:"persist_degraded,omitempty" jsonschema:"true when the most recent background token-persist write failed"`
	PersistError    string `json:"persist_error,omitempty" jsonschema:"human-readable reason from the keychain layer"`
}

type sessionStatusInput struct{}
type sessionStatusOutput struct {
	LoggedIn        bool   `json:"logged_in"`
	Email           string `json:"email,omitempty"`
	PersistDegraded bool   `json:"persist_degraded,omitempty"`
	PersistError    string `json:"persist_error,omitempty"`
}

func registerIdentity(server *mcp.Server, d Deps) {
	addTool(server, d, &mcp.Tool{
		Name:        "proton_whoami",
		Description: "Returns the logged-in Proton account's email, display name, storage usage, and token-persistence health.",
	}, func(ctx context.Context, d Deps, _ whoamiInput) (whoamiOutput, *proterr.Error) {
		c, perr := client(ctx, d)
		if perr != nil {
			return whoamiOutput{}, perr
		}
		u, err := c.GetUser(ctx)
		if err != nil {
			return whoamiOutput{}, proterr.Map(err)
		}
		st := d.Session.Status()
		return whoamiOutput{
			Email:           u.Email,
			Name:            u.DisplayName,
			UsedSpace:       int64(u.UsedSpace),
			MaxSpace:        int64(u.MaxSpace),
			PersistDegraded: st.PersistDegraded,
			PersistError:    st.PersistError,
		}, nil
	})

	addTool(server, d, &mcp.Tool{
		Name:        "proton_session_status",
		Description: "Reports whether a session is currently authenticated and whether token persistence is healthy.",
	}, func(ctx context.Context, d Deps, _ sessionStatusInput) (sessionStatusOutput, *proterr.Error) {
		c, err := d.Session.Client(ctx)
		st := d.Session.Status()
		out := sessionStatusOutput{
			PersistDegraded: st.PersistDegraded,
			PersistError:    st.PersistError,
		}
		if err != nil {
			return out, nil
		}
		u, err := c.GetUser(ctx)
		if err != nil {
			return out, nil
		}
		out.LoggedIn = true
		out.Email = u.Email
		return out, nil
	})
}

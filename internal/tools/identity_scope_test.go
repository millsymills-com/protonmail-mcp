package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/tools"
)

// TestSessionStatusSurfacesKeyringUnlock proves proton_session_status reports
// the session's keyring-unlock capability derived from its token scope. The
// field is populated from Status even when Client() can't reach the backend,
// so the condition is observable before a decrypt call blows up.
func TestSessionStatusSurfacesKeyringUnlock(t *testing.T) {
	keyring.MockInit()
	sess, err := session.NewForTesting("http://invalid.test",
		keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r", Scope: "full"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}

	out := callSessionStatus(t, sess)
	if v, ok := out["keyring_unlock"]; !ok || v != "ok" {
		t.Fatalf("keyring_unlock = %v, want ok", out["keyring_unlock"])
	}
}

func TestSessionStatusKeyringUnlockUnderScoped(t *testing.T) {
	keyring.MockInit()
	sess, err := session.NewForTesting("http://invalid.test",
		keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r", Scope: "twofactor"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}

	out := callSessionStatus(t, sess)
	if v, ok := out["keyring_unlock"]; !ok || v != "under_scoped" {
		t.Fatalf("keyring_unlock = %v, want under_scoped", out["keyring_unlock"])
	}
}

func callSessionStatus(t *testing.T, sess *session.Session) map[string]any {
	t.Helper()
	mcpSrv := mcp.NewServer(&mcp.Implementation{Name: "protonmail-mcp", Version: "test"}, nil)
	tools.Register(mcpSrv, tools.Deps{Session: sess})

	clientT, serverT := mcp.NewInMemoryTransports()
	srvSession, err := mcpSrv.Connect(context.Background(), serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = srvSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	csess, err := client.Connect(context.Background(), clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = csess.Close() })

	res, err := csess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "proton_session_status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

package server_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/server"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func TestBootRegistersToolsAndDispatches(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	if err := kc.SaveSession(keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}); err != nil {
		t.Fatal(err)
	}
	rt := testvcr.New(t, "boot_dispatch")
	sess := session.New("https://mail.proton.me/api", kc, session.WithTransport(rt))
	sess.OnAuthRotated(keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	})

	mcpSrv := mcp.NewServer(&mcp.Implementation{Name: "protonmail-mcp", Version: "test"}, nil)
	server.RegisterAll(mcpSrv, sess)

	ctx := context.Background()
	clientT, serverT := mcp.NewInMemoryTransports()

	srvSession, err := mcpSrv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("mcp server connect: %v", err)
	}
	defer func() { _ = srvSession.Close() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("mcp client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	listed, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(listed.Tools) == 0 {
		t.Fatal("no tools registered")
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "proton_whoami"})
	if err != nil {
		t.Fatalf("call whoami: %v", err)
	}
	if res.IsError {
		t.Fatalf("whoami returned error: %v", res.Content)
	}
}

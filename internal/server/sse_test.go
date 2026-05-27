package server_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/server"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func TestSSEMuxServesTools(t *testing.T) {
	keyring.MockInit()
	sess := session.New("https://mail.proton.me/api", keychain.New())
	srv := mcp.NewServer(&mcp.Implementation{Name: "protonmail-mcp", Version: "test"}, nil)
	server.RegisterAll(srv, sess)

	ts := httptest.NewServer(server.SSEMux(srv))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, &mcp.SSEClientTransport{Endpoint: ts.URL + "/sse"}, nil)
	if err != nil {
		t.Fatalf("sse connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	listed, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(listed.Tools) == 0 {
		t.Fatal("no tools over SSE")
	}
}

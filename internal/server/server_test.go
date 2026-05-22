package server_test

import (
	"context"
	"testing"
	"time"

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

// TestRunWithOptionsHonoursCancelledContext exercises the RunWithOptions
// entry point. A pre-cancelled context returns the cancellation error
// without blocking on stdin, so coverage reaches the registration + Run
// call without an integration harness.
func TestRunWithOptionsHonoursCancelledContext(t *testing.T) {
	keyring.MockInit()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- server.RunWithOptions(ctx, "https://mail.proton.me/api", nil) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error from a cancelled context")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunWithOptions did not return within 5s of context cancel")
	}
}

// TestRunDefaultsExercise covers the Run wrapper (which delegates to
// RunWithOptions with the default API URL).
func TestRunDefaultsExercise(t *testing.T) {
	keyring.MockInit()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s of context cancel")
	}
}

// TestRunWithOptionsFallsBackToEnvAPIURL covers the env-fallback branch of
// RunWithOptions (apiURL empty -> default -> PROTONMAIL_MCP_API_URL).
func TestRunWithOptionsFallsBackToEnvAPIURL(t *testing.T) {
	keyring.MockInit()
	t.Setenv("PROTONMAIL_MCP_API_URL", "https://test-override.example.test/api")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- server.RunWithOptions(ctx, "", nil) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunWithOptions did not return within 5s of context cancel")
	}
}

package tools_test

import (
	"context"
	"testing"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/millsymills-com/protonmail-mcp/internal/keyring"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

// fixedAddrKeyrings delegates everything to the real session but serves a
// generated in-memory keyring for every address ID, so replay never attempts a
// real keyring unlock (the cassette's keys are scrubbed fixtures and the test
// session has no mailbox password). Address IDs come from the replayed
// GetAddresses response because opaque Proton IDs are not scrubbed and are
// unknown until the cassette is recorded.
type fixedAddrKeyrings struct {
	session.Service
	kr *crypto.KeyRing
}

func (f fixedAddrKeyrings) Keyrings(ctx context.Context) (*keyring.Keyrings, error) {
	c, err := f.Service.Client(ctx)
	if err != nil {
		return nil, err
	}
	addrs, err := c.GetAddresses(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*crypto.KeyRing, len(addrs))
	for _, a := range addrs {
		m[a.ID] = f.kr
	}
	return &keyring.Keyrings{User: f.kr, Addr: m}, nil
}

func newFixedAddrKeyrings(t *testing.T) func(session.Service) session.Service {
	t.Helper()
	key, err := crypto.GenerateKey("test", "test@example.test", "x25519", 0)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	kr, err := crypto.NewKeyRing(key)
	if err != nil {
		t.Fatalf("keyring: %v", err)
	}
	return func(s session.Service) session.Service {
		return fixedAddrKeyrings{Service: s, kr: kr}
	}
}

func TestCreateDraftHappyCassette(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "create_draft_happy",
		testharness.WithSessionService(newFixedAddrKeyrings(t)))
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_create_draft", map[string]any{
		"to":      []any{"recipient@example.test"},
		"subject": "Hello from the agent",
		"body":    "This is a draft body.",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["message"]; !ok {
		t.Fatalf("envelope missing %q: %#v", "message", out)
	}
}

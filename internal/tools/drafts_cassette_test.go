package tools_test

import (
	"context"
	"fmt"
	"sync"
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
	gen *lazyKey
}

// lazyKey memoizes a generated keyring. It must be generated only after the
// first replayed response: go-proton-api calls crypto.UpdateTime with each
// response's Date header, pinning gopenpgp's clock to the (past) recording
// moment. A key generated before that (at real now) is dated after the pinned
// clock and is rejected as a future key when CreateDraft encrypts the body
// ("no valid encryption keys"). Generating lazily inside Keyrings — after
// GetAddresses has pinned the clock — stamps the key at the cassette's time.
type lazyKey struct {
	once sync.Once
	kr   *crypto.KeyRing
	err  error
}

func (l *lazyKey) keyring() (*crypto.KeyRing, error) {
	l.once.Do(func() {
		key, err := crypto.GenerateKey("test", "test@example.test", "x25519", 0)
		if err != nil {
			l.err = fmt.Errorf("generate test keyring: %w", err)
			return
		}
		kr, err := crypto.NewKeyRing(key)
		if err != nil {
			l.err = fmt.Errorf("build test keyring: %w", err)
			return
		}
		l.kr = kr
	})
	return l.kr, l.err
}

func (f fixedAddrKeyrings) Keyrings(ctx context.Context) (*keyring.Keyrings, error) {
	c, err := f.Client(ctx)
	if err != nil {
		return nil, err
	}
	addrs, err := c.GetAddresses(ctx)
	if err != nil {
		return nil, err
	}
	// Generate only here, after GetAddresses has replayed and pinned gopenpgp's
	// clock to the cassette's recording time (see lazyKey). Generating before
	// this point reintroduces the future-dated-key encryption failure.
	kr, err := f.gen.keyring()
	if err != nil {
		return nil, err
	}
	m := make(map[string]*crypto.KeyRing, len(addrs))
	for _, a := range addrs {
		m[a.ID] = kr
	}
	return &keyring.Keyrings{User: kr, Addr: m}, nil
}

func newFixedAddrKeyrings(t *testing.T) func(session.Service) session.Service {
	t.Helper()
	gen := &lazyKey{}
	return func(s session.Service) session.Service {
		return fixedAddrKeyrings{Service: s, gen: gen}
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
	msg, ok := out["message"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing %q object: %#v", "message", out)
	}
	if id, _ := msg["id"].(string); id == "" {
		t.Fatalf("created draft has empty id: %#v", msg)
	}
}

func TestCreateDraftHTMLHappyCassette(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootWithCassette(t, "create_draft_html_happy",
		testharness.WithSessionService(newFixedAddrKeyrings(t)))
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_create_draft", map[string]any{
		"to":        []any{"recipient@example.test"},
		"subject":   "Hello in HTML",
		"body":      "<p>This is an HTML draft body.</p>",
		"mime_type": "text/html",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	msg, ok := out["message"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing %q object: %#v", "message", out)
	}
	if id, _ := msg["id"].(string); id == "" {
		t.Fatalf("created draft has empty id: %#v", msg)
	}
}

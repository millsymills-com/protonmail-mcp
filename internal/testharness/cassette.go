package testharness

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
	"github.com/millsmillsymills/protonmail-mcp/internal/tools"
)

const cassetteBaseURL = "https://mail.proton.me/api"

// BootWithCassette constructs a Harness backed by a pre-recorded cassette
// instead of the dev server. scenarioName must match a .yaml file under
// testdata/cassettes/ relative to the calling test's package.
func BootWithCassette(t *testing.T, scenarioName string, opts ...Option) *Harness {
	t.Helper()
	// Use in-memory keyring so tests never touch the real OS keychain.
	keyring.MockInit()

	rt := testvcr.New(t, scenarioName)
	kc := keychain.New()
	seed := keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}
	if err := kc.SaveSession(seed); err != nil {
		t.Fatalf("seed keychain: %v", err)
	}
	sess := session.New(cassetteBaseURL, kc, session.WithTransport(rt))
	// Seed the bearer token directly so Raw() skips the cold-start refresh path.
	sess.OnAuthRotated(seed)

	mcpSrv := mcp.NewServer(&mcp.Implementation{Name: "protonmail-mcp", Version: "test"}, nil)
	tools.Register(mcpSrv, tools.Deps{Session: sess})

	h := &Harness{
		t:      t,
		sess:   sess,
		mcpSrv: mcpSrv,
	}
	if err := h.connectInMemoryClient(); err != nil {
		t.Fatalf("connect mcp client: %v", err)
	}
	t.Cleanup(h.Close)
	return h
}

// Ping issues a GET /tests/ping through the raw session client.
func (h *Harness) Ping(ctx context.Context) error {
	_, err := h.sess.Raw(ctx).Get(ctx, "/tests/ping")
	return err
}

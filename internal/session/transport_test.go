package session_test

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
)

type countingTransport struct{ hits atomic.Int32 }

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.hits.Add(1)
	return &http.Response{StatusCode: 599, Body: http.NoBody, Request: req}, nil
}

func TestNewWiresTransportIntoBothClients(t *testing.T) {
	rt := &countingTransport{}
	s := session.New("https://example.test/api", keychain.New(), session.WithTransport(rt))

	// resty surfaces 599 as a non-error status, so we ignore both return
	// values — the only assertion is that the stub transport was invoked
	// (checked via rt.hits below).
	_, _ = s.RawForTest().Get(t.Context(), "/core/v4/domains")
	if got := rt.hits.Load(); got < 1 {
		t.Fatalf("resty client did not use injected transport: hits=%d", got)
	}

	_ = s.ManagerForTest().Ping(t.Context())
	if got := rt.hits.Load(); got < 2 {
		t.Fatalf("proton.Manager did not use injected transport: hits=%d", got)
	}
}

package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
)

func TestBearerAuth(t *testing.T) {
	const token = "s3cret"
	var served bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served = true
		w.WriteHeader(http.StatusNoContent)
	})
	h := bearerAuth(token, next)

	tests := []struct {
		name       string
		header     string
		setHeader  bool
		wantServed bool
		wantStatus int
	}{
		{"valid", "Bearer s3cret", true, true, http.StatusNoContent},
		{"wrong token", "Bearer nope", true, false, http.StatusUnauthorized},
		{"missing prefix", "s3cret", true, false, http.StatusUnauthorized},
		{"no header", "", false, false, http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			served = false
			req := httptest.NewRequest(http.MethodGet, "/sse", nil)
			if tc.setHeader {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if served != tc.wantServed {
				t.Fatalf("served=%v want %v", served, tc.wantServed)
			}
			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

type authRT struct {
	token string
	base  http.RoundTripper
}

func (a authRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", "Bearer "+a.token)
	return a.base.RoundTrip(r)
}

func newTestServer(t *testing.T) *mcp.Server {
	t.Helper()
	keyring.MockInit()
	sess := session.New("https://mail.proton.me/api", keychain.New())
	srv := mcp.NewServer(&mcp.Implementation{Name: "protonmail-mcp", Version: "test"}, nil)
	RegisterAll(srv, sess)
	return srv
}

func TestServeSSEListenerLifecycle(t *testing.T) {
	const token = "tok"
	srv := newTestServer(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	url := "http://" + ln.Addr().String() + "/sse"

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveSSEListener(ctx, ln, token, srv) }()

	connCtx, connCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer connCancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(connCtx, &mcp.SSEClientTransport{
		Endpoint:   url,
		HTTPClient: &http.Client{Transport: authRT{token: token, base: http.DefaultTransport}},
	}, nil)
	if err != nil {
		t.Fatalf("authorized connect: %v", err)
	}
	listed, err := cs.ListTools(connCtx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(listed.Tools) == 0 {
		t.Fatal("no tools over authenticated SSE")
	}
	_ = cs.Close()

	// A client with no bearer token must not reach the MCP handler.
	noAuth := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	if cs2, err := noAuth.Connect(connCtx, &mcp.SSEClientTransport{Endpoint: url}, nil); err == nil {
		if _, lerr := cs2.ListTools(connCtx, nil); lerr == nil {
			t.Fatal("unauthenticated client must not list tools")
		}
		_ = cs2.Close()
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveSSEListener returned %v, want nil on ctx cancel", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveSSEListener did not return after ctx cancel")
	}
}

func TestServeSSEBindFailure(t *testing.T) {
	srv := newTestServer(t)

	// Occupy a port, then ask serveSSE to bind the same one.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr is not TCP: %T", ln.Addr())
	}

	err = serveSSE(context.Background(),
		transportConfig{kind: "sse", host: "127.0.0.1", port: addr.Port, token: "tok"}, srv)
	if err == nil {
		t.Fatal("expected bind failure on occupied port")
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Fatalf("want listen error, got %v", err)
	}
}

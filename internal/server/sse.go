package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SSEMux returns an http.Handler serving the MCP SSE transport at /sse. One
// *mcp.Server is reused across sessions; SSEHandler calls Connect per GET.
// Exported so tests can host it via httptest. DNS-rebinding/localhost
// protection is left on (SSEOptions default) — correct for a loopback listener.
func SSEMux(srv *mcp.Server) http.Handler {
	h := mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.SSEOptions{})
	mux := http.NewServeMux()
	mux.Handle("/sse", h)
	return mux
}

// bearerAuth rejects any request whose Authorization header does not carry the
// exact bearer token, before the MCP handler runs. The comparison is
// constant-time so a timing side-channel can't reveal the token.
func bearerAuth(token string, next http.Handler) http.Handler {
	want := []byte(token)
	const prefix = "Bearer "
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, prefix) ||
			subtle.ConstantTimeCompare([]byte(h[len(prefix):]), want) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serveSSE binds host:port and serves the bearer-authenticated SSE handler
// until ctx is cancelled.
func serveSSE(ctx context.Context, cfg transportConfig, srv *mcp.Server) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.host, cfg.port))
	if err != nil {
		return fmt.Errorf("listen %s:%d: %w", cfg.host, cfg.port, err)
	}
	return serveSSEListener(ctx, ln, cfg.token, srv)
}

// serveSSEListener serves the bearer-authenticated SSE handler on ln until ctx
// is cancelled, then shuts down gracefully. Split from serveSSE so tests can
// drive the lifecycle over an ephemeral listener.
func serveSSEListener(ctx context.Context, ln net.Listener, token string, srv *mcp.Server) error {
	httpSrv := &http.Server{Handler: bearerAuth(token, SSEMux(srv)), ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutCtx); err != nil {
			return fmt.Errorf("sse shutdown: %w", err)
		}
		// Shutdown unblocks Serve with ErrServerClosed; drain errCh so a real
		// Serve failure that raced the shutdown isn't silently dropped.
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("sse serve: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("sse serve: %w", err)
	}
}

package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
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

// serveSSE binds host:port and serves SSEMux until ctx is cancelled, then
// shuts down gracefully.
func serveSSE(ctx context.Context, cfg transportConfig, srv *mcp.Server) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.host, cfg.port))
	if err != nil {
		return fmt.Errorf("listen %s:%d: %w", cfg.host, cfg.port, err)
	}
	httpSrv := &http.Server{Handler: SSEMux(srv), ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("sse serve: %w", err)
	}
}

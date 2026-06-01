# 0002. Keep legacy SSE for the remote transport; defer Streamable HTTP

- **Status:** Accepted
- **Date:** 2026-06-01
- **Deciders:** maintainer
- **Issue:** #131 (follow-up from #128)

## Context

The remote transport added in #128 serves the MCP HTTP endpoint with the
go-sdk's SSE handler. `internal/server/sse.go` constructs it via
`mcp.NewSSEHandler` (`SSEMux`) and `serveSSE` binds the listener;
`internal/server/server.go` dispatches to `serveSSE` when
`transportConfig.kind == "sse"` (the `cfg.kind == "sse"` branch). This is the
2024-11-05 MCP HTTP+SSE transport: two endpoints, no resumability.

The same go-sdk (`github.com/modelcontextprotocol/go-sdk v1.6.0`) also ships
`mcp.NewStreamableHTTPHandler` (`mcp/streamable.go`), the 2025-03-26
Streamable HTTP transport: a single endpoint that upgrades to a stream on
demand, with session IDs and event-replay resumability.

#131 asks for a deliberate decision on whether to migrate, recorded as an ADR,
rather than an implicit default.

### Current shape

- `internal/server/transport.go` — `transportConfig{ kind, host, port, token }`
  and `transportConfigFromEnv`. `kind` is `"stdio"` or `"sse"`; the value is
  read from `PROTONMAIL_MCP_TRANSPORT`.
- `internal/server/server.go` — single dispatch point:
  `if cfg.kind == "sse" { return serveSSE(ctx, cfg, srv) }`, otherwise stdio.
- `internal/server/sse.go` — `SSEMux` (wraps `mcp.NewSSEHandler`),
  `bearerAuth`, `serveSSE`, `serveSSEListener`.

The transport is isolated behind `transportConfig.kind` and `serveSSE`. The
handler construction is one call (`mcp.NewSSEHandler` in `SSEMux`), and both
SDK constructors share the same signature
(`func(getServer func(*http.Request) *Server, opts *...Options) *...Handler`),
so the bearer-auth wrapper, listener lifecycle, and graceful shutdown in
`serveSSEListener` carry over unchanged.

### Test and cassette coverage

The shipped SSE path is covered:

- `internal/server/transport_test.go` — `TestTransportConfigFromEnv` (env
  parsing, token-length floor).
- `internal/server/sse_serve_test.go` — `TestBearerAuth`,
  `TestServeSSEListenerLifecycle`, `TestServeSSEBindFailure`.
- `internal/server/testdata/cassettes/boot_dispatch.yaml` — boot/dispatch
  cassette.

Streamable HTTP has none of this yet.

## Decision

Keep the legacy SSE transport. Do not adopt Streamable HTTP now.

Rationale:

- SSE is what is shipped, tested, and cassette-covered. Migrating spends
  effort to replace a working path with no behavioural gain for current
  clients.
- No current consuming client requires Streamable HTTP's single-endpoint
  routing or stream resumability. Sessions are short and the bearer-token
  access model does not depend on the transport.
- The transport is already isolated behind `transportConfig.kind` and
  `serveSSE`, so migrating later stays a localized change rather than a
  rewrite. Deferring costs nothing in future flexibility.

## Consequences

- The remote endpoint stays on the 2024-11-05 HTTP+SSE transport: two
  endpoints, no resumability. Clients that only speak the 2025-03-26
  Streamable HTTP transport cannot connect over the remote transport until
  this decision is revisited.
- The `transportConfig.kind` switch and `serveSSE` seam are load-bearing for
  the cheap-swap property and should stay isolated.

### Swap point for a future maintainer

When this is revisited, the concrete change is:

1. In `internal/server/sse.go`, swap `mcp.NewSSEHandler` for
   `mcp.NewStreamableHTTPHandler` inside `SSEMux` (signatures match; the
   `bearerAuth` wrapper and `serveSSEListener` lifecycle are reused as-is).
2. Decide whether Streamable HTTP replaces SSE under the existing `"sse"`
   `transportConfig.kind`, or is added as a new `kind` value alongside it
   (the latter keeps both transports available behind one flag).
3. Re-record `internal/server/testdata/cassettes/boot_dispatch.yaml` and add
   coverage mirroring `sse_serve_test.go` for the new handler.

## Revisit when

- A consuming client needs the 2025-03-26 Streamable HTTP transport
  (single-endpoint or resumability), or
- the go-sdk deprecates `NewSSEHandler` / the SSE transport.

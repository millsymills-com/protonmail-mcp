package server

import (
	"fmt"
	"strconv"
)

// minSSETokenLen is the floor for PROTONMAIL_MCP_SSE_TOKEN. The SSE endpoint
// exposes an authenticated Proton session, so a trivially short (guessable)
// token is rejected at startup rather than relied upon.
const minSSETokenLen = 16

type transportConfig struct {
	kind  string // "stdio" | "sse"
	host  string // sse only
	port  int    // sse only
	token string // sse only; bearer token every request must present
}

// transportConfigFromEnv reads PROTONMAIL_MCP_TRANSPORT/_HOST/_PORT. getenv is
// injected so tests need no process env. stdio is the default; sse requires an
// explicit port (binding a listener is never implicit).
func transportConfigFromEnv(getenv func(string) string) (transportConfig, error) {
	kind := getenv("PROTONMAIL_MCP_TRANSPORT")
	if kind == "" {
		kind = "stdio"
	}
	switch kind {
	case "stdio":
		return transportConfig{kind: "stdio"}, nil
	case "sse":
		host := getenv("PROTONMAIL_MCP_HOST")
		if host == "" {
			host = "127.0.0.1"
		}
		rawPort := getenv("PROTONMAIL_MCP_PORT")
		if rawPort == "" {
			return transportConfig{}, fmt.Errorf("PROTONMAIL_MCP_PORT is required when PROTONMAIL_MCP_TRANSPORT=sse")
		}
		port, err := strconv.Atoi(rawPort)
		if err != nil || port < 1 || port > 65535 {
			return transportConfig{}, fmt.Errorf("PROTONMAIL_MCP_PORT %q is not a valid port (1-65535)", rawPort)
		}
		// The SSE endpoint exposes the authenticated Proton session, so it is
		// never served unauthenticated — not even on loopback, where any local
		// process could otherwise reach it. Require a bearer token clients must
		// present as `Authorization: Bearer <token>`.
		token := getenv("PROTONMAIL_MCP_SSE_TOKEN")
		if token == "" {
			return transportConfig{}, fmt.Errorf(
				"PROTONMAIL_MCP_SSE_TOKEN is required when PROTONMAIL_MCP_TRANSPORT=sse " +
					"(generate a random secret; clients must send `Authorization: Bearer <token>`)")
		}
		if len(token) < minSSETokenLen {
			return transportConfig{}, fmt.Errorf(
				"PROTONMAIL_MCP_SSE_TOKEN must be at least %d characters (generate a random secret)",
				minSSETokenLen)
		}
		return transportConfig{kind: "sse", host: host, port: port, token: token}, nil
	default:
		return transportConfig{}, fmt.Errorf("invalid PROTONMAIL_MCP_TRANSPORT %q (allowed: stdio, sse)", kind)
	}
}

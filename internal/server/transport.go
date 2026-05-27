package server

import (
	"fmt"
	"strconv"
)

type transportConfig struct {
	kind string // "stdio" | "sse"
	host string // sse only
	port int    // sse only
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
		return transportConfig{kind: "sse", host: host, port: port}, nil
	default:
		return transportConfig{}, fmt.Errorf("invalid PROTONMAIL_MCP_TRANSPORT %q (allowed: stdio, sse)", kind)
	}
}

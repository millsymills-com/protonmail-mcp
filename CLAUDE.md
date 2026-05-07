# protonmail-mcp — Claude Code instructions

## Canonical MCP standards

Authoritative source: `~/Desktop/Projects/consistency-check/docs/standards/`. This repo is graded against `mcp.md`, `go.md`, and `mcp-protocol.md`.

Run the audit:

```bash
cd ~/Desktop/Projects/consistency-check
uv run consistency-check audit --repo protonmail-mcp
```

## Repo-specific notes

Project layout follows standard Go conventions: `cmd/protonmail-mcp/` for the entrypoint, `internal/` for protocol handlers, raw API client, error mapping, keychain, and session management.

## Agent skills

### Issue tracker

GitHub Issues at `millsmillsymills/protonmail-mcp`. See `docs/agents/issue-tracker.md`.

### Triage labels

Canonical names used verbatim (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout (no per-area split). `CONTEXT.md` and `docs/adr/` are not yet present; the consumer skills proceed silently when absent and `/grill-with-docs` will populate them lazily as terms and decisions get resolved. See `docs/agents/domain.md`.

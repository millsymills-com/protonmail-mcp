# Security policy

## Reporting a vulnerability

Please report security issues privately via GitHub's [private vulnerability reporting](https://github.com/millsmillsymills/protonmail-mcp/security/advisories/new) for this repository.

Do not file public issues for vulnerabilities.

Expect an initial response within 7 days. Coordinated disclosure timeline is negotiated case by case.

## Scope

In scope:

- The `protonmail-mcp` binary and all code under `cmd/` and `internal/`.
- The MCP tool surface registered by `internal/server` and `internal/tools`.
- Credential handling via `internal/keychain` and `internal/session`.

Out of scope:

- Vulnerabilities in upstream `go-proton-api`, `gopenpgp`, or `go-keyring` — report those to their respective maintainers.
- Issues that require physical access to an unlocked machine (the keychain trust boundary is the OS user session).
- Prompt-injection attacks against the LLM driving the MCP client. The server's job is to validate inputs at its boundary; LLM trust is the host's responsibility.

## Trust model

- Credentials and refresh tokens live in the macOS Keychain only, under service `protonmail-mcp`. They are never written to disk by this binary.
- Mutating tools are gated behind `PROTONMAIL_MCP_ENABLE_WRITES=1`. Read tools are always registered.
- The server speaks MCP over stdio only — no network listener, no IPC socket.
- Logs redact any field name containing `password`, `passphrase`, `token`, `secret`, `totp`, or `key`.

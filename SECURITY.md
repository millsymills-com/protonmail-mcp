# Security policy

## Reporting a vulnerability

Please report security issues privately via GitHub's [private vulnerability reporting](https://github.com/millsymills-com/protonmail-mcp/security/advisories/new) for this repository.

Do not file public issues for vulnerabilities.

Expect an initial response within 7 days. Coordinated disclosure timeline is negotiated case by case.

## Scope

In scope:

- The `protonmail-mcp` binary and all code under `cmd/` and `internal/`.
- The MCP tool surface registered by `internal/server` and `internal/tools`.
- Credential handling via `internal/keychain` and `internal/session`.

Out of scope:

- Vulnerabilities in upstream `go-proton-api`, `gopenpgp`, or `go-keyring` - report those to their respective maintainers.
- Issues that require physical access to an unlocked machine (the keychain trust boundary is the OS user session).
- Prompt-injection attacks against the LLM driving the MCP client. The server's job is to validate inputs at its boundary; LLM trust is the host's responsibility.

## Trust model

- The default `keychain` backend stores credentials and refresh tokens in the OS keychain (macOS Keychain, or the Linux Secret Service over D-Bus via `zalando/go-keyring`) under service `protonmail-mcp`. The optional `file` backend (`PROTONMAIL_MCP_CREDENTIAL_BACKEND=file`) instead writes a 0600 state file under `$PROTONMAIL_MCP_STATE_DIR` for headless deployments, so on those deployments the on-disk state file is part of the trust boundary.
- Mutating tools are gated behind `PROTONMAIL_MCP_ENABLE_WRITES=1`. Read tools are always registered.
- The server speaks MCP over stdio by default, with no network listener. An optional SSE transport (`PROTONMAIL_MCP_TRANSPORT=sse`) binds an HTTP listener (`127.0.0.1` by default) gated behind a bearer token (`PROTONMAIL_MCP_SSE_TOKEN`, at least 16 characters) that is required regardless of bind address.
- Logs redact any field name containing `password`, `passphrase`, `token`, `secret`, `totp`, or `key`.

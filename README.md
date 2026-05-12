<img src="assets/logo.png" alt="protonmail-mcp logo" width="160" align="right">

# protonmail-mcp

A Model Context Protocol (MCP) server for [Proton Mail](https://proton.me/mail), letting Claude Code (or any MCP client) manage addresses, custom domains, mail settings, and encryption keys.

## Status

v1. 23 tools total: 12 reads (always registered) + 11 writes (registered when `PROTONMAIL_MCP_ENABLE_WRITES=1`).

| Capability | v1 | Notes |
|---|---|---|
| Addresses (list, get, set status, delete) | yes | via `go-proton-api` |
| Create address (alias on custom domain) | yes | via `internal/protonraw` |
| Update address display name + signature | yes | global account fields; upstream has no per-address setter |
| Custom domains (list, get, add, verify, remove) | yes | via `internal/protonraw` |
| Mail settings (get, update display name + signature) | yes | |
| Account settings (get, update telemetry + crash reports) | yes | locale update is not exposed by upstream |
| Encryption keys (list with fingerprint + armored public key) | yes | via `gopenpgp/v2` |
| Encryption key generation / set primary | **deferred to v1.5** | requires keyring unlock + signed KeyList |
| Mail search + header inspection (read-only) | yes | metadata + raw headers; body decryption needs unlocked keyring (v1.5) |
| Mail send / draft / label mutations | **v2** | |
| Calendar / Drive | **v3** | |

## Install

Requires Go 1.26+ (the toolchain is auto-bumped by `go-proton-api` master).

The Go module path is `github.com/millsmillsymills/protonmail-mcp`. Build from a clone:

```
git clone https://github.com/millsymills-com/protonmail-mcp.git
cd protonmail-mcp
go build -o ./protonmail-mcp ./cmd/protonmail-mcp
```

Or `go install github.com/millsmillsymills/protonmail-mcp/cmd/protonmail-mcp@latest`.

`go.mod` already pins `go-proton-api` to a master HEAD pseudo-version and adds a `replace` directive routing `github.com/go-resty/resty/v2` to ProtonMail's fork. Both are required — do not remove them.

## First-time login

```
./protonmail-mcp login
```

Prompts for your Proton email, password, and (if 2FA is enabled) an `otpauth://` URI or a one-shot 6-digit code. Pasting the URI lets the server refresh sessions silently; pasting a code requires re-login on token expiry.

Credentials are stored in the macOS Keychain under service `protonmail-mcp`.

```
./protonmail-mcp status
```

Confirms the session and prints email + storage. Run `./protonmail-mcp logout` to clear stored credentials.

## Run as MCP server

Configure your MCP host (Claude Code, etc.) to launch:

```
./protonmail-mcp
```

over stdio. By default the server registers **read-only** tools. To expose mutating tools (create address, add domain, delete, update settings):

```
PROTONMAIL_MCP_ENABLE_WRITES=1 ./protonmail-mcp
```

## Environment variables

| Variable | Purpose | Default |
|---|---|---|
| `PROTONMAIL_MCP_ENABLE_WRITES` | When `1`/`true`/`yes`, registers mutating tools. Read tools are always available. | unset (reads only) |
| `PROTONMAIL_MCP_LOG_LEVEL` | `debug` for verbose JSON logs to stderr. | `info` |
| `PROTONMAIL_MCP_API_URL` | Override Proton API base URL (used in tests). | `https://mail.proton.me/api` |

## Tool reference

See `docs/superpowers/specs/2026-04-26-protonmail-mcp-design.md` §5 for the full inventory and field-by-field schemas. Key behaviors worth knowing:

- **`proton_update_address`** updates the *global* account display name / signature (upstream's `SetDisplayName` / `SetSignature` are not per-address). The `id` parameter is accepted for forward compatibility but ignored. The tool description spells this out.
- **`proton_update_core_settings`** toggles telemetry and crash reports — `SetUserSettingsLocale` is not exposed by upstream `go-proton-api` master, so locale update is intentionally absent.
- **`proton_list_address_keys`** uses `gopenpgp/v2` to extract the fingerprint + armored public key from each stored key. Private key material never leaves the process.
- **DNS records** for custom domains are returned as structured JSON; orchestrate with your DNS provider's MCP (e.g. Gandi MCP) to publish them.

### Reads (always)

`proton_whoami`, `proton_session_status`, `proton_list_addresses`, `proton_get_address`, `proton_list_custom_domains`, `proton_get_custom_domain`, `proton_get_catchall`, `proton_get_mail_settings`, `proton_get_core_settings`, `proton_list_address_keys`, `proton_search_messages`, `proton_get_message`.

### Writes (gated)

`proton_create_address`, `proton_update_address`, `proton_set_address_status`, `proton_delete_address`, `proton_add_custom_domain`, `proton_verify_custom_domain`, `proton_remove_custom_domain`, `proton_set_catchall`, `proton_disable_catchall`, `proton_update_mail_settings`, `proton_update_core_settings`.

## Security model

See spec §8. tl;dr:

- Credentials and refresh tokens stored in macOS Keychain only; never in files.
- Logs redact any field name containing `password`, `passphrase`, `token`, `secret`, `totp`, `key`.
- Writes opt-in via env flag — Claude Code's per-tool permission UI provides defense-in-depth.
- No daemon, no IPC socket, no HTTP listener — stdio MCP only.
- Sends `x-pm-appversion: macos-bridge@3.24.1` because Proton's API rejects unknown product names with code 2064. We impersonate proton-bridge (live-tested 2026-04-26 against `mail.proton.me`) — if Proton tightens the minimum (codes 5002/5003), bump the version in `internal/session/appversion.go` to whatever proton-bridge has tagged latest.

## Development

```
go vet ./...
go test ./... -race                            # unit + harness tests (uses go-proton-api dev server in-process)
go test -tags=integration ./... -race          # extra integration suite for read tools
```

Manual pre-release checks: `docs/testing-checklist.md`.

## License

MIT.

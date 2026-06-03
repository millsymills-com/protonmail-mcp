# Pre-release manual testing checklist

Run before tagging any release. Uses a real Proton account (Unlimited or Business). Reset session afterwards with `protonmail-mcp logout`.

## Setup

- [ ] `go build -o ./protonmail-mcp ./cmd/protonmail-mcp`
- [ ] `./protonmail-mcp logout` (clean slate)

## Auth

- [ ] `./protonmail-mcp login` with valid credentials and 2FA enabled - succeeds with `otpauth://` URI
- [ ] `./protonmail-mcp login` with valid credentials and a 6-digit code - succeeds, prints warning about refresh
- [ ] `./protonmail-mcp login` with wrong password - fails with the underlying Proton error (not the masked `proton/auth_required` message)
- [ ] `./protonmail-mcp status` - prints the logged-in email + storage usage
- [ ] No `code 2064` ("Platform/Product is not valid") errors on any login or read - confirms the `macos-bridge@3.24.1` appversion impersonation is still accepted by Proton's product allowlist

## Read tools (writes flag OFF)

Run via Claude Code (or any MCP host) connected to `./protonmail-mcp`. All 12 read tools should be advertised; none of the 11 write tools should appear.

- [ ] `proton_whoami` - returns the right email and storage usage
- [ ] `proton_session_status` - `logged_in: true`, email matches whoami
- [ ] `proton_list_addresses` - includes the primary address and any aliases
- [ ] `proton_get_address` for the primary address - display name + status look right
- [ ] `proton_list_custom_domains` - includes any custom domains; verification states match the Proton web UI
- [ ] `proton_get_custom_domain` for one verified domain - returns DNS records matching the live records at the registrar
- [ ] `proton_get_catchall` for one verified domain - reports the catch-all address (or none) matching the web UI
- [ ] `proton_get_mail_settings` - display name + signature match the web UI
- [ ] `proton_get_core_settings` - telemetry / crash-report flags match the web UI
- [ ] `proton_list_address_keys` for the primary address - fingerprints match what `gpg --list-keys` shows after importing the armored public key
- [ ] `proton_search_messages` with a known query - returns matching message metadata
- [ ] `proton_get_message` for one returned id - returns metadata; with `include_headers=true` returns raw + parsed headers; with `include_body=true` decrypts the plaintext body when the keyring is unlocked

## Write tools (`PROTONMAIL_MCP_ENABLE_WRITES=1`)

Use a throwaway custom domain you don't mind churning. With writes enabled, all 23 tools (12 reads + 11 writes) should be advertised.

- [ ] `proton_add_custom_domain` for a new domain - returns required DNS records
- [ ] Manually publish the records (or via Gandi MCP)
- [ ] `proton_verify_custom_domain` after DNS propagation - moves verification states forward
- [ ] `proton_create_address` on the new domain - alias appears in `proton_list_addresses`
- [ ] `proton_set_catchall` on the new domain - catch-all set, reflected by `proton_get_catchall` and the web UI
- [ ] `proton_disable_catchall` on the new domain - catch-all cleared, reflected by `proton_get_catchall` and the web UI
- [ ] `proton_update_address` with `display_name` set - global account display name changes (note: this is global, not per-address; the `id` parameter is ignored)
- [ ] `proton_set_address_status enabled=false` - alias disabled in web UI
- [ ] `proton_set_address_status enabled=true` - re-enabled
- [ ] `proton_delete_address` - alias gone
- [ ] `proton_remove_custom_domain` - domain gone
- [ ] `proton_update_mail_settings` with `signature` - signature appears in next outgoing mail
- [ ] `proton_update_core_settings` with `telemetry: false` - toggle reflects in web UI
- [ ] `proton_update_core_settings` with `crash_reports: false` - toggle reflects in web UI

## Failure modes

- [ ] Force a CAPTCHA (run login from an unusual IP, e.g. via VPN) - `proton/captcha` error includes the verification token
- [ ] Make 30+ rapid identical requests - eventually `proton/rate_limited` with retry-after

## Cleanup

- [ ] `./protonmail-mcp logout`
- [ ] `security find-generic-password -s protonmail-mcp -a username` returns "not found" (macOS-only; on Linux the credentials live in the Secret Service, e.g. check with `secret-tool`)

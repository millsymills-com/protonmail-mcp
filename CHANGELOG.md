# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Cross-link to canonical MCP standards (`consistency-check/docs/standards/`).

## [1.0.0] - 2026-05-05

### Added
- Address tools (list, get, create, set status, delete, update display
  name + signature) backed by `go-proton-api` and the internal `protonraw`
  client.
- Custom domain tools (list, get, add, verify, remove) and DNS record
  output for downstream DNS-provider MCPs.
- Catchall configuration tools (get, set, disable).
- Mail settings tools (get, update display name + signature).
- Core/account settings tools (get, update telemetry + crash reports).
- Address-key inspection tool returning fingerprints and armored public keys.
- Read-only mail search and per-message header inspection.
- Stdio MCP server with read tools always registered and write tools
  gated behind `PROTONMAIL_MCP_ENABLE_WRITES=1`.
- macOS Keychain credential storage and 2FA-aware login flow.
- Slog-based JSON logging to stderr with automatic redaction of
  credential-bearing field names.
- Pre-public security sweep, CI hardening, govulncheck step, and pinned
  GitHub Actions.
- 8-bit ProtonMail logo and favicon assets.

[Unreleased]: https://github.com/millsymills-com/protonmail-mcp/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/millsymills-com/protonmail-mcp/releases/tag/v1.0.0

# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Cross-link to canonical MCP standards (`consistency-check/docs/standards/`).
- `CONTRIBUTING.md` (#33).
- `.golangci.yml` config with a full clear of new findings (GO-004) (#34).
- Table-driven test conversion across four suites (GO-006) (#40).
- Fuzz targets plus `internal/testharness` fixture and HTTP interceptor
  helpers (#43).
- Property-based test-plan/v1 unit coverage (#45).
- gopenpgp regression test for `proton_list_address_keys` after the
  `v2.10.0-proton` -> `v2.10.0` swap (#50).
- CI: `gofmt` enforcement step (GO-005) (#36).
- GitHub Pro baseline — branch protections and required-check configuration
  (#46).
- Cassette-based integration tests against real Proton API responses
  (replayed offline in CI). Recordings live under
  `testdata/cassettes/`; refresh with `make record SCENARIO=<name>` (#62).
- `make coverage-check` enforces ≥90% weighted aggregate statement
  coverage and a ≥75% per-package floor across included packages (#62).
- `cmd/record-cassettes/` — maintainer-only recording tool, gated by
  `//go:build recording` (#62).
- `cmd/testvcr-lint/` — scrub-leak scanner for committed cassettes,
  wired into `prek` via `.pre-commit-config.yaml` (#62).
- `CI_REQUIRE_CASSETTES=1` env switch flips `testvcr.New` from
  skip-on-missing to fatal failure for opt-in CI enforcement (#66).
- `internal/testvcr`: package-doc + inline comments covering the
  `resolveStackDepth` budget, `opaqueIDSegment` tolerance contract,
  and the shared-cassette pattern used by paired tests (#67).

### Changed
- Go toolchain directive bumped 1.26.2 -> 1.26.3 (#41).
- Dependency bump: `github.com/modelcontextprotocol/go-sdk` 1.5.0 -> 1.6.0
  (#47).
- `cmd/`: documented the bare-goroutine vs `errgroup` choice (GO-013) (#39).
- GitHub URLs across docs updated after `millsmillsymills` -> `millsymills-com`
  org rename (#54).
- `session.New` now accepts `session.WithTransport(http.RoundTripper)`
  to inject a transport shared between `proton.Manager` and the inner
  resty client (#62).
- `testharness.Boot` renamed to `testharness.BootDevServer`. The new
  `testharness.BootWithCassette` replays cassettes against the same
  in-memory MCP transport (#62).
- `cmd/testvcr-lint/` auto-discovers `testdata/cassettes/` roots from
  the working tree (pruning `.git`, `vendor`, `node_modules`); replaces
  the prior hardcoded root list (#66).

### Fixed
- `proterr`: expose `ErrToMCP` alias for the canonical mapping name (PROTO-010)
  (#38, #42).
- `log`: redact `key` field names in addition to the existing redaction set
  (PROTO-012) (#35, #42).
- `protonraw`: wrap returned errors with op context (GO-009) (#37).

### Security
- Pin `govulncheck` install version (no more `@latest`); landed at v1.1.4 (#49)
  and bumped to v1.3.0 (#53).
- Advanced-setup CodeQL workflow for deeper static analysis (#44).
- CI: `go mod verify` step runs before vet/build to catch `go.sum` drift (#48).

## [1.0.0] - 2026-05-05

> No git tag or GitHub release has been cut for `v1.0.0` yet. This block
> records the contents of the v1 surface as it was at the time the changelog
> was first introduced (PR #32, 2026-05-05). A future maintainer can either
> retroactively tag the appropriate commit or roll the contents below into a
> later release when one is cut.

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

[Unreleased]: https://github.com/millsymills-com/protonmail-mcp/commits/main


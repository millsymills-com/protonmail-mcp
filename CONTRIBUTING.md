# Contributing

Thanks for taking the time to contribute. This repo follows a simple workflow.

## Reporting issues

- Bugs and feature requests: open a GitHub issue with reproduction steps,
  expected behavior, observed behavior, and version/commit.
- Security vulnerabilities: do **not** file a public issue. Use GitHub's
  [private vulnerability reporting](https://github.com/millsymills-com/protonmail-mcp/security/advisories/new).
  See [SECURITY.md](SECURITY.md).

## Development setup

Requires Go 1.26+ (the toolchain is auto-bumped by `go-proton-api` master).

```sh
git clone https://github.com/millsymills-com/protonmail-mcp.git
cd protonmail-mcp
go build ./...
go test ./... -race
```

The `go.mod` `replace` directive routes `go-resty/resty/v2` to ProtonMail's
fork. Do not remove it.

## Standards

This repo is graded against the canonical MCP standards in
[`consistency-check/docs/standards/`](https://github.com/millsmillsymills/consistency-check/tree/main/docs/standards).
Re-run the audit before opening a PR:

```sh
cd ~/Desktop/Projects/consistency-check
uv run consistency-check audit --repo protonmail-mcp
```

## Pull requests

- One logical change per PR. Keep diffs reviewable.
- Run `go vet ./...`, `gofmt -d`, and `go test ./... -race` before pushing.
- Update `CHANGELOG.md` under `## [Unreleased]`.
- PR description: what changes and why; link the related issue.
- Commits: imperative mood, ≤72-char subject line.

## Code style

- Standard `gofmt`. No bikeshedding.
- Wrap errors with `fmt.Errorf("op: %w", err)`.
- Tests prefer the table-driven pattern (`tests := []struct{...}{}`).
- No `panic` in `internal/`; return errors.
- Logging via `log/slog` only (use `internal/log`, which auto-redacts
  credential-bearing field names).

## Release process

Tagged releases follow [Semantic Versioning](https://semver.org). Maintainers
cut releases by tagging `vX.Y.Z` after updating `CHANGELOG.md`.

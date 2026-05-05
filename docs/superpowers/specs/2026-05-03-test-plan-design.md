# Test plan: raise project maturity

**Date:** 2026-05-03
**Status:** Design — pending review
**Scope:** `github.com/millsmillsymills/protonmail-mcp` (v1, 23 tools)

## Context

Today's test surface:

- 35% total coverage. `internal/tools` 19%, `internal/session` 52%, `internal/protonraw` 79%, `internal/server` 0%.
- `integration/integration_test.go` smokes 6 read tools with no value assertions.
- 11 write tools have zero automated coverage; only the manual `docs/testing-checklist.md` exercises them, against a real Proton account.
- No protocol-surface tests (MCP catalog, schemas, write-flag gating).
- No error-path coverage. Token-rotation regression (`5a99b02`) has no guard test.
- No fuzz, no property tests, no `golangci-lint`, no `govulncheck`.

Goal: ship every functional path through automated tests on every PR using the in-process `go-proton-api` dev server, while keeping CI total runtime under 60s.

## Decisions captured during brainstorming

| # | Question | Decision |
|---|---|---|
| Q1 | What does "live" mean? | Dev server only (in-process `go-proton-api`, real HTTP, no internet). No real Proton account in CI. |
| Q2 | Write-tool coverage scope? | All 11 writes covered. Recorded fixtures replace dev server for paths where dev-server semantics diverge from production. |
| Q3 | Failure-mode depth? | Hand-rolled fakes returning canned `proterr` JSON for every classified code, plus token-rotation regression test for `5a99b02`. |
| Q4 | CI gates? | Existing (vet/build/race/integration) + `golangci-lint` + `govulncheck`. No coverage gate, no zizmor, no mutation testing. |
| Q5 | MCP protocol surface? | Schema-shape validation + golden catalog file (description audit signal in PR diff). |
| Q6 | Property/fuzz scope? | Fuzz `proterr` mapper, fuzz `protonraw` JSON decoders, property test on `filterSensitiveHeaders`. |

Manual checklist (`docs/testing-checklist.md`) is retained for pre-release validation against a real Proton account; it stays out of CI.

## Architecture: three-layer test pyramid

| Layer | Location | Boots harness? | Build tag | Runtime |
|---|---|---|---|---|
| Unit | `internal/<pkg>/*_test.go` | no | none | <1s/pkg |
| Harness-integration | `internal/tools/*_integration_test.go` + `integration/` | yes (in-process `go-proton-api`) | `//go:build integration` | ~5–10s total |
| Protocol contract | `internal/server/contract_test.go` | partial (registry only, no live calls) | none | <1s |

### Layer rules

- **Unit:** no `net/http`, no goroutines, no `testharness` import. Pure functions and table-driven cases. Fuzz and property targets live here.
- **Harness-integration:** dev server only. No outbound traffic to `mail.proton.me`. Writes always exercised — env flag flipped per-test via `t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")`.
- **Protocol-contract:** registers tools against a bare `mcp.Server`, lists them, marshals deterministically, compares to a checked-in golden file at `internal/server/testdata/catalog_*.json`.

Recorded fixtures live at `internal/protonraw/testdata/<endpoint>/<case>.json`, format:

```json
{
  "request":  {"method": "POST", "path": "/...", "body": {...}},
  "response": {"status": 200, "body": {...}}
}
```

An `httptest.Server`-backed replayer asserts the request shape, returns the canned response. Mismatch on request fails the test (catches request-shape regressions).

## Layer 1: Unit tests

Target: pure functions, near-100% coverage, deterministic, fast.

### New table-driven tests

| File | Functions under test | Cases |
|---|---|---|
| `internal/tools/addresses_test.go` (extend) | `toAddressDTO` | nil-safe; empty key list; status enum mapping; primary flag |
| `internal/tools/domains_test.go` (new) | `toDomainDTO`, DNS-record extraction | verified/unverified; missing MX/SPF/DKIM/DMARC rows; mixed-case |
| `internal/tools/settings_test.go` (new) | `toMailSettingsDTO`, `toCoreSettingsDTO`, `boolToSettingsBool` | 0/1/2 enum round-trip; telemetry + crash-report flag combos |
| `internal/tools/tools_test.go` (new) | `requireField`, `failure`, `clientOrFail` | missing field; empty string; nil session; valid path |
| `internal/tools/messages_test.go` (extend) | `formatAddress`, `formatAddresses`, `filterSensitiveHeaders` | empty; multi-recipient |
| `internal/proterr/proterr_test.go` (extend) | error mapper | every Proton code → typed error; unknown code fallback |

### Fuzz targets

```go
func FuzzProterrMapping(f *testing.F)         // (status int, body []byte) -> never panic, classified
func FuzzProtonrawDecodeAddress(f *testing.F) // []byte -> never panic
func FuzzProtonrawDecodeDomain(f *testing.F)
func FuzzProtonrawDecodeCatchall(f *testing.F)
```

Seed corpus: real captures from harness runs (saved to `testdata/fuzz/<target>/`), plus malformed/truncated/empty.

### Property test

`filterSensitiveHeaders` — generate random header maps including case and whitespace variants of `authorization`, `cookie`, `x-pm-uid`, `x-pm-appversion`, etc. Assert the deny-list never appears in output regardless of casing. Use deterministic seed for reproducibility.

### Run cadence

- Per PR: `go test -run=^$ -fuzz=<each> -fuzztime=10s` (~40s for four targets) — smoke for new crashes.
- Nightly: `-fuzztime=5m` — wider exploration; uploads corpus diffs as workflow artifact.

## Layer 2: Harness-integration

Goal: every tool exercised through the MCP transport against the real protocol stack.

### Per-tool matrix

```
TestTools/<tool_name>/happy
TestTools/<tool_name>/invalid_input    // missing/empty/wrong-type required field
TestTools/<tool_name>/error_path       // canned proterr from fixture/fake (where applicable)
```

23 tools × {happy, invalid_input} = 46 baseline cases, plus targeted error-path subtests on tools that surface classified errors.

### Reads (12)

| Tool | Strategy |
|---|---|
| `whoami`, `session_status`, `list_addresses`, `get_address` | dev server; assert email, storage, status, primary flag |
| `get_mail_settings`, `get_core_settings` | dev server; assert defaults |
| `list_address_keys` | dev server (creates real keys at user-create); assert fingerprint shape and armored PEM round-trip via `gopenpgp/v2` |
| `list_custom_domains`, `get_custom_domain` | recorded fixture (dev server is thin here); assert DNS records present |
| `get_catchall` | recorded fixture |
| `search_messages`, `get_message` | new harness method `SeedMessage(t *testing.T, raw []byte)` to inject test message; assert filtered headers absent from output |

### Writes (11) — `PROTONMAIL_MCP_ENABLE_WRITES=1` per subtest

| Tool | Strategy |
|---|---|
| `create_address`, `update_address`, `set_address_status`, `delete_address` | dev server full lifecycle; verify state via subsequent read |
| `update_mail_settings`, `update_core_settings` | dev server; write→read round-trip |
| `add_custom_domain`, `verify_custom_domain`, `remove_custom_domain` | recorded fixtures (verification state machine diverges) |
| `set_catchall`, `disable_catchall` | recorded fixtures |

### Token-rotation regression

`TestSessionRefreshPersists` lives in `internal/session/session_test.go` (the rotation-persistence behavior is the session package's responsibility) but uses the harness for end-to-end realism.

1. Boot harness with mock keychain.
2. Force a 401 response once on the next API call via the fixture interceptor (see "Fixture interceptor hook" below).
3. Issue a tool call.
4. Assert: rotated `AccessToken` and `RefreshToken` written to mock keychain match what the dev server returned on the refresh round-trip.

Regression for commit `5a99b02` (cold-start refresh dropped rotated tokens).

### Fixture interceptor hook

`testharness.Boot` is extended with an optional `WithInterceptor(fn func(*http.Request) *http.Response) Option` parameter. When set, the harness wraps the dev server's HTTP handler in a `httptest.Server` that consults `fn` first; if `fn` returns non-nil, that response is served, otherwise the call falls through to the dev server. Used by the token-rotation test to inject a one-shot 401, and by recorded-fixture tests to short-circuit endpoints the dev server doesn't implement.

### Write-flag gating

`TestWriteFlagGating`:
- Boot harness with flag off → assert `mcp.ListTools` returns exactly the 12 read names.
- Boot harness with flag on → assert exactly the 23 expected names. Set is exhaustive (extra or missing names fail).

Asserts on tool *registration*, not handler runtime checks.

### File layout

- `internal/tools/addresses_integration_test.go`
- `internal/tools/domains_integration_test.go`
- `internal/tools/settings_integration_test.go`
- `internal/tools/messages_integration_test.go`
- `internal/tools/keys_integration_test.go`
- `internal/tools/identity_integration_test.go`

`integration/integration_test.go` is reduced to a cross-cutting smoke (one read + one write end-to-end via real MCP catalog) — its current overlap with per-tool harness tests is removed.

### Cleanup

Each test boots a fresh harness (already the pattern). No shared user state. `t.Cleanup` already wired in `testharness.Boot`.

## Layer 3: Protocol contract

Catches MCP-surface regressions before they reach clients.

### Catalog golden

`internal/server/contract_test.go`:

```
TestCatalog_Reads_FlagOff -> golden internal/server/testdata/catalog_reads.json
TestCatalog_All_FlagOn    -> golden internal/server/testdata/catalog_all.json
```

Boots a bare `mcp.Server`, registers tools, lists them, marshals `(name, description, inputSchema, outputSchema)` deterministically (sorted by name), compares to golden. Update via `-update` flag (`go test -update ./internal/server/...`). Diff in PR signals a description or schema change for human review.

### Schema validity

For every registered tool:

- `inputSchema` parses as JSON Schema draft-07 (use `github.com/santhosh-tekuri/jsonschema/v5`).
- `outputSchema` parses where declared.
- Every property name in `required[]` exists in `properties`.
- No tool name collision across reads + writes.

### Description quality lints (advisory, non-blocking)

- Description ≥ 30 chars.
- No trailing whitespace.
- Mentions side effects for write tools (regex match on `creates|updates|deletes|disables|removes|sets`).

Failure surfaces as warning in test output, does not fail the build — keeps it useful without bikeshedding.

## CI + tooling

Additions to `.github/workflows/ci.yml`:

```yaml
- name: golangci-lint
  uses: golangci/golangci-lint-action@<sha>  # version pinned during implementation per global GH Actions standard
  with: { version: v1.62 }

- name: govulncheck
  run: |
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./...

- name: fuzz (smoke)
  run: |
    for t in FuzzProterrMapping FuzzProtonrawDecodeAddress FuzzProtonrawDecodeDomain FuzzProtonrawDecodeCatchall; do
      go test -run=^$ -fuzz=$t -fuzztime=10s ./...
    done
```

New `.golangci.yml` enabling: `gofmt`, `govet`, `errcheck`, `staticcheck`, `unused`, `gosec`, `ineffassign`, `misspell`, `unparam`, `gocritic`, `revive` (line length 100, function length 100, cyclomatic complexity 8).

Existing race + integration jobs unchanged.

New `.github/workflows/nightly.yml` (cron `0 7 * * *` UTC + `workflow_dispatch`): same four fuzz targets at `-fuzztime=5m` each, uploads `testdata/fuzz/<target>/` as workflow artifact for triage.

## Test data and hygiene

- All harness tests use `keyring.MockInit()` (already enforced in `testharness.Boot`) — real keychain is never touched.
- Dev server creds are always `user@example.test` / `hunter2` — no real Proton creds in repo, fixtures, or CI.
- Recorded fixtures contain no real tokens, keys, or signatures. Capture pipeline scrubs `Authorization`, `x-pm-uid`, `Set-Cookie`, anything matching `[A-Za-z0-9]{40,}` to `<redacted-token>`.
- New helper `scripts/capture-fixture.sh <tool> <case>` boots the harness, runs the tool, saves the request/response pair, runs the scrubber.
- `.gitignore` adds `*.fuzz-corpus.local/` for local exploratory corpora that should never be committed.
- `prek run` (manual pre-commit) adds a check: no string matching `/Bearer [A-Za-z0-9]+/` or `/x-pm-uid: [a-f0-9]{20,}/i` in `testdata/`.

## Acceptance criteria — maturity rubric

| Dimension | Today | Target |
|---|---|---|
| `internal/tools` coverage | 19% | 85% |
| `internal/session` coverage | 52% | 80% |
| `internal/protonraw` coverage | 79% | 90% |
| `internal/server` coverage | 0% | 75% |
| Tools with happy-path integration test | 6/23 | 23/23 |
| Tools with invalid-input test | 0/23 | 23/23 |
| Tool write-flag gating tested | no | yes |
| MCP catalog golden | no | yes |
| Fuzz targets | 0 | 4 |
| Property tests | 0 | 1 (header filter) |
| Token-rotation regression | no | yes |
| `golangci-lint` clean | no | yes |
| `govulncheck` clean | no | yes |
| Total PR test runtime | ~4s | <60s |
| Total nightly test runtime | n/a | <10 min |

### Gating policy

All green required to merge. Coverage numbers are targets, not gates (per Q4). `golangci-lint` and `govulncheck` block. Description quality lints (Layer 3) are advisory.

## Out of scope (explicitly deferred)

- Real Proton account testing in CI (Q1: dev server only). Manual checklist remains the live-account safety net.
- Coverage gate (Q4: targets not gates).
- Mutation testing (Q4: not now).
- `zizmor` and Dependabot (Q4: not now).
- Invalid-input matrix at protocol layer (Q5: shape + golden only; per-tool invalid input is covered in Layer 2).
- Fuzz on `session.parseAuthHeader` and TOTP parsing (Q6: scope limited to `proterr` + `protonraw` + header filter).

## Files changed (summary)

**New:**
- `internal/tools/domains_test.go`
- `internal/tools/settings_test.go`
- `internal/tools/tools_test.go`
- `internal/tools/{addresses,domains,settings,messages,keys,identity}_integration_test.go`
- `internal/proterr/fuzz_test.go`
- `internal/protonraw/fuzz_test.go`
- `internal/protonraw/testdata/<endpoint>/<case>.json` (multiple)
- `internal/server/contract_test.go`
- `internal/server/testdata/catalog_reads.json`
- `internal/server/testdata/catalog_all.json`
- `.golangci.yml`
- `.github/workflows/nightly.yml`
- `scripts/capture-fixture.sh`

**Modified:**
- `internal/tools/addresses_test.go`, `internal/tools/messages_test.go` (extended)
- `internal/proterr/proterr_test.go` (extended)
- `internal/session/session_test.go` (token-rotation regression)
- `internal/testharness/harness.go` (`SeedMessage` helper, fixture interceptor hook)
- `integration/integration_test.go` (reduced to cross-cutting smoke)
- `.github/workflows/ci.yml` (lint + vuln + fuzz smoke)
- `.gitignore`
- `docs/testing-checklist.md` (note: superseded for read paths; retained for live-account verification)

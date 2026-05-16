# Live-Data Cassette Tests + 90% Aggregate Coverage

Status: shipped (as-built record).
Date: 2026-05-12 (original), 2026-05-16 (corrected against landed code).
Supersedes: portions of `2026-05-03-test-plan-design.md` (per-package targets).

> **As-built note.** This file is preserved as the design-time record but has been corrected for factual divergences from the code that landed in #62 / #66 / #67. Where the original design described an approach that did not ship verbatim, the text now matches the shipped code; where a contingency (e.g. HTTP/2 workaround) turned out unnecessary, that is noted inline. The 35-cassette recording phase tracked by #63 has not yet completed — coverage projections in §"Coverage projection" remain targets, not measurements.

## Problem

Today's automated suite tops out at 47.9% weighted statement coverage. Reads, writes, and error paths are validated against the in-process `go-proton-api` dev server, which diverges from production Proton in field ordering, header casing, key serialisation, and the entire custom-domain surface. The 23 registered `mcp.AddTool` invocations include a write-side surface gated behind `PROTONMAIL_MCP_ENABLE_WRITES` (addresses, domains, settings) that has no automated coverage; the only signal is the manual checklist at `docs/testing-checklist.md`. Token-rotation regression `5a99b02` has no guard test. CI cannot detect prod-API drift.

Goal: replace dev-server-backed behaviour tests with replays of real Proton responses ("live data"), bring weighted aggregate statement coverage to ≥90% across non-test code, and keep CI offline + deterministic.

## Goals

- **High fidelity.** Behaviour tests assert against the byte shape of real Proton responses, not dev-server approximations.
- **Offline CI.** No network or credentials required to run `go test ./...`.
- **Deterministic replay.** Same cassette, same result, every run.
- **Safe artefacts.** No tokens, key material, real emails, or session UIDs in committed files.
- **Drift detection.** Maintainers can refresh cassettes on demand; staleness is surfaced but not blocking.
- **Coverage gate.** ≥90% weighted aggregate; ≥75% per included package; enforced in CI.

## Non-goals

- Mutation-score gate (informational only).
- Branch-coverage gate (Go statement coverage is the contract).
- Coverage on test scaffolding (`internal/testharness`, `internal/testvcr`, `cmd/record-cassettes`).
- Hitting real Proton in default CI. Only the maintainer recording workflow touches the network.
- Replacing fuzz/property tests. They stay as-is on the dev-server harness.

## Architecture

Two test rigs coexist. The new cassette rig is the default for behaviour tests; the existing dev-server rig is retained only for offline fuzz and property tests.

```
cmd/protonmail-mcp ──┐
                     │
internal/server ─────┤        ┌── proton.Manager  ─ proton.WithTransport(rt)
internal/tools ──────┤── session.Session
internal/session ────┤        └── rawClient (resty) ─ rc.SetTransport(rt)
                     │                                        │
internal/protonraw ──┘                              prod:  http.DefaultTransport
                                                    test:  vcr.RoundTripper(cassette, mode)
```

`session.Session` owns two HTTP clients: `proton.Manager` (go-proton-api, used for endpoints supported by the SDK) and `rawClient` (a `resty.Client` used for `/core/v4/domains` and other endpoints go-proton-api does not cover). Both must share the same transport so a single cassette captures a test run that spans both code paths.

The implementation adds a single optional `http.RoundTripper` to `session.New` (default `nil` → both clients fall back to `http.DefaultTransport`). When set, it is passed to `proton.New` via `proton.WithTransport(rt)` and to `resty.New().SetTransport(rt)` in `newRawClient`. In tests the same `testvcr.New(...)` RoundTripper is supplied to both. No other production code changes shape.

## Components

### `internal/testvcr/` (new package)

- `recorder.go` — thin wrapper around `gopkg.in/dnaeon/go-vcr.v4`. Exports `New(t *testing.T, name string) http.RoundTripper` and `Mode()` (reads `VCR_MODE`, default `replay`). Cassette path resolved relative to caller package directory: `<caller-pkg-dir>/testdata/cassettes/<name>.yaml`.
- `scrub.go` — `SaveHook` pipeline applied before any cassette is written to disk. Pipeline:
  - Header redaction: `Authorization`, `x-pm-uid`, `Cookie`, `Set-Cookie` → `REDACTED`.
  - JSON body walker scrubs 12 known sensitive keys to deterministic placeholders: `AccessToken`, `RefreshToken`, `UID`, `KeySalt`, `PrivateKey`, `Signature`, `Token`, `SrpSession`, `ServerProof`, `ClientProof`, `ClientEphemeral`, `TwoFactorCode` → `REDACTED_<KEY>_<N>` where `<N>` is the running occurrence index per scenario.
  - Identifier rewrite: `RECORD_EMAIL` → `user@example.test`, `RECORD_DOMAIN` → `example.test`, `RECORD_THROWAWAY_DOMAIN` → `throwaway.example.test`.
- `matcher.go` — custom matcher. Method + path (regex-tolerant for opaque IDs) + canonicalised body (sorted JSON, placeholders preserved). SRP login bodies match on `Username` plus presence of `ClientProof`, not the proof value (it is non-deterministic).
- `lint.go` — standalone scanner invoked by `make verify-cassettes`. Regex set: `Bearer [A-Za-z0-9._-]{20,}`, `"AccessToken":\s*"[^R]`, `BEGIN PGP PRIVATE KEY BLOCK`, `BEGIN PGP MESSAGE`, `@protonmail\.|@proton\.me`. Public PGP key blocks (`BEGIN PGP PUBLIC KEY BLOCK`) are deliberately not flagged — fingerprints must remain assertable in `proton_list_address_keys` tests. Any match fails the lint.

### `internal/testharness/cassette.go` (new file)

- `BootWithCassette(t *testing.T, scenarioName string, opts ...Option) *Harness` — same shape as today's `Boot`. Constructs `testvcr.New(t, scenarioName)` once and passes it to `session.New` so both `proton.Manager` and `rawClient` share the same transport. Base URL constant: `https://mail.proton.me/api` (the actual `cassetteBaseURL` in `internal/testharness/cassette.go`). The `scenarioName` argument is the explicit cassette identifier (not derived from `t.Name()`, which contains `/` for subtests). Cassette path: `<caller-pkg-dir>/testdata/cassettes/<scenarioName>.yaml`.
- The existing `WithInterceptor` option is preserved on the cassette harness. Interceptors run before VCR matching, so synthetic responses (e.g. injected 5xx for error-path tests) can stand in for cassette interactions when a recording is impractical to obtain.
- Existing `Boot` renamed `BootDevServer`. Retained for `protonraw` fuzz tests and `internal/tools/headers_property_test.go`.
- `Harness.Call` signature unchanged.

### `<caller-pkg-dir>/testdata/cassettes/<scenarioName>.yaml`

- One YAML file per `scenarioName` (the explicit argument to `BootWithCassette`). Multi-step flows (login → rotate → call) stored as ordered interactions in the same file.
- Cassette metadata: `recorded_at`, `go_proton_api_version`, `mcp_version`, `scenario`. Populated by the record tool via a sidecar `<cassettePath>.meta.yaml` file (see `recorder.go:writeMeta`). The cassette `extensions` alternative was considered and rejected — sidecars keep the YAML cassette format identical to a plain go-vcr capture.

### `cmd/record-cassettes/` (new, `//go:build recording`)

- Entrypoint `main.go` dispatches to scenario functions by name: `record-cassettes <scenario>`.
- Reads `.env.record` (gitignored) for `RECORD_EMAIL`, `RECORD_PASSWORD`, `RECORD_TOTP_SECRET`, `RECORD_DOMAIN`.
- `scenarios/` directory: one file per scenario family. Initial set: `read_tools.go`, `write_address.go`, `custom_domain_lifecycle.go`, `token_rotation.go`, `error_envelopes.go`, `cli_flows.go`, `server_boot.go`.
- Each scenario follows setup → exercise → teardown. Teardown deletes anything created. Cassette is only written if teardown succeeds.

### `cmd/protonmail-mcp/run.go` (refactor)

- Extract `main()` body into `run(ctx context.Context, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer, transport http.RoundTripper) int`. `main` becomes `os.Exit(run(...))`.
- CLI subcommands (`login`, `logout`, `status`, server default) read `PROTONMAIL_MCP_API_URL` from `env`; production omits it (defaults to `https://mail.proton.me/api`); tests inject the value plus cassette wiring through the trailing `transport` parameter.
- Tests live in `cmd/protonmail-mcp/*_test.go` and call `run` directly.

### Makefile targets

- `make record SCENARIO=<name>` → `go run -tags recording ./cmd/record-cassettes $(SCENARIO)`.
- `make verify-cassettes` → runs `go run ./cmd/testvcr-lint` over discovered `testdata/cassettes/**` roots and warns on stale metadata (>90 days or `go_proton_api_version` mismatch with `go.mod`).
- `make coverage` → `go test -coverprofile=cov.out -coverpkg=$(INCLUDED) ./...`.
- `make coverage-check` → parses `go tool cover -func=cov.out`, computes weighted aggregate, fails if <90% or any included package <75%.

### Dependencies

- Add `gopkg.in/dnaeon/go-vcr.v4` as a test-only dependency.
- No other additions. No production-binary impact.

## Data flow

**Replay (CI + local default, `VCR_MODE=replay`):**

```
test ─> tools.Handler ─> session.Client ─> http.Client
                                              │
                                              └─> vcr.RoundTripper ─> cassette.yaml
                                                                          │
                                                                          └─> matched interaction
                                                                                     │
                                                                          ┌──────────┘
                                                              scrubbed response bytes
                                              ◀──────────────┘
```

- Match order per interaction: method → path → normalised body.
- Cassette miss is a hard failure (`testvcr.ErrMissingInteraction`). No fallback to network.
- Interactions consumed in order within a cassette to preserve flow semantics (e.g. token rotation: original call → 401 → refresh → retried call).

**Record (`VCR_MODE=record`, only inside `cmd/record-cassettes`):**

```
scenario fn ─> BootWithCassette ─> session.Login(real creds) ─> real Proton API
                                              │
                                              └─> vcr.RoundTripper records req/res
                                                                          │
                                                  SaveHook scrub pipeline ┘
                                                                          │
                                                                          ▼
                                                                cassette.yaml (written)
```

- TOTP code derived from `RECORD_TOTP_SECRET` at record time; scrubbed to placeholder before write.
- Scenarios are idempotent — they delete any resource they create.

**CLI test path:**

```
*_test.go ─> run(ctx, args, env, ..., transport) ─> session.New(BaseURL=env.PROTONMAIL_MCP_API_URL)
                                              │
                                              └─ transport = testvcr.New(t, cassette)
```

## Error handling

### Test-time failures

- Cassette missing → `t.Fatalf("cassette not found: %s", path)`.
- Interaction miss → `t.Fatalf` with method, URL, request body, and a dump of available matchers in the cassette.
- Scrub leak detected at record time → abort write, print offending JSON key path.
- `VCR_MODE=record` while any of `CI`, `GITHUB_ACTIONS`, `BUILDKITE`, or `CIRCLECI` is set → abort. Prevents accidental network in CI.

### Production error paths covered by cassettes

One cassette per case, asserting the structured-output envelope:

- `proton/auth_required` — 401 on a protected endpoint without bearer.
- `proton/captcha` — 422 carrying `HumanVerificationToken`.
- `proton/rate_limited` — 429 with `Retry-After`.
- `proton/not_found` — 404 on `proton_get_message`, `proton_get_address`, `proton_get_custom_domain`.
- `proton/permission_denied` — 403 from real Proton on an attempted write the server rejects (distinct from the local `PROTONMAIL_MCP_ENABLE_WRITES` gate, which short-circuits before HTTP).
- `proton/conflict` — 409 on duplicate `proton_add_custom_domain`.
- `proton/validation` — 422 from `proton_create_address` with an invalid local part.
- Token rotation — ordered interactions: call → 401 → refresh → retried call → 200.
- `proton/upstream` — 502 then 503 from upstream.

### Record-time failures

- Capture aborts on the first error not in the scenario's expected set. Partial cassettes are deleted.
- Resource cleanup runs in `defer`. Teardown failure fails the scenario and blocks commit.

### Refresh drift

- `make verify-cassettes` warns when `recorded_at` is older than 90 days or `go_proton_api_version` mismatches `go.mod`. Warn-only by default. (A `STRICT=1` upgrade-to-error mode was specified but is not yet implemented in `cmd/testvcr-lint`; tracked in residual spec drift, see followup issue.)

## Testing strategy

| Layer | Tool | Scope | Cassettes |
|---|---|---|---|
| Unit | `go test` | Pure functions in `proterr`, `protonraw`, `log`, `keychain`, headers parser | no |
| Property | `rapid` (existing) | Parser + header invariants | no |
| Fuzz | `go test -fuzz` | `protonraw` + `proterr` (existing) | no |
| Cassette integration | `BootWithCassette` | Every tool, happy + dominant error; token rotation; SRP login | yes |
| CLI | `cmd/protonmail-mcp/*_test.go` | `run()` for login/logout/status with cassette transport | yes |
| Boot | `internal/server/server_test.go` | `server.New` + `Register` + first tool dispatch via cassette | yes |

**Cassette inventory (target: 35 recording scenarios scaffolded in #62; size budget ~2.4MB committed after scrub):**

- `tools/`: read + write happy + dominant error across the 23 registered tools, plus shared cassettes for paired tests.
- `session/`: login (no-2FA), login (2FA), token rotation, logout, refresh-revoked.
- `cmd/`: login, logout, status (logged-in), status (logged-out).
- `server/`: boot + dispatch.

The 35 scenarios are enumerated in `cmd/record-cassettes/scenarios/` (see `read_tools.go`, `write_addresses.go`, `write_settings.go`, `cli_flows.go`, `server_boot.go`, `token_rotation.go`, `logout_invalidates.go`, `refresh_revoked.go`, `custom_domain_lifecycle.go`, `error_envelopes.go`). Only `internal/testharness/testdata/cassettes/smoke.yaml` is committed today; the full 35 land via #63.

**Naming convention:**

- `testdata/cassettes/tools/proton_list_addresses_happy.yaml`
- `testdata/cassettes/tools/proton_add_custom_domain_conflict.yaml`

**Existing tests:**

- `internal/tools/*_test.go` migrate from `Boot` to `BootWithCassette` for behaviour assertions. Envelope assertions strengthened against real-Proton shapes.
- `internal/tools/headers_property_test.go` stays on `BootDevServer` — offline, property-based, shape-agnostic.
- `integration/integration_test.go` retired. Its 7 read assertions are absorbed into cassette tests with stricter envelope checks. The `//go:build integration` tag is removed; everything runs in the default `go test ./...`.
- `protonraw/fuzz_test.go`, `proterr/fuzz_test.go` untouched.

**Mutation testing (stretch, not gated):**

- `go-mutesting` config under `tools/`. Nightly job, results posted to a tracking issue. Not in the critical path; informational only.

**Verification commands:**

```
make test            # full suite, replay-only
make test-race       # -race
make coverage        # produces cov.out
make coverage-check  # asserts weighted aggregate >=90% and per-pkg floor >=75%
make verify-cassettes
make record SCENARIO=<name>   # maintainer only
```

## Coverage targets

### Inclusion set (denominator for the 90% headline)

- `cmd/protonmail-mcp`
- `internal/server`
- `internal/tools`
- `internal/session`
- `internal/protonraw`
- `internal/proterr`
- `internal/log`
- `internal/keychain`

### Exclusion set (zero weight)

- `internal/testharness`
- `internal/testvcr`
- `cmd/record-cassettes`
- Files behind `//go:build recording`

### Per-package targets

| Package | Current | Target | Drivers |
|---|---|---|---|
| `cmd/protonmail-mcp` | 0% | 85% | `run()` refactor + cassette-backed login/logout/status |
| `internal/server` | 0% | 90% | boot test + cassette dispatch |
| `internal/tools` | 32% | 92% | cassette tests (happy + error) for the 23 registered tools |
| `internal/session` | 52% | 90% | login/refresh/rotation cassettes + interceptor synthetic errors |
| `internal/protonraw` | 82% | 95% | cassette-backed request/response coverage + existing fuzz |
| `internal/proterr` | 94% | 96% | add error-code-not-mapped path |
| `internal/log` | 82% | 92% | structured-log assertions at every log site |
| `internal/keychain` | 70% | 90% | macOS Keychain stub + in-mem keyring branch coverage |

### Weighted aggregate projection

Approximate statement counts (rough estimate, to be measured in CI): server ~80, tools ~1100, session ~350, protonraw ~600, proterr ~220, log ~250, keychain ~180, cmd ~240. Total ≈3020. Per-package covered (count × target): 72 + 1012 + 315 + 570 + 211 + 230 + 162 + 204 ≈ 2776. Aggregate ≈91.9%, ~1.9 points above the 90% floor. Buffer is thin; actual statement counts must be measured before locking the CI gate so the headline doesn't depend on the rough estimates above.

### CI gate

- `make coverage-check` parses `go tool cover -func=cov.out`, computes the weighted aggregate over the inclusion set, and fails the build if aggregate <90% **or** any included package <75%.
- Runs after `go test -coverprofile=cov.out -coverpkg=$(inclusion_list) ./...`.

## Risks + mitigations

- **SRP body non-determinism.** Login bodies contain random `ClientProof` per attempt. Mitigated by the custom matcher ignoring proof values; matched on `Username` + presence.
- **HTTP/2 + go-vcr compatibility.** `go-proton-api` may negotiate HTTP/2 via the resty default transport. *As-built outcome:* the smoke cassette (`internal/testharness/testdata/cassettes/smoke.yaml`) records and replays cleanly without any `ForceAttemptHTTP2: false` workaround. No transport adjustment shipped.
- **Real-account churn.** Recording requires creating/destroying real addresses + domains. Scoped to a maintainer-owned throwaway domain; scenarios are idempotent. Manual checklist still applies pre-release.
- **Cassette diff review burden.** Each refresh PR can be noisy. Mitigated by deterministic scrub placeholders (stable across reruns when payloads are unchanged) and a metadata-only diff mode in `make verify-cassettes`.
- **Secret leak in committed cassette.** Defence in depth: scrub pipeline (record-time) + `testvcr.lint` scanner invoked by `make verify-cassettes` + a new `.pre-commit-config.yaml` (this repo does not yet have one; adding it is in scope) with a `make verify-cassettes` hook so cassettes are linted before every commit + gitignored `.env.record`.
- **Drift between cassette and real Proton.** Default warn-only on stale metadata; maintainers re-run scenarios on `go-proton-api` bumps and before release.

## Out of scope (deferred)

- Real-Proton CI job (nightly drift detection). Possible follow-on once cassette workflow is stable.
- Mutation-score CI gate.
- Replacement of manual `docs/testing-checklist.md` — it remains the pre-release sign-off.

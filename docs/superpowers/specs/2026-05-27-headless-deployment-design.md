# Headless Linux deployment — design

Tracks GitHub issue #108. Lets `protonmail-mcp` run as a long-lived, headless
loopback service on Linux (no macOS Keychain, no D-Bus Secret Service), serving
SSE so `interceptor-dashboard` can connect as an MCP client.

## Goals

- Serve over loopback SSE on a configurable host/port; **stdio stays the default**.
- Persist and refresh credentials without macOS Keychain or Linux D-Bus Secret Service.
- Document a one-time headless bootstrap → a persisted session usable by a
  systemd unit running as a dedicated `--no-create-home` system user.
- Stay unattended across refresh-token revocation (auto-relogin), within the
  CAPTCHA ceiling described below.

## Non-goals

- Dashboard-side source adapters (`proton_mail`, `proton_cal`).
- Calendar tools.
- Multi-account / multi-tenant serving (single account, as today).

## Interface surface (the committed contract)

All new env vars are backward-compatible: unset → today's stdio + Keychain
behavior, unchanged.

| Var | Values | Default | Scope |
|---|---|---|---|
| `PROTONMAIL_MCP_TRANSPORT` | `stdio` \| `sse` | `stdio` | always |
| `PROTONMAIL_MCP_HOST` | listen address | `127.0.0.1` | sse only |
| `PROTONMAIL_MCP_PORT` | listen port | **required when sse** (fail fast if unset) | sse only |
| `PROTONMAIL_MCP_CREDENTIAL_BACKEND` | `keychain` \| `file` | `keychain` | always |
| `PROTONMAIL_MCP_STATE_DIR` | credentials directory | `$STATE_DIRECTORY` → `$XDG_STATE_HOME/protonmail-mcp` → `~/.local/state/protonmail-mcp` | file only |

Decisions:

- **Backend selection is explicit**, not auto-detect-Secret-Service. No surprise
  fallbacks; predictable in CI and on the box.
- **No silent port default.** Binding a network listener is a deliberate act;
  `sse` without `PROTONMAIL_MCP_PORT` is a startup error, not a guess. (Earlier
  drafts defaulted to the consumer's reserved `8770`; rejected — that's
  deployment-specific, not a tool default.)
- Invalid `TRANSPORT`/`BACKEND` values fail fast at startup with the offending
  value and the allowed set.

## Architecture: four tracer slices

Each slice is independently shippable and unit-testable. A is independent of the
rest; B is independent of A; C and D depend on B. Build order: **A ∥ B → C → D**.

### Slice A — transport seam

`internal/server/server.go` currently hardcodes `srv.Run(ctx, &mcp.StdioTransport{})`
(the existing `transport http.RoundTripper` param is the *Proton API*
round-tripper — unrelated to the MCP transport).

- Resolve the transport from env into a small `transportConfig{kind, host, port}`.
- `stdio` (default): unchanged — `srv.Run(ctx, &mcp.StdioTransport{})`.
- `sse`: build `mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.SSEOptions{})`,
  mount it at `/sse` on an `http.Server` bound to `host:port`. One `*mcp.Server`
  is reused across sessions — `SSEHandler.ServeHTTP` calls `getServer(req)` then
  `server.Connect` per GET (verified against go-sdk v1.6.0). We do **not** call
  `srv.Run` in SSE mode. On `ctx` cancel, `http.Server.Shutdown` for a graceful stop.
- `SSEOptions{}` leaves DNS-rebinding / localhost protection **on** (default),
  which is correct for a loopback listener.

**SSE endpoint shape (compatibility note).** go-sdk's `SSEHandler` serves both the
GET event stream and the POST messages on the *same* mounted path, advertising
the per-session message endpoint via the SSE `endpoint` event as
`/sse?sessionid=<id>` (query param `sessionid`). This differs from FastMCP's
layout (separate `/messages/` path, `session_id`). A spec-compliant MCP SSE
client reads the advertised `endpoint` event and works unmodified; only a client
that hardcodes FastMCP's `/messages` path would break. The dashboard adapter
(currently a stub) must follow the `endpoint` event — standard MCP SSE behavior.

**Tests.** Env parsing: defaults; `sse` without port → error; invalid kind/port
→ error. Integration: start `sse` on an ephemeral port, connect a real MCP
client over SSE, list tools, shut down via ctx cancel.

### Slice B — file credential backend

New package `internal/credfile` implementing the same five-method surface
`session.keychainStore` requires (`SaveCreds`, `LoadCreds`, `SaveSession`,
`LoadSession`, `Clear`) over `keychain.Creds` / `keychain.Session`.

- Storage: single JSON document at `<state_dir>/credentials.json`.
- Permissions: directory `0700`, file `0600`, both owned by the service user.
- Writes are atomic: write a temp file in the same dir, `fsync`, `rename` over
  the target — no torn writes on crash.
- **Absent file** → `LoadSession`/`LoadCreds` return an error. `session.go:201`
  already collapses any `LoadSession` error to `proterr.ErrNoSession` ("run
  `protonmail-mcp login`"), so cold start behaves identically to Keychain.
- Optional hardening (non-core, flagged): a *corrupt* or *permission-denied*
  file currently masks as `ErrNoSession` (same pre-existing limitation as
  Keychain). Distinguishing absent (→ logged-out) from unreadable (→ surfaced
  error) would require a change at `session.go:201` that also touches the
  Keychain path; left out of core scope.

**Tests.** Creds + session round-trip; absent file → not-found parity (maps to
`ErrNoSession`); permission bits asserted (`0700`/`0600`); atomic write leaves no
partial file on simulated mid-write failure; `Clear` removes the file (and
tolerates already-absent).

### Slice C — backend selection wiring + bootstrap + systemd

- Promote the currently-unexported `session.keychainStore` interface to an
  exported `session.Store` (same five methods) so a shared selector can name and
  return it. (`session.New`'s parameter type changes from the unexported alias to
  `session.Store`; existing concrete callers are unaffected.)
- Add `session.SelectStore(env) session.Store`, choosing `keychain.New()` vs
  `credfile.New(stateDir)` from `PROTONMAIL_MCP_CREDENTIAL_BACKEND`. It lives in
  `session` (imports `keychain` already; adds `credfile` — no cycle, since neither
  `keychain` nor `credfile` imports `session`). Wire it into the four production
  construction sites: `internal/server/server.go`, `cmd/protonmail-mcp/login.go`,
  `logout.go`, `status.go`. (Test/record-cassette sites keep `keychain.New()`.)
- With `backend=file`, the **existing interactive `login`** writes creds +
  session to the file store — no new login flow for bootstrap itself.
- `status` reports the active backend and session validity (bootstrap
  verification).
- **Docs** (`docs/` + README): a systemd unit (`User=interceptor-mcp`,
  `StateDirectory=protonmail-mcp`, `Environment=` the sse + file vars,
  `ExecStart=…`) and a one-time bootstrap command run as the service user with
  the same state dir, e.g.
  `sudo -u interceptor-mcp env PROTONMAIL_MCP_CREDENTIAL_BACKEND=file PROTONMAIL_MCP_STATE_DIR=/var/lib/protonmail-mcp protonmail-mcp login`.
- **Bootstrap constraint (required for slice D):** at the 2FA prompt the operator
  must enter the **`otpauth://` TOTP secret URI**, not a one-time 6-digit code
  (`login.go:55` already accepts the URI into `TOTPSecret`). Without the stored
  secret, slice D cannot generate future codes.

**Tests.** `login` with `backend=file` writes a `0600` file containing creds +
session; `status` reads and reports it; `logout` clears it. Real-systemd
acceptance (a `--no-create-home` user with no D-Bus) stays **manual**,
operator-side — an agent can't exercise it here; covered by a documented
verification checklist, not automated tests.

### Slice D — unattended self-heal (auto-relogin)

Delivers the acceptance criterion "credentials persist **and refresh** without
Keychain/D-Bus" across refresh-token revocation.

Today `session.go:204-214`: when Proton rejects the stored refresh token, the
session returns `proton/auth_required` and stops — no recovery. Slice D adds
recovery **at this cold-start refresh path only** (the primary headless scenario:
box rebooted, or the refresh token expired/was revoked while the service idled).
A refresh rejection on an *already-live* client mid-flight still surfaces
`auth_required`; it recovers on the next cold-start `Client()` call rather than
mid-operation — bounding slice D to one well-defined seam. The added steps:

1. On the `NewClientWithRefresh` rejection at line 204, `LoadCreds()`.
2. If creds are present, call the existing
   `session.Login(ctx, LoginInput{Username, Password, TOTPSecret})` —
   non-interactive; it auto-generates the TOTP code from the stored secret
   (`session.go:323-324`).
3. Persist the new session via the active store and retry the original operation
   once.
4. If creds are absent, or `Login` fails, surface the existing
   `auth_required` error (pointing at `login`).

**Hard ceiling — CAPTCHA.** If Proton answers the re-login with a CAPTCHA
challenge (`proton/captcha`, common for headless / new-IP logins), it **cannot**
be solved unattended. Slice D surfaces a clear `proton/captcha`-derived error
instructing a manual re-login on the box. This ceiling is inherent to Proton's
anti-automation and is documented, not worked around.

**Concurrency.** Auto-relogin runs inside the existing `s.mu.Lock()` cold-start
path (`Client()`), so it can't race concurrent callers; a single relogin attempt
per failure, no retry storm.

**Tests.** With a fake store seeded with creds + a stale/rejected session and a
fake manager that rejects the refresh then accepts the login: assert one
`Login` call, new session persisted, operation retried. Creds absent → no login
attempt, `auth_required` surfaced. Login returns `proton/captcha` → surfaced as
manual-relogin error, no infinite retry.

## Data flow (sse + file, steady state)

1. systemd starts the binary as `interceptor-mcp`; env sets `TRANSPORT=sse`,
   `HOST`, `PORT`, `BACKEND=file`, `STATE_DIR`.
2. Server resolves transport (sse) and store (file), mounts `SSEHandler` at
   `/sse` on `host:port`.
3. Dashboard GETs `/sse`; handler opens a session, `server.Connect`.
4. First authenticated tool call: cold-start `Client()` loads the session from
   the file, refreshes via go-proton-api, persists rotated tokens (file `0600`).
5. On a later refresh-token rejection: slice D loads creds, re-logins
   (TOTP from stored secret), persists, retries — unless CAPTCHA, which surfaces
   a manual-relogin error.

## Error handling

- Startup config errors (bad transport/backend, missing sse port) → fail fast,
  exit non-zero, message names the offending var, allowed values, and fix.
- Store I/O errors on save → existing `persistDegraded` warning path (tokens
  still work in-memory; logged).
- Refresh/relogin terminal failures → stable `proton/auth_required` or
  `proton/captcha` codes with the `login` hint, matching today's contract.

## Backward compatibility

stdio + Keychain remain the defaults; with no new env vars set, behavior is
byte-for-byte the current behavior. No migration, no dual config.

## Decision log

- Credential backend: **0600 file in StateDirectory** (plaintext, perms +
  dedicated unprivileged user). Rejected: systemd `LoadCredentialEncrypted`
  (couples bootstrap to systemd), env-keyed encryption (key must live with the
  unattended process → theater vs local root), operator-injected path (punts too
  much).
- Transport: **SSE only** (matches the reserved `:8770/sse` and FastMCP sibling
  fleet). Rejected: Streamable-HTTP-only (diverges from fleet), both-by-env
  (extra surface to commit, YAGNI).
- Bootstrap: **store full creds, interactive `login` on the box**, plus slice D
  auto-relogin. Rejected: session-only export/import (manual re-bootstrap on
  every refresh break), env/stdin non-interactive login (long-lived password
  through env/args).
- Port default: **none — required when sse**. Rejected: defaulting to the
  consumer's `8770` (deployment-specific, not a tool default).

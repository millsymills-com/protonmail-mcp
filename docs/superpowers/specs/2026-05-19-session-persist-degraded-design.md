# Surfacing background token-persist failures via Session.Status()

**Issue:** [#76 — session: OnAuthRotated swallows keychain persist errors via slog.Warn](https://github.com/millsymills-com/protonmail-mcp/issues/76)

**Date:** 2026-05-19

## Problem

`Session.OnAuthRotated` and the cold-start refresh inside `Session.Client`
both call `s.kc.SaveSession(...)`. On failure, the only signal today is a
`slog.Warn("session: persist rotated tokens failed", ...)`. The in-memory
tokens still work for the current process, but the keychain holds the
stale pre-rotation refresh token. The next time the user starts a new
process, that stale token is used and Proton rejects the refresh.

The user never sees this between the rotation and the next process
start. They learn about it as an opaque `proton/auth_required` error
much later.

## Decision

Add a per-`Session` `PersistDegraded` flag that callers can read via a
new `Session.Status()` accessor. The flag is set when any rotation-time
`SaveSession` fails, cleared on the next successful `SaveSession` or on
`Logout`. It is surfaced through:

- `protonmail-mcp status` — an extra warning line.
- `proton_session_status` MCP tool — new JSON fields.
- `proton_whoami` MCP tool — same new JSON fields, so callers that only
  use `whoami` still see degradation.

This is option 2 from the issue's triage notes: status flag + dedicated
status surfacing. Rejected alternatives are recorded in the issue.

## Architecture

### `internal/session/session.go`

New private state on `Session`:

```go
type Session struct {
    // ...existing fields...

    persistDegraded  bool
    persistErrReason string
}
```

New public `Status` value type:

```go
type Status struct {
    PersistDegraded bool
    PersistError    string
}

func (s *Session) Status() Status
```

`Status` is intentionally scoped to persistence state. Logged-in-ness is
determined elsewhere (CLI calls `Client(ctx)` then `GetUser`; MCP tools
use `clientOrFail` + `GetUser`).

### Set / clear sites

| Call site | On `SaveSession` error | On `SaveSession` success |
|---|---|---|
| `Client` cold-start refresh (session.go:170) | set flag + reason, keep existing `slog.Warn` | clear flag + reason |
| `OnAuthRotated` (session.go:197) | same | clear flag + reason |
| `persistLoginState` (session.go:322) | (existing rollback path returns the error) | clear flag + reason |
| `Logout` success (session.go:201) | n/a | clear flag + reason |

The `poisoned` flag (set when `Clear` itself fails during login
rollback) is a separate failure mode and is unchanged. It surfaces as
`ErrSessionInconsistent` from `Client()`, not via `Status()`.

### Concurrency

- Cold-start path (`Client`): the lock is held throughout `SaveSession`,
  so set/clear happens inline under `s.mu.Lock`.
- `OnAuthRotated`: the existing code releases `s.mu` before calling
  `SaveSession`. After the call, re-acquire `s.mu.Lock` briefly to write
  the flag/reason. No new lock-ordering concerns.
- `persistLoginState`: caller (`Login`) holds `s.mu.Lock`. Inline set
  on success.
- `Logout`: holds `s.mu.Lock`. Inline clear.
- `Status()`: takes `s.mu.RLock`.

### Test helper

Add `func (s *Session) SetPersistDegradedForTest(reason string)` that
sets the flag + reason under `s.mu.Lock`. Keeps CLI and MCP tests
focused on the surfacing behavior without plumbing a fake keychain
through their boot paths. Real unit tests in `internal/session/` still
exercise the production path via a fake `keychainStore`.

## CLI surfacing — `cmd/protonmail-mcp/status.go`

After the existing email / storage line (or after `not logged in`):

```
warning: token persistence degraded — rotated tokens not saved to keychain ("save session: keychain locked"). Re-run `protonmail-mcp login` to restore.
```

- Emitted to stdout, same as the success line. Status command is
  informational; degraded persistence is not a hard failure.
- Exit code stays `0`.
- The parenthesized reason is `Status().PersistError` verbatim. Whatever
  the keychain layer returned is what the user sees.

## MCP tool surfacing — `internal/tools/identity.go`

Both tool output types gain two `omitempty` fields:

```go
type sessionStatusOutput struct {
    LoggedIn        bool   `json:"logged_in"`
    Email           string `json:"email,omitempty"`
    PersistDegraded bool   `json:"persist_degraded,omitempty"`
    PersistError    string `json:"persist_error,omitempty"`
}

type whoamiOutput struct {
    Email           string `json:"email" jsonschema:"..."`
    Name            string `json:"name,omitempty" jsonschema:"..."`
    UsedSpace       int64  `json:"used_space_bytes" jsonschema:"..."`
    MaxSpace        int64  `json:"max_space_bytes" jsonschema:"..."`
    PersistDegraded bool   `json:"persist_degraded,omitempty" jsonschema:"true when rotated tokens failed to persist to keychain"`
    PersistError    string `json:"persist_error,omitempty" jsonschema:"human-readable reason from the keychain layer"`
}
```

Both tools call `d.Session.Status()` after the existing logic and
populate the two fields. Backward compatible: existing field shape is
unchanged; new fields are absent (via `omitempty`) when persistence is
healthy.

Other read tools (addresses, settings, messages, etc.) are unchanged.
The existing `slog.Warn` continues to land in MCP server logs as the
operational signal for in-flight tool calls.

## Data flow

```
Proton /auth/v4/refresh
   ↓ (token rotation event)
Session.OnAuthRotated(next)
   ├── s.mu.Lock
   ├── s.current = next
   ├── s.raw.setAuth(next.AccessToken, next.UID)
   ├── s.mu.Unlock
   ├── kc.SaveSession(next)
   │     ├── ok    → s.mu.Lock; persistDegraded = false; persistErrReason = ""; s.mu.Unlock
   │     └── err   → s.mu.Lock; persistDegraded = true;  persistErrReason = err.Error(); s.mu.Unlock
   │               slog.Warn (unchanged)
```

Caller paths:

```
protonmail-mcp status        →  sess.Client(ctx) → GetUser → sess.Status()
proton_session_status (MCP)  →  clientOrFail → GetUser → d.Session.Status()
proton_whoami (MCP)          →  clientOrFail → GetUser → d.Session.Status()
```

## Test plan

Use TDD: each test goes red before the corresponding production change.

1. `internal/session/session_internal_test.go` (new file, `package session`):
   - Fake `keychainStore` with a toggleable `SaveSession` error.
   - `TestStatusPersistDegradedOnRotation` — `OnAuthRotated` while
     SaveSession errs → `Status().PersistDegraded == true`,
     `PersistError` contains the err text.
   - `TestStatusPersistDegradedClearsOnNextSuccess` — set flag, flip
     fake to OK, `OnAuthRotated` again → `Status().PersistDegraded ==
     false`, `PersistError == ""`.
   - `TestStatusPersistDegradedClearsOnLogout` — set flag, `Logout` →
     cleared.
   - `TestStatusPersistDegradedClearsOnLogin` — set flag, `Login`
     using the existing `login_no_2fa` cassette transport → cleared.
   - `TestStatusPersistDegradedOnColdStart` — `Client(ctx)` driven by
     the existing `token_rotation` cassette, with a fake kc that
     `LoadSession` returns the seed but `SaveSession` errs → flag set.

   Because the fake `keychainStore` is unexported, these tests live
   inside `package session` (internal test file), unlike the existing
   `package session_test` external tests.

2. `cmd/protonmail-mcp/run_test.go`:
   - `TestStatusReportsPersistDegraded` — boot status with a
     pre-degraded session (helper) + cassette for GetUser, assert the
     warning line.

3. `internal/tools/identity_test.go`:
   - `TestSessionStatusReportsPersistDegraded` — assert the new JSON
     fields appear when flag is set.
   - `TestWhoamiReportsPersistDegraded` — same, on `proton_whoami`.

4. Verify existing tests in all three packages still pass — backward
   compatibility check.

## Non-goals

(Recorded so future readers know these were considered and rejected.)

- No "side cache file" fallback store. Reverted in design discussion.
- No fail-loud on the in-flight caller that triggered the rotation.
- No degradation fields on tools other than `session_status` and
  `whoami`. Operational signal for other tools stays at `slog.Warn`.
- No retry loop. The user re-runs `login` to restore persistence.
- No surfacing of the existing `poisoned`/`ErrSessionInconsistent`
  state through `Status()`. That has its own surface already.

## Files touched

- `internal/session/session.go` — fields, set/clear, `Status()`, test helper.
- `internal/session/session_internal_test.go` — new file.
- `cmd/protonmail-mcp/status.go` — emit warning line.
- `cmd/protonmail-mcp/run_test.go` — new test.
- `internal/tools/identity.go` — extend both tool outputs.
- `internal/tools/identity_test.go` — two new tests.

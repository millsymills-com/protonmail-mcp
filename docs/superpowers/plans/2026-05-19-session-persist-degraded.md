# Session persist-degraded surfacing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface failed background token-persist writes (`OnAuthRotated` / cold-start refresh) via a new `Session.Status()` accessor, propagated to the `protonmail-mcp status` CLI command and the `proton_session_status` + `proton_whoami` MCP tools. Closes #76.

**Architecture:** Add `persistDegraded bool` + `persistErrReason string` to `Session`. Toggle inside the existing rotation/persist call sites (set on `SaveSession` error, clear on success or `Logout`). Expose via `Status() Status`. Two read consumers (CLI status, two MCP tools) wire the flag into their existing output paths. No new packages, no new dependencies.

**Tech Stack:** Go 1.26, `gopkg.in/dnaeon/go-vcr.v4` (existing cassette infra), `github.com/modelcontextprotocol/go-sdk/mcp` (existing tool surface).

**Reference spec:** `docs/superpowers/specs/2026-05-19-session-persist-degraded-design.md`.

---

## File map

**Created:**
- `internal/session/persist_degraded_test.go` — TDD tests against `Session.Status()`, persist-failure injection via a fake `keychainStore` (external package; method-set assignability).

**Modified:**
- `internal/session/session.go` — fields, `Status` type, `Status()` method, set/clear at four sites (`Client` cold-start, `OnAuthRotated`, `persistLoginState`, `Logout`), `SetPersistDegradedForTest`.
- `cmd/protonmail-mcp/status.go` — emit warning line when `Status().PersistDegraded`.
- `cmd/protonmail-mcp/run_test.go` — new `TestStatusReportsPersistDegraded`.
- `internal/tools/identity.go` — add `PersistDegraded` + `PersistError` to `sessionStatusOutput` and `whoamiOutput`; populate from `d.Session.Status()`.
- `internal/tools/identity_test.go` — new `TestSessionStatusReportsPersistDegraded` + `TestWhoamiReportsPersistDegraded`.

**Out of scope (do not touch):**
- Any other `internal/tools/*.go` tool — operational signal for these stays at `slog.Warn`.
- `internal/keychain/*.go` — fake lives in the session test file, not in keychain.
- The `poisoned` flag and `ErrSessionInconsistent` — separate failure mode.

---

## Task 0: Commit spec + plan

**Files:**
- `docs/superpowers/specs/2026-05-19-session-persist-degraded-design.md`
- `docs/superpowers/plans/2026-05-19-session-persist-degraded.md`

- [ ] **Step 1: Confirm both files are present**

```bash
ls docs/superpowers/specs/2026-05-19-session-persist-degraded-design.md \
   docs/superpowers/plans/2026-05-19-session-persist-degraded.md
```

Expected: both files listed, no errors.

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-05-19-session-persist-degraded-design.md \
        docs/superpowers/plans/2026-05-19-session-persist-degraded.md
git commit -m "docs: spec + plan for session persist-degraded surfacing (#76)"
```

---

## Task 1: Add `Status` type, `Status()` method, fields, and helper

**Files:**
- Modify: `internal/session/session.go`
- Create: `internal/session/persist_degraded_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/session/persist_degraded_test.go`:

```go
package session_test

import (
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/zalando/go-keyring"
)

func TestStatusZeroOnFreshSession(t *testing.T) {
	keyring.MockInit()
	s, err := session.NewForTesting("http://invalid.test",
		keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	defer func() { _ = s.Logout() }()

	got := s.Status()
	if got.PersistDegraded || got.PersistError != "" {
		t.Fatalf("fresh Status() = %+v, want zero", got)
	}
}
```

- [ ] **Step 2: Run test, confirm it fails to compile**

```bash
go test ./internal/session/ -run TestStatusZeroOnFreshSession -count=1
```

Expected: build failure — `s.Status undefined`.

- [ ] **Step 3: Add fields, `Status` type, and method**

In `internal/session/session.go`, extend the `Session` struct (keep ordering aligned with existing fields):

```go
type Session struct {
	mu       sync.RWMutex
	mgr      *proton.Manager
	client   *proton.Client
	raw      *rawClient
	kc       keychainStore
	current  keychain.Session
	poisoned bool

	// persistDegraded is true when the most recent rotation-time
	// SaveSession write failed; the in-memory tokens still work for the
	// current process, but the keychain holds the stale pre-rotation
	// refresh token. Cleared by the next successful SaveSession or by
	// Logout.
	persistDegraded  bool
	persistErrReason string
}
```

Add the public surface just below `ErrTOTPRequired`:

```go
// Status reports persistence-layer health. PersistDegraded is true when
// the most recent background SaveSession write failed; in-memory tokens
// still work for the current process. Surfaced through
// `protonmail-mcp status` and the proton_session_status /
// proton_whoami MCP tools.
type Status struct {
	PersistDegraded bool
	PersistError    string
}

func (s *Session) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{
		PersistDegraded: s.persistDegraded,
		PersistError:    s.persistErrReason,
	}
}

// SetPersistDegradedForTest is used by CLI and MCP-tool tests to inject
// the degraded state without driving an actual SaveSession failure. The
// real session_test cases drive it through a failing fake keychainStore.
func (s *Session) SetPersistDegradedForTest(reason string) {
	s.mu.Lock()
	s.persistDegraded = reason != ""
	s.persistErrReason = reason
	s.mu.Unlock()
}
```

- [ ] **Step 4: Run test, confirm it passes**

```bash
go test ./internal/session/ -run TestStatusZeroOnFreshSession -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/session/session.go internal/session/persist_degraded_test.go
git commit -m "feat(session): Status() accessor for persist-degraded state (#76)"
```

---

## Task 2: Set flag in `OnAuthRotated` on SaveSession failure

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/persist_degraded_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/session/persist_degraded_test.go`:

```go
import (
	"errors"
	// ... existing imports ...
)

// fakeKC satisfies the unexported keychainStore method set. Go's
// assignability rules permit external packages to pass values that
// satisfy unexported interfaces, even though the interface type itself
// is not visible.
type fakeKC struct {
	seed       keychain.Session
	saveErr    error
	loadErr    error
	clearErr   error
	saveCalled int
}

func (f *fakeKC) SaveCreds(keychain.Creds) error          { return nil }
func (f *fakeKC) LoadCreds() (keychain.Creds, error)      { return keychain.Creds{}, nil }
func (f *fakeKC) SaveSession(s keychain.Session) error {
	f.saveCalled++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.seed = s
	return nil
}
func (f *fakeKC) LoadSession() (keychain.Session, error) {
	if f.loadErr != nil {
		return keychain.Session{}, f.loadErr
	}
	return f.seed, nil
}
func (f *fakeKC) Clear() error { f.seed = keychain.Session{}; return f.clearErr }

func TestStatusPersistDegradedOnRotation(t *testing.T) {
	kc := &fakeKC{
		seed:    keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"},
		saveErr: errors.New("save session: keychain locked"),
	}
	s := session.New("http://invalid.test", kc)

	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "b", RefreshToken: "r2"})

	got := s.Status()
	if !got.PersistDegraded {
		t.Fatalf("PersistDegraded = false, want true")
	}
	if got.PersistError != "save session: keychain locked" {
		t.Fatalf("PersistError = %q, want %q", got.PersistError, "save session: keychain locked")
	}
}
```

- [ ] **Step 2: Run test, confirm it fails**

```bash
go test ./internal/session/ -run TestStatusPersistDegradedOnRotation -count=1
```

Expected: FAIL — `PersistDegraded = false, want true`.

- [ ] **Step 3: Set flag inside `OnAuthRotated`**

In `internal/session/session.go`, replace the body of `OnAuthRotated`:

```go
func (s *Session) OnAuthRotated(next keychain.Session) {
	s.mu.Lock()
	s.current = next
	s.raw.setAuth(next.AccessToken, next.UID)
	s.mu.Unlock()
	if err := s.kc.SaveSession(next); err != nil {
		s.mu.Lock()
		s.persistDegraded = true
		s.persistErrReason = err.Error()
		s.mu.Unlock()
		slog.Warn("session: persist rotated tokens failed", "err", err)
	}
}
```

- [ ] **Step 4: Run test, confirm it passes**

```bash
go test ./internal/session/ -run TestStatusPersistDegradedOnRotation -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/session/session.go internal/session/persist_degraded_test.go
git commit -m "feat(session): mark persist-degraded on OnAuthRotated SaveSession err (#76)"
```

---

## Task 3: Clear flag in `OnAuthRotated` on success

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/persist_degraded_test.go`

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestStatusPersistDegradedClearsOnNextRotationSuccess(t *testing.T) {
	kc := &fakeKC{
		seed:    keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"},
		saveErr: errors.New("save session: keychain locked"),
	}
	s := session.New("http://invalid.test", kc)

	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "b", RefreshToken: "r2"})
	if !s.Status().PersistDegraded {
		t.Fatalf("setup: expected PersistDegraded=true")
	}

	kc.saveErr = nil
	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "c", RefreshToken: "r3"})

	got := s.Status()
	if got.PersistDegraded || got.PersistError != "" {
		t.Fatalf("Status() = %+v, want zero after successful rotation", got)
	}
}
```

- [ ] **Step 2: Run test, confirm it fails**

```bash
go test ./internal/session/ -run TestStatusPersistDegradedClearsOnNextRotationSuccess -count=1
```

Expected: FAIL — `Status() = {true ...}, want zero`.

- [ ] **Step 3: Clear on success**

Replace the body of `OnAuthRotated` again:

```go
func (s *Session) OnAuthRotated(next keychain.Session) {
	s.mu.Lock()
	s.current = next
	s.raw.setAuth(next.AccessToken, next.UID)
	s.mu.Unlock()
	if err := s.kc.SaveSession(next); err != nil {
		s.mu.Lock()
		s.persistDegraded = true
		s.persistErrReason = err.Error()
		s.mu.Unlock()
		slog.Warn("session: persist rotated tokens failed", "err", err)
		return
	}
	s.mu.Lock()
	s.persistDegraded = false
	s.persistErrReason = ""
	s.mu.Unlock()
}
```

- [ ] **Step 4: Run test, confirm it passes**

```bash
go test ./internal/session/ -run TestStatusPersistDegradedClearsOnNextRotationSuccess -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/session/session.go internal/session/persist_degraded_test.go
git commit -m "feat(session): clear persist-degraded on next rotation success (#76)"
```

---

## Task 4: Clear flag in `Logout`

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/persist_degraded_test.go`

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestStatusPersistDegradedClearsOnLogout(t *testing.T) {
	keyring.MockInit()
	kc := &fakeKC{
		seed:    keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"},
		saveErr: errors.New("save session: keychain locked"),
	}
	s := session.New("http://invalid.test", kc)

	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "b", RefreshToken: "r2"})
	if !s.Status().PersistDegraded {
		t.Fatalf("setup: expected PersistDegraded=true")
	}

	if err := s.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	got := s.Status()
	if got.PersistDegraded || got.PersistError != "" {
		t.Fatalf("Status() = %+v, want zero after Logout", got)
	}
}
```

- [ ] **Step 2: Run test, confirm it fails**

```bash
go test ./internal/session/ -run TestStatusPersistDegradedClearsOnLogout -count=1
```

Expected: FAIL — flag still set.

- [ ] **Step 3: Clear in `Logout`**

In `internal/session/session.go`, inside `Logout` after the `s.poisoned = false` line (still under `s.mu.Lock()`), add:

```go
	s.persistDegraded = false
	s.persistErrReason = ""
```

So the full method tail becomes:

```go
func (s *Session) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	s.current = keychain.Session{}
	s.raw.setAuth("", "")
	if err := s.kc.Clear(); err != nil {
		// Leave poisoned flag set if it was set — Clear failed again, so
		// state is still inconsistent.
		return err
	}
	s.poisoned = false
	s.persistDegraded = false
	s.persistErrReason = ""
	return nil
}
```

- [ ] **Step 4: Run test, confirm it passes**

```bash
go test ./internal/session/ -run TestStatusPersistDegradedClearsOnLogout -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/session/session.go internal/session/persist_degraded_test.go
git commit -m "feat(session): clear persist-degraded on Logout (#76)"
```

---

## Task 5: Set flag in cold-start refresh in `Client`

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/persist_degraded_test.go`

- [ ] **Step 1: Write the failing test**

Append:

```go
import (
	"context"
	// ... existing imports ...
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func TestStatusPersistDegradedOnColdStart(t *testing.T) {
	kc := &fakeKC{
		seed: keychain.Session{
			UID:          "REDACTED_UID_1",
			AccessToken:  "REDACTED_ACCESSTOKEN_1",
			RefreshToken: "REDACTED_REFRESHTOKEN_1",
		},
		saveErr: errors.New("save session: keychain locked"),
	}
	rt := testvcr.New(t, "token_rotation")
	s := session.New("https://mail.proton.me/api", kc, session.WithTransport(rt))

	c, err := s.Client(context.Background())
	if err != nil {
		t.Fatalf("Client: %v", err)
	}
	if _, err := c.GetUser(context.Background()); err != nil {
		t.Fatalf("GetUser: %v", err)
	}

	got := s.Status()
	if !got.PersistDegraded {
		t.Fatalf("PersistDegraded = false, want true")
	}
	if got.PersistError != "save session: keychain locked" {
		t.Fatalf("PersistError = %q", got.PersistError)
	}
}
```

- [ ] **Step 2: Run test, confirm it fails**

```bash
go test ./internal/session/ -run TestStatusPersistDegradedOnColdStart -count=1
```

Expected: FAIL — flag not set on cold-start path.

- [ ] **Step 3: Set flag inside `Client` cold-start**

In `internal/session/session.go`, replace the cold-start `SaveSession` block (currently at lines 170–172) with:

```go
	if err := s.kc.SaveSession(rotated); err != nil {
		s.persistDegraded = true
		s.persistErrReason = err.Error()
		slog.Warn("session: persist rotated tokens failed", "err", err)
	} else {
		s.persistDegraded = false
		s.persistErrReason = ""
	}
```

(The lock is already held throughout this code path, so direct field writes are safe.)

- [ ] **Step 4: Run test, confirm it passes**

```bash
go test ./internal/session/ -run TestStatusPersistDegradedOnColdStart -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/session/session.go internal/session/persist_degraded_test.go
git commit -m "feat(session): mark persist-degraded on cold-start SaveSession err (#76)"
```

---

## Task 6: Clear flag in `persistLoginState` success path

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/persist_degraded_test.go`

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestStatusPersistDegradedClearsOnLogin(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()

	rt := testvcr.New(t, "login_no_2fa")
	s := session.New("https://mail.proton.me/api", kc, session.WithTransport(rt))

	// Inject degraded state directly; Login below should clear it.
	s.SetPersistDegradedForTest("save session: keychain locked")
	if !s.Status().PersistDegraded {
		t.Fatalf("setup: expected PersistDegraded=true")
	}

	if err := s.Login(context.Background(), session.LoginInput{
		Username: "user@example.test",
		Password: "hunter2",
	}); err != nil {
		t.Fatalf("Login: %v", err)
	}

	got := s.Status()
	if got.PersistDegraded || got.PersistError != "" {
		t.Fatalf("Status() = %+v, want zero after successful Login", got)
	}
}
```

- [ ] **Step 2: Run test, confirm it fails**

```bash
go test ./internal/session/ -run TestStatusPersistDegradedClearsOnLogin -count=1
```

Expected: FAIL — flag not cleared by Login.

- [ ] **Step 3: Clear in `persistLoginState` after both saves succeed**

In `internal/session/session.go`, replace the body of `persistLoginState`:

```go
func (s *Session) persistLoginState(creds keychain.Creds, sess keychain.Session) error {
	if err := s.kc.SaveCreds(creds); err != nil {
		return s.rollbackLoginPersist("save creds", err)
	}
	if err := s.kc.SaveSession(sess); err != nil {
		return s.rollbackLoginPersist("save session", err)
	}
	s.persistDegraded = false
	s.persistErrReason = ""
	return nil
}
```

(Caller `Login` already holds `s.mu.Lock()`; safe to write directly.)

- [ ] **Step 4: Run test, confirm it passes**

```bash
go test ./internal/session/ -run TestStatusPersistDegradedClearsOnLogin -count=1
```

Expected: PASS.

- [ ] **Step 5: Run the full session-package test suite**

```bash
go test ./internal/session/ -count=1
```

Expected: PASS (all pre-existing tests + the new ones).

- [ ] **Step 6: Commit**

```bash
git add internal/session/session.go internal/session/persist_degraded_test.go
git commit -m "feat(session): clear persist-degraded on Login success (#76)"
```

---

## Task 7: CLI `status` warning line

**Files:**
- Modify: `cmd/protonmail-mcp/status.go`
- Modify: `cmd/protonmail-mcp/run_test.go`

- [ ] **Step 1: Write the failing test**

Add to `cmd/protonmail-mcp/run_test.go` (after `TestStatusLoggedInUsesCassette`):

```go
func TestStatusReportsPersistDegraded(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	seed := keychain.Session{
		UID:          "REDACTED_UID_1",
		AccessToken:  "REDACTED_ACCESSTOKEN_1",
		RefreshToken: "REDACTED_REFRESHTOKEN_1",
	}
	if err := kc.SaveSession(seed); err != nil {
		t.Fatal(err)
	}
	rt := testvcr.New(t, "status_logged_in")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	preboot := func(s *session.Session) {
		s.SetPersistDegradedForTest("save session: keychain locked")
	}

	code := runWithSessionHook(
		context.Background(),
		[]string{"status"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		strings.NewReader(""),
		stdout,
		stderr,
		rt,
		preboot,
	)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "user@example.test") {
		t.Fatalf("stdout missing email: %q", out)
	}
	if !strings.Contains(out, "warning: token persistence degraded") {
		t.Fatalf("stdout missing warning: %q", out)
	}
	if !strings.Contains(out, "save session: keychain locked") {
		t.Fatalf("stdout missing persist error reason: %q", out)
	}
}
```

Add the matching import line:

```go
"github.com/millsmillsymills/protonmail-mcp/internal/session"
```

This test calls a not-yet-existing `runWithSessionHook` shim that mirrors the existing `run(ctx, args, env, stdin, stdout, stderr, transport)` signature plus a `statusHook func(*session.Session)` parameter. Implementation lives in `cmd/protonmail-mcp/main.go` alongside `run`.

- [ ] **Step 2: Run test, confirm it fails**

```bash
go test ./cmd/protonmail-mcp/ -run TestStatusReportsPersistDegraded -count=1
```

Expected: build failure (`runWithSessionHook undefined`).

- [ ] **Step 3: Add the hook + warning line**

In `cmd/protonmail-mcp/status.go`, replace the file with:

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func runStatus(
	ctx context.Context, apiURL string, transport http.RoundTripper, out io.Writer,
) error {
	return runStatusWithHook(ctx, apiURL, transport, out, nil)
}

// runStatusWithHook is the test-injectable variant. `hook`, when non-nil,
// runs after Session is constructed and before any API calls, letting
// tests seed Session state (e.g. PersistDegraded) deterministically.
func runStatusWithHook(
	ctx context.Context,
	apiURL string,
	transport http.RoundTripper,
	out io.Writer,
	hook func(*session.Session),
) error {
	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	sess := session.New(apiURL, keychain.New(), session.WithTransport(transport))
	if hook != nil {
		hook(sess)
	}
	c, err := sess.Client(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(out, "not logged in")
		writePersistWarning(out, sess.Status())
		return nil
	}
	u, err := c.GetUser(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "%s — %d / %d bytes\n", u.Email, u.UsedSpace, u.MaxSpace)
	writePersistWarning(out, sess.Status())
	return nil
}

func writePersistWarning(out io.Writer, st session.Status) {
	if !st.PersistDegraded {
		return
	}
	_, _ = fmt.Fprintf(out,
		"warning: token persistence degraded — rotated tokens not saved to keychain (%q). Re-run `protonmail-mcp login` to restore.\n",
		st.PersistError)
}
```

In `cmd/protonmail-mcp/main.go`, add a `runWithSessionHook` helper directly below `run`. `run()` already reads the API URL via `envLookup(env, "PROTONMAIL_MCP_API_URL")` inline (no separate helper), so the new function does the same and dispatches only the `status` subcommand through the hooked path. All other args fall through to `run` so existing callers stay untouched. Required import: add `"github.com/millsmillsymills/protonmail-mcp/internal/session"`.

```go
// runWithSessionHook is the test-injectable variant of run. Only the
// "status" subcommand honours statusHook today; all other subcommands
// delegate to run for parity.
func runWithSessionHook(
	ctx context.Context,
	args []string,
	env []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	transport http.RoundTripper,
	statusHook func(*session.Session),
) int {
	if len(args) > 0 && args[0] == "status" {
		apiURL := envLookup(env, "PROTONMAIL_MCP_API_URL")
		if err := runStatusWithHook(ctx, apiURL, transport, stdout, statusHook); err != nil {
			_, _ = stderr.Write([]byte("status: " + err.Error() + "\n"))
			return 1
		}
		return 0
	}
	return run(ctx, args, env, stdin, stdout, stderr, transport)
}
```

- [ ] **Step 4: Run test, confirm it passes**

```bash
go test ./cmd/protonmail-mcp/ -run TestStatusReportsPersistDegraded -count=1
```

Expected: PASS.

- [ ] **Step 5: Run the full CLI-package test suite**

```bash
go test ./cmd/protonmail-mcp/ -count=1
```

Expected: PASS (existing tests untouched).

- [ ] **Step 6: Commit**

```bash
git add cmd/protonmail-mcp/status.go cmd/protonmail-mcp/main.go cmd/protonmail-mcp/run_test.go
git commit -m "feat(cli): surface persist-degraded warning in status (#76)"
```

---

## Task 8: MCP `proton_session_status` fields

**Files:**
- Modify: `internal/tools/identity.go`
- Modify: `internal/tools/identity_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/tools/identity_test.go`:

```go
func TestSessionStatusReportsPersistDegraded(t *testing.T) {
	h := testharness.BootWithCassette(t, "session_status_happy")
	defer h.Close()
	h.Session().SetPersistDegradedForTest("save session: keychain locked")

	out, err := h.Call(context.Background(), "proton_session_status", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if v, ok := out["persist_degraded"]; !ok || v != true {
		t.Fatalf("persist_degraded = %v, want true", out["persist_degraded"])
	}
	if v, ok := out["persist_error"]; !ok || v != "save session: keychain locked" {
		t.Fatalf("persist_error = %v, want %q", out["persist_error"], "save session: keychain locked")
	}
}
```

`testharness.Harness` already stores the session as the unexported field `sess` (declared in `internal/testharness/harness.go`). Add an exported accessor in the same file:

```go
// Session exposes the underlying *session.Session so tests can inject
// state (e.g. SetPersistDegradedForTest) before calling a tool.
func (h *Harness) Session() *session.Session { return h.sess }
```

- [ ] **Step 2: Run test, confirm it fails**

```bash
go test ./internal/tools/ -run TestSessionStatusReportsPersistDegraded -count=1
```

Expected: build failure (Session accessor missing) or assertion failure (fields absent).

- [ ] **Step 3: Add the `Harness.Session()` accessor**

In `internal/testharness/harness.go`, add (placement: just below the
`Harness` struct declaration, alongside any other small accessors):

```go
// Session exposes the underlying *session.Session so tests can inject
// state (e.g. SetPersistDegradedForTest) before calling a tool.
func (h *Harness) Session() *session.Session { return h.sess }
```

- [ ] **Step 4: Extend the tool output**

In `internal/tools/identity.go`, replace the `sessionStatusOutput` struct and the handler body:

```go
type sessionStatusOutput struct {
	LoggedIn        bool   `json:"logged_in"`
	Email           string `json:"email,omitempty"`
	PersistDegraded bool   `json:"persist_degraded,omitempty"`
	PersistError    string `json:"persist_error,omitempty"`
}
```

Inside `registerIdentity`, replace the existing `proton_session_status` handler:

```go
mcp.AddTool(server, &mcp.Tool{
	Name:        "proton_session_status",
	Description: "Reports whether a session is currently authenticated and whether token persistence is healthy.",
}, func(ctx context.Context, _ *mcp.CallToolRequest, _ sessionStatusInput) (*mcp.CallToolResult, sessionStatusOutput, error) {
	st := d.Session.Status()
	c, fail := clientOrFail(ctx, d)
	if fail != nil {
		return nil, sessionStatusOutput{
			LoggedIn:        false,
			PersistDegraded: st.PersistDegraded,
			PersistError:    st.PersistError,
		}, nil
	}
	u, err := c.GetUser(ctx)
	if err != nil {
		return nil, sessionStatusOutput{
			LoggedIn:        false,
			PersistDegraded: st.PersistDegraded,
			PersistError:    st.PersistError,
		}, nil
	}
	return nil, sessionStatusOutput{
		LoggedIn:        true,
		Email:           u.Email,
		PersistDegraded: st.PersistDegraded,
		PersistError:    st.PersistError,
	}, nil
})
```

- [ ] **Step 5: Run test, confirm it passes**

```bash
go test ./internal/tools/ -run TestSessionStatusReportsPersistDegraded -count=1
```

Expected: PASS.

- [ ] **Step 6: Verify existing session_status test still passes**

```bash
go test ./internal/tools/ -run TestSessionStatusHappyCassette -count=1
```

Expected: PASS (no schema break — fields omitempty when not set).

- [ ] **Step 7: Commit**

```bash
git add internal/tools/identity.go internal/tools/identity_test.go internal/testharness/
git commit -m "feat(tools): surface persist-degraded in proton_session_status (#76)"
```

---

## Task 9: MCP `proton_whoami` fields

**Files:**
- Modify: `internal/tools/identity.go`
- Modify: `internal/tools/identity_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/tools/identity_test.go`:

```go
func TestWhoamiReportsPersistDegraded(t *testing.T) {
	h := testharness.BootWithCassette(t, "whoami_happy")
	defer h.Close()
	h.Session().SetPersistDegradedForTest("save session: keychain locked")

	out, err := h.Call(context.Background(), "proton_whoami", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if v, ok := out["persist_degraded"]; !ok || v != true {
		t.Fatalf("persist_degraded = %v, want true", out["persist_degraded"])
	}
	if v, ok := out["persist_error"]; !ok || v != "save session: keychain locked" {
		t.Fatalf("persist_error = %v, want %q", out["persist_error"], "save session: keychain locked")
	}
	for _, k := range []string{"email", "used_space_bytes", "max_space_bytes"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("envelope missing %q", k)
		}
	}
}
```

- [ ] **Step 2: Run test, confirm it fails**

```bash
go test ./internal/tools/ -run TestWhoamiReportsPersistDegraded -count=1
```

Expected: FAIL — fields absent.

- [ ] **Step 3: Extend `whoamiOutput` and the handler**

In `internal/tools/identity.go`:

```go
type whoamiOutput struct {
	Email           string `json:"email" jsonschema:"the primary email of the logged-in account"`
	Name            string `json:"name,omitempty" jsonschema:"the user's display name if set"`
	UsedSpace       int64  `json:"used_space_bytes" jsonschema:"current storage usage in bytes"`
	MaxSpace        int64  `json:"max_space_bytes" jsonschema:"plan's storage quota in bytes"`
	PersistDegraded bool   `json:"persist_degraded,omitempty" jsonschema:"true when the most recent background token-persist write failed"`
	PersistError    string `json:"persist_error,omitempty" jsonschema:"human-readable reason from the keychain layer"`
}
```

Replace the `proton_whoami` handler body:

```go
mcp.AddTool(server, &mcp.Tool{
	Name:        "proton_whoami",
	Description: "Returns the logged-in Proton account's email, display name, storage usage, and token-persistence health.",
}, func(ctx context.Context, _ *mcp.CallToolRequest, _ whoamiInput) (*mcp.CallToolResult, whoamiOutput, error) {
	c, fail := clientOrFail(ctx, d)
	if fail != nil {
		return fail, whoamiOutput{}, nil
	}
	u, err := c.GetUser(ctx)
	if err != nil {
		return failure(proterr.Map(err)), whoamiOutput{}, nil
	}
	st := d.Session.Status()
	return nil, whoamiOutput{
		Email:           u.Email,
		Name:            u.DisplayName,
		UsedSpace:       int64(u.UsedSpace),
		MaxSpace:        int64(u.MaxSpace),
		PersistDegraded: st.PersistDegraded,
		PersistError:    st.PersistError,
	}, nil
})
```

- [ ] **Step 4: Run test, confirm it passes**

```bash
go test ./internal/tools/ -run TestWhoamiReportsPersistDegraded -count=1
```

Expected: PASS.

- [ ] **Step 5: Verify all existing whoami tests still pass**

```bash
go test ./internal/tools/ -run 'TestWhoami' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tools/identity.go internal/tools/identity_test.go
git commit -m "feat(tools): surface persist-degraded in proton_whoami (#76)"
```

---

## Task 10: Full-suite regression check

**Files:** none (verification only).

- [ ] **Step 1: Run the whole test suite**

```bash
go test ./... -count=1
```

Expected: PASS, no skips outside the existing recording-tagged tests.

- [ ] **Step 2: Lint**

```bash
golangci-lint run ./...
```

Expected: zero findings. Fix anything new the changes introduced.

- [ ] **Step 3: Verify the spec doc is present in the worktree**

```bash
ls docs/superpowers/specs/2026-05-19-session-persist-degraded-design.md docs/superpowers/plans/2026-05-19-session-persist-degraded.md
```

Both files must be present and tracked.

- [ ] **Step 4: Confirm no stray uncommitted changes**

```bash
git status
```

Expected: clean tree. The spec + plan docs were committed in Task 0; impl + tests were committed in Tasks 1–9.

---

## Task 11: Push branch and open PR

**Files:** none.

- [ ] **Step 1: Push the worktree branch**

```bash
git push -u origin "$(git branch --show-current)"
```

- [ ] **Step 2: Open the PR against `main`**

```bash
gh pr create \
  --repo millsymills-com/protonmail-mcp \
  --base main \
  --title "feat(session): surface persist-degraded state via Status() (#76)" \
  --body "$(cat <<'EOF'
## Summary

Closes #76. Surfaces failed background token-persist writes
(`OnAuthRotated` and cold-start refresh in `Session.Client`) through a
new `Session.Status()` accessor, so users learn about keychain write
failures at the next `protonmail-mcp status` or
`proton_session_status` / `proton_whoami` MCP-tool call instead of
discovering them on the next process start.

Design picked option 2 from the triage notes on #76 (status flag +
dedicated status surfacing). Other options rejected during design.

## Changes

- `internal/session/session.go` — `persistDegraded` + `persistErrReason`
  fields; new `Status` type + `Status()` accessor; `SetPersistDegradedForTest`
  helper; toggle at four sites (`Client` cold-start, `OnAuthRotated`,
  `persistLoginState` success, `Logout`).
- `cmd/protonmail-mcp/status.go` + `main.go` — emit a warning line on
  stdout when `Status().PersistDegraded`. Exit code unchanged.
- `internal/tools/identity.go` — `proton_session_status` and
  `proton_whoami` both gain `persist_degraded` + `persist_error`
  (omitempty) output fields.

Backward-compatible: existing fields untouched; new fields absent from
JSON when persistence is healthy.

## Test plan

- [x] New unit tests in `internal/session/persist_degraded_test.go`
      cover all four toggle paths (rotation set/clear, cold-start set,
      Login clear, Logout clear).
- [x] New CLI test `TestStatusReportsPersistDegraded` asserts the
      warning line and the error reason both appear on stdout.
- [x] New MCP tests `TestSessionStatusReportsPersistDegraded` and
      `TestWhoamiReportsPersistDegraded` assert the new JSON fields.
- [x] Existing tests in `internal/session/`, `cmd/protonmail-mcp/`,
      `internal/tools/` continue to pass (backward-compat check).
- [x] `golangci-lint run ./...` clean.

> *This was generated by AI during implementation.*
EOF
)"
```

- [ ] **Step 3: Capture and report the PR URL**

`gh pr create` prints the PR URL on success. Report it back so the
maintainer can review.

---

## Self-review notes (author)

- Tasks 1–6 fully cover the four set/clear sites and the `Status()`
  accessor from the spec.
- Task 7 covers the CLI surface.
- Tasks 8–9 cover the MCP surface (both tools).
- Task 10 catches anything that breaks unrelated tests.
- Task 11 ships.
- All test code blocks contain real, runnable Go. Every commit step
  uses an exact `git commit` invocation. No "TBD" or "similar to
  Task N" placeholders.

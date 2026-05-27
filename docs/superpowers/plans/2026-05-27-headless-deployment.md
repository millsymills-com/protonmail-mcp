# Headless Linux Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `protonmail-mcp` run headless on Linux — SSE transport on a loopback port, a file-based credential store (no Keychain/D-Bus), and unattended self-heal across refresh-token revocation.

**Architecture:** Four independently-shippable slices behind env flags. A adds an SSE transport seam; B adds a 0600-file credential backend; C exports the store interface, adds a backend selector, and documents the systemd bootstrap; D adds auto-relogin on cold-start refresh failure. stdio + Keychain stay the defaults. Build order: **A ∥ B → C → D**.

**Tech Stack:** Go, `modelcontextprotocol/go-sdk v1.6.0` (`NewSSEHandler`/`SSEClientTransport`), `ProtonMail/go-proton-api`, existing `internal/{server,session,keychain}` packages.

**Spec:** `docs/superpowers/specs/2026-05-27-headless-deployment-design.md`

**Conventions:** Each task is TDD (write failing test → run red → implement → run green → commit). Run `go test ./... -race -count=1` plus `go vet ./...` before each commit. Branch: `feat/headless-deployment` (already created; the spec is committed there).

---

## File Structure

| Path | Responsibility | Slice |
|---|---|---|
| `internal/server/transport.go` (new) | `transportConfig` + `transportConfigFromEnv` parsing/validation | A |
| `internal/server/sse.go` (new) | `sseMux` (handler) + `serveSSE` (listener + graceful shutdown) | A |
| `internal/server/server.go` (modify) | `RunWithOptions` branches stdio vs sse | A |
| `internal/credfile/credfile.go` (new) | `Store`: 0600 JSON creds/session backend implementing the 5-method surface | B |
| `internal/credfile/path.go` (new) | `resolveStateDir(getenv)` (STATE_DIR → STATE_DIRECTORY → XDG → home) | B |
| `internal/session/session.go` (modify) | export `keychainStore` → `Store`; extract `loginLocked`; add relogin | C, D |
| `internal/session/select.go` (new) | `SelectStore(getenv) (Store, error)` | C |
| `cmd/protonmail-mcp/{login,logout,status,main}.go` (modify) | use `session.SelectStore`; `status` reports backend | C |
| `docs/headless-deployment.md` (new) | systemd unit + one-time bootstrap + CAPTCHA note | C |
| `README.md` (modify) | env-var table + link to headless doc | C |

---

## Slice A — Transport seam

### Task A1: transport config parsing

**Files:**
- Create: `internal/server/transport.go`
- Test: `internal/server/transport_test.go`

- [ ] **Step 1: Write the failing test**

```go
package server

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestTransportConfigFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    transportConfig
		wantErr string
	}{
		{name: "default stdio", env: nil, want: transportConfig{kind: "stdio"}},
		{
			name: "sse with host and port",
			env:  map[string]string{"PROTONMAIL_MCP_TRANSPORT": "sse", "PROTONMAIL_MCP_PORT": "8770"},
			want: transportConfig{kind: "sse", host: "127.0.0.1", port: 8770},
		},
		{
			name: "sse custom host",
			env:  map[string]string{"PROTONMAIL_MCP_TRANSPORT": "sse", "PROTONMAIL_MCP_HOST": "0.0.0.0", "PROTONMAIL_MCP_PORT": "9000"},
			want: transportConfig{kind: "sse", host: "0.0.0.0", port: 9000},
		},
		{name: "sse missing port", env: map[string]string{"PROTONMAIL_MCP_TRANSPORT": "sse"}, wantErr: "PROTONMAIL_MCP_PORT is required"},
		{name: "invalid transport", env: map[string]string{"PROTONMAIL_MCP_TRANSPORT": "grpc"}, wantErr: `invalid PROTONMAIL_MCP_TRANSPORT "grpc"`},
		{name: "invalid port", env: map[string]string{"PROTONMAIL_MCP_TRANSPORT": "sse", "PROTONMAIL_MCP_PORT": "nope"}, wantErr: "PROTONMAIL_MCP_PORT"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := transportConfigFromEnv(env(tc.env))
			if tc.wantErr != "" {
				if err == nil || !contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && stringIndex(s, sub) >= 0) }
func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestTransportConfigFromEnv -count=1`
Expected: FAIL — `undefined: transportConfig` / `transportConfigFromEnv`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package server: transport selection from the environment.
package server

import (
	"fmt"
	"strconv"
)

type transportConfig struct {
	kind string // "stdio" | "sse"
	host string // sse only
	port int    // sse only
}

// transportConfigFromEnv reads PROTONMAIL_MCP_TRANSPORT/_HOST/_PORT. getenv is
// injected so tests need no process env. stdio is the default; sse requires an
// explicit port (binding a listener is never implicit).
func transportConfigFromEnv(getenv func(string) string) (transportConfig, error) {
	kind := getenv("PROTONMAIL_MCP_TRANSPORT")
	if kind == "" {
		kind = "stdio"
	}
	switch kind {
	case "stdio":
		return transportConfig{kind: "stdio"}, nil
	case "sse":
		host := getenv("PROTONMAIL_MCP_HOST")
		if host == "" {
			host = "127.0.0.1"
		}
		rawPort := getenv("PROTONMAIL_MCP_PORT")
		if rawPort == "" {
			return transportConfig{}, fmt.Errorf("PROTONMAIL_MCP_PORT is required when PROTONMAIL_MCP_TRANSPORT=sse")
		}
		port, err := strconv.Atoi(rawPort)
		if err != nil || port < 1 || port > 65535 {
			return transportConfig{}, fmt.Errorf("PROTONMAIL_MCP_PORT %q is not a valid port (1-65535)", rawPort)
		}
		return transportConfig{kind: "sse", host: host, port: port}, nil
	default:
		return transportConfig{}, fmt.Errorf("invalid PROTONMAIL_MCP_TRANSPORT %q (allowed: stdio, sse)", kind)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestTransportConfigFromEnv -race -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go vet ./internal/server/
git add internal/server/transport.go internal/server/transport_test.go
git commit -m "feat(server): parse SSE transport config from env"
```

### Task A2: SSE handler + serving, wired into RunWithOptions

**Files:**
- Create: `internal/server/sse.go`
- Test: `internal/server/sse_test.go`
- Modify: `internal/server/server.go:36-52`

- [ ] **Step 1: Write the failing test** (real MCP SSE client against the mux via httptest)

```go
package server_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/server"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func TestSSEMuxServesTools(t *testing.T) {
	keyring.MockInit()
	sess := session.New("https://mail.proton.me/api", keychain.New())
	srv := mcp.NewServer(&mcp.Implementation{Name: "protonmail-mcp", Version: "test"}, nil)
	server.RegisterAll(srv, sess)

	ts := httptest.NewServer(server.SSEMux(srv)) // exported test seam
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, &mcp.SSEClientTransport{Endpoint: ts.URL + "/sse"}, nil)
	if err != nil {
		t.Fatalf("sse connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	listed, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(listed.Tools) == 0 {
		t.Fatal("no tools over SSE")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestSSEMuxServesTools -count=1`
Expected: FAIL — `undefined: server.SSEMux`.

- [ ] **Step 3: Write minimal implementation** (`internal/server/sse.go`)

```go
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SSEMux returns an http.Handler serving the MCP SSE transport at /sse. One
// *mcp.Server is reused across sessions; SSEHandler calls Connect per GET.
// Exported so tests can host it via httptest. DNS-rebinding/localhost
// protection is left on (SSEOptions default) — correct for a loopback listener.
func SSEMux(srv *mcp.Server) http.Handler {
	h := mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.SSEOptions{})
	mux := http.NewServeMux()
	mux.Handle("/sse", h)
	return mux
}

// serveSSE binds host:port and serves SSEMux until ctx is cancelled, then
// shuts down gracefully.
func serveSSE(ctx context.Context, cfg transportConfig, srv *mcp.Server) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.host, cfg.port))
	if err != nil {
		return fmt.Errorf("listen %s:%d: %w", cfg.host, cfg.port, err)
	}
	httpSrv := &http.Server{Handler: SSEMux(srv), ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("sse serve: %w", err)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestSSEMuxServesTools -race -count=1`
Expected: PASS.

- [ ] **Step 5: Wire into `RunWithOptions`** — replace the final block of `internal/server/server.go` (the `srv.Run(ctx, &mcp.StdioTransport{})` tail at lines ~46-52):

```go
	srv := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: version.MCP}, nil)
	tools.Register(srv, tools.Deps{Session: sess})

	cfg, err := transportConfigFromEnv(os.Getenv)
	if err != nil {
		return fmt.Errorf("transport config: %w", err)
	}
	if cfg.kind == "sse" {
		return serveSSE(ctx, cfg, srv)
	}
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp server: %w", err)
	}
	return nil
```

- [ ] **Step 6: Run the full server suite**

Run: `go test ./internal/server/ -race -count=1 && go vet ./internal/server/`
Expected: PASS, no vet warnings. (stdio default path unchanged → existing `TestBootRegistersToolsAndDispatches` still passes.)

- [ ] **Step 7: Commit**

```bash
git add internal/server/sse.go internal/server/sse_test.go internal/server/server.go
git commit -m "feat(server): serve MCP over SSE on a loopback port"
```

---

## Slice B — File credential backend

### Task B1: state-dir resolution

**Files:**
- Create: `internal/credfile/path.go`
- Test: `internal/credfile/path_test.go`

- [ ] **Step 1: Write the failing test**

```go
package credfile

import (
	"path/filepath"
	"testing"
)

func TestResolveStateDir(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"explicit override wins", map[string]string{"PROTONMAIL_MCP_STATE_DIR": "/srv/p", "STATE_DIRECTORY": "/var/lib/x"}, "/srv/p"},
		{"systemd StateDirectory", map[string]string{"STATE_DIRECTORY": "/var/lib/protonmail-mcp"}, "/var/lib/protonmail-mcp"},
		{"xdg state home", map[string]string{"XDG_STATE_HOME": "/home/u/.local/state"}, filepath.Join("/home/u/.local/state", "protonmail-mcp")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveStateDir(func(k string) string { return tc.env[k] })
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/credfile/ -run TestResolveStateDir -count=1`
Expected: FAIL — package/`resolveStateDir` undefined.

- [ ] **Step 3: Implement** (`internal/credfile/path.go`)

```go
// Package credfile stores Proton credentials and session tokens in a
// permission-locked JSON file, for headless hosts without an OS keyring.
package credfile

import (
	"fmt"
	"os"
	"path/filepath"
)

const dirName = "protonmail-mcp"

// resolveStateDir picks the credentials directory: explicit override, then the
// systemd StateDirectory, then XDG, then the home fallback. getenv is injected
// for tests.
func resolveStateDir(getenv func(string) string) (string, error) {
	if v := getenv("PROTONMAIL_MCP_STATE_DIR"); v != "" {
		return v, nil
	}
	if v := getenv("STATE_DIRECTORY"); v != "" {
		return v, nil
	}
	if v := getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, dirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve state dir: no PROTONMAIL_MCP_STATE_DIR/STATE_DIRECTORY/XDG_STATE_HOME and home unknown: %w", err)
	}
	return filepath.Join(home, ".local", "state", dirName), nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/credfile/ -run TestResolveStateDir -race -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go vet ./internal/credfile/
git add internal/credfile/path.go internal/credfile/path_test.go
git commit -m "feat(credfile): resolve headless state directory"
```

### Task B2: Store round-trip, permissions, atomic write, not-found, Clear

**Files:**
- Create: `internal/credfile/credfile.go`
- Test: `internal/credfile/credfile_test.go`

- [ ] **Step 1: Write the failing test**

```go
package credfile

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

func newTmp(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestCredsRoundTrip(t *testing.T) {
	s := newTmp(t)
	in := keychain.Creds{Username: "u@example.test", Password: "pw", TOTPSecret: "otpauth://x"}
	if err := s.SaveCreds(in); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	got, err := s.LoadCreds()
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if got != in {
		t.Fatalf("got %+v want %+v", got, in)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	s := newTmp(t)
	in := keychain.Session{UID: "uid", AccessToken: "at", RefreshToken: "rt"}
	if err := s.SaveSession(in); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, err := s.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got != in {
		t.Fatalf("got %+v want %+v", got, in)
	}
}

func TestPermissions(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "sub")
	s, err := New(inner)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSession(keychain.Session{UID: "x"}); err != nil {
		t.Fatal(err)
	}
	di, err := os.Stat(inner)
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != fs.FileMode(0o700) {
		t.Fatalf("dir mode = %o want 700", di.Mode().Perm())
	}
	fi, err := os.Stat(filepath.Join(inner, "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != fs.FileMode(0o600) {
		t.Fatalf("file mode = %o want 600", fi.Mode().Perm())
	}
}

func TestLoadAbsentIsNotFound(t *testing.T) {
	s := newTmp(t)
	if _, err := s.LoadSession(); err == nil {
		t.Fatal("expected error loading absent session")
	}
	if _, err := s.LoadCreds(); err == nil {
		t.Fatal("expected error loading absent creds")
	}
}

func TestClearRemovesFile(t *testing.T) {
	s := newTmp(t)
	if err := s.SaveSession(keychain.Session{UID: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := s.LoadSession(); err == nil {
		t.Fatal("expected not-found after Clear")
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear on absent should be nil, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/credfile/ -run 'TestCreds|TestSession|TestPermissions|TestLoad|TestClear' -count=1`
Expected: FAIL — `undefined: Store` / `New`.

- [ ] **Step 3: Implement** (`internal/credfile/credfile.go`)

```go
package credfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

const fileName = "credentials.json"

// Store is a file-backed credential store satisfying session.Store. Both
// bundles live in one 0600 JSON document under dir (0700).
type Store struct{ dir string }

type doc struct {
	Creds   keychain.Creds   `json:"creds"`
	Session keychain.Session `json:"session"`
}

// New returns a Store rooted at dir. The directory is created lazily on first
// write so construction is side-effect-free.
func New(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("credfile: empty state dir")
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path() string { return filepath.Join(s.dir, fileName) }

func (s *Store) load() (doc, error) {
	b, err := os.ReadFile(s.path())
	if err != nil {
		return doc{}, fmt.Errorf("credfile read: %w", err) // absent → error → session maps to ErrNoSession
	}
	var d doc
	if err := json.Unmarshal(b, &d); err != nil {
		return doc{}, fmt.Errorf("credfile parse %s: %w", s.path(), err)
	}
	return d, nil
}

func (s *Store) save(d doc) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("credfile mkdir %s: %w", s.dir, err)
	}
	b, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("credfile marshal: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, fileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("credfile temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("credfile chmod: %w", err)
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("credfile write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("credfile sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("credfile close: %w", err)
	}
	if err := os.Rename(tmpName, s.path()); err != nil {
		return fmt.Errorf("credfile rename: %w", err)
	}
	return nil
}

// merge loads the current doc (tolerating absence) and applies fn.
func (s *Store) merge(fn func(*doc)) error {
	d, err := s.load()
	if err != nil && !os.IsNotExist(errors.Unwrap(err)) {
		// parse/permission error on an existing file: surface it.
		if _, statErr := os.Stat(s.path()); statErr == nil {
			return err
		}
	}
	fn(&d)
	return s.save(d)
}

func (s *Store) SaveCreds(c keychain.Creds) error {
	return s.merge(func(d *doc) { d.Creds = c })
}

func (s *Store) LoadCreds() (keychain.Creds, error) {
	d, err := s.load()
	if err != nil {
		return keychain.Creds{}, err
	}
	return d.Creds, nil
}

func (s *Store) SaveSession(sess keychain.Session) error {
	return s.merge(func(d *doc) { d.Session = sess })
}

func (s *Store) LoadSession() (keychain.Session, error) {
	d, err := s.load()
	if err != nil {
		return keychain.Session{}, err
	}
	return d.Session, nil
}

// Clear removes the credentials file. Absent file is not an error.
func (s *Store) Clear() error {
	if err := os.Remove(s.path()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("credfile clear: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/credfile/ -race -count=1`
Expected: PASS (all five tests).

- [ ] **Step 5: Commit**

```bash
go vet ./internal/credfile/
git add internal/credfile/credfile.go internal/credfile/credfile_test.go
git commit -m "feat(credfile): 0600 file credential backend"
```

---

## Slice C — Backend selection + bootstrap (depends on B)

### Task C1: export `session.Store`

**Files:**
- Modify: `internal/session/session.go:24-32,118`

- [ ] **Step 1: Rename the interface and its parameter use.** In `internal/session/session.go` replace the `keychainStore` interface declaration with:

```go
// Store is the persistence surface Session needs. *keychain.Keychain and
// *credfile.Store both satisfy it. Tests inject failure-injecting wrappers.
type Store interface {
	SaveCreds(keychain.Creds) error
	LoadCreds() (keychain.Creds, error)
	SaveSession(keychain.Session) error
	LoadSession() (keychain.Session, error)
	Clear() error
}
```

Then update the field and constructor:
- `kc keychainStore` → `kc Store` (struct field, ~line 39)
- `func New(apiURL string, kc keychainStore, opts ...Option)` → `func New(apiURL string, kc Store, opts ...Option)` (~line 118)

- [ ] **Step 2: Find any other `keychainStore` references**

Run: `rg -n "keychainStore" internal/ cmd/`
Expected: zero matches after renaming (tests pass concrete types positionally, so none should reference the alias). If a test references it, rename there too.

- [ ] **Step 3: Run the full suite (refactor — behavior unchanged)**

Run: `go test ./... -race -count=1 && go vet ./...`
Expected: PASS, no warnings.

- [ ] **Step 4: Commit**

```bash
git add internal/session/session.go
git commit -m "refactor(session): export Store interface"
```

### Task C2: `SelectStore` + wire into the four sites

**Files:**
- Create: `internal/session/select.go`
- Test: `internal/session/select_test.go`
- Modify: `internal/server/server.go:45`, `cmd/protonmail-mcp/{login,logout,status}.go`

- [ ] **Step 1: Write the failing test**

```go
package session_test

import (
	"testing"

	"github.com/zalando/go-keyring"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func TestSelectStore(t *testing.T) {
	keyring.MockInit()
	get := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	t.Run("default keychain", func(t *testing.T) {
		s, err := session.SelectStore(get(nil))
		if err != nil || s == nil {
			t.Fatalf("got %v, %v", s, err)
		}
	})
	t.Run("file backend", func(t *testing.T) {
		s, err := session.SelectStore(get(map[string]string{
			"PROTONMAIL_MCP_CREDENTIAL_BACKEND": "file",
			"PROTONMAIL_MCP_STATE_DIR":          t.TempDir(),
		}))
		if err != nil || s == nil {
			t.Fatalf("got %v, %v", s, err)
		}
	})
	t.Run("invalid backend", func(t *testing.T) {
		if _, err := session.SelectStore(get(map[string]string{"PROTONMAIL_MCP_CREDENTIAL_BACKEND": "vault"})); err == nil {
			t.Fatal("expected error")
		}
	})
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/session/ -run TestSelectStore -count=1`
Expected: FAIL — `undefined: session.SelectStore`.

- [ ] **Step 3: Implement** (`internal/session/select.go`)

```go
package session

import (
	"fmt"

	"github.com/millsmillsymills/protonmail-mcp/internal/credfile"
	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

// SelectStore picks the credential backend from
// PROTONMAIL_MCP_CREDENTIAL_BACKEND ("keychain" default | "file"). getenv is
// injected so callers pass os.Getenv (or a test stub).
func SelectStore(getenv func(string) string) (Store, error) {
	switch backend := getenv("PROTONMAIL_MCP_CREDENTIAL_BACKEND"); backend {
	case "", "keychain":
		return keychain.New(), nil
	case "file":
		dir, err := credfile.ResolveStateDir(getenv)
		if err != nil {
			return nil, err
		}
		return credfile.New(dir)
	default:
		return nil, fmt.Errorf("invalid PROTONMAIL_MCP_CREDENTIAL_BACKEND %q (allowed: keychain, file)", backend)
	}
}
```

- [ ] **Step 4: Export `ResolveStateDir`.** In `internal/credfile/path.go` rename `resolveStateDir` → `ResolveStateDir` (and update `path_test.go`'s call). Run: `go test ./internal/credfile/ -race -count=1` → PASS.

- [ ] **Step 5: Run to verify C2 test passes**

Run: `go test ./internal/session/ -run TestSelectStore -race -count=1`
Expected: PASS.

- [ ] **Step 6: Wire the four production sites.** Replace each `keychain.New()` passed to `session.New` with the selected store. In `internal/server/server.go` (~line 45):

```go
	store, err := session.SelectStore(os.Getenv)
	if err != nil {
		return fmt.Errorf("credential backend: %w", err)
	}
	sess := session.New(apiURL, store, session.WithTransport(transport))
```

In each of `cmd/protonmail-mcp/login.go`, `logout.go`, `status.go`, replace the `sess := session.New(apiURL, keychain.New(), session.WithTransport(transport))` line with:

```go
	store, err := session.SelectStore(os.Getenv)
	if err != nil {
		return fmt.Errorf("credential backend: %w", err)
	}
	sess := session.New(apiURL, store, session.WithTransport(transport))
```

Add `"os"` to imports where missing and drop the now-unused `keychain` import in those files (run `goimports -w` on each).

- [ ] **Step 7: Run the full suite**

Run: `go test ./... -race -count=1 && go vet ./...`
Expected: PASS, no warnings. (Production paths default to keychain when the env var is unset → existing tests unaffected.)

- [ ] **Step 8: Commit**

```bash
git add internal/session/select.go internal/session/select_test.go internal/credfile/ internal/server/server.go cmd/protonmail-mcp/login.go cmd/protonmail-mcp/logout.go cmd/protonmail-mcp/status.go
git commit -m "feat(session): select credential backend from env"
```

### Task C3: status reports backend + headless docs

**Files:**
- Modify: `cmd/protonmail-mcp/status.go`
- Create: `docs/headless-deployment.md`
- Modify: `README.md`

- [ ] **Step 1: Write the failing test** — new `cmd/protonmail-mcp/status_backend_test.go`. Reads the backend from process env (same source `SelectStore(os.Getenv)` uses), so no `env`-slice threading. Asserts only the substring (no-session exit semantics are out of scope here):

```go
package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestStatusReportsFileBackend(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_CREDENTIAL_BACKEND", "file")
	t.Setenv("PROTONMAIL_MCP_STATE_DIR", t.TempDir())
	var out bytes.Buffer
	_ = run(context.Background(), []string{"status"}, os.Environ(),
		strings.NewReader(""), &out, &out, nil)
	if !strings.Contains(out.String(), "backend: file") {
		t.Fatalf("status did not report backend; got: %s", out.String())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/protonmail-mcp/ -run TestStatusReportsFileBackend -count=1`
Expected: FAIL — no "backend: file" in output.

- [ ] **Step 3: Implement** — in `runStatus` (`cmd/protonmail-mcp/status.go`), print the active backend first, reading the same source `SelectStore` uses (`os.Getenv`), so report and selection can't diverge. No signature change:

```go
	backend := os.Getenv("PROTONMAIL_MCP_CREDENTIAL_BACKEND")
	if backend == "" {
		backend = "keychain"
	}
	_, _ = fmt.Fprintf(stdout, "backend: %s\n", backend)
```

Add `"os"` to the `status.go` imports if absent. Print this before any session/network work so it appears even when logged out.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cmd/protonmail-mcp/ -run TestStatusReportsFileBackend -race -count=1`
Expected: PASS.

- [ ] **Step 5: Write the headless deployment doc** (`docs/headless-deployment.md`):

````markdown
# Headless Linux deployment

Run `protonmail-mcp` as a loopback SSE service with a file credential backend.

## 1. One-time bootstrap (interactive, on the box)

Run `login` once as the service user, writing to the file backend. **At the 2FA
prompt, paste the `otpauth://` secret URI (not a 6-digit code)** so the service
can self-heal later:

```bash
sudo -u interceptor-mcp env \
  PROTONMAIL_MCP_CREDENTIAL_BACKEND=file \
  PROTONMAIL_MCP_STATE_DIR=/var/lib/protonmail-mcp \
  protonmail-mcp login
```

Verify: `... protonmail-mcp status` prints `backend: file` and a valid session.

## 2. systemd unit (`/etc/systemd/system/protonmail-mcp.service`)

```ini
[Unit]
Description=protonmail-mcp (headless SSE)
After=network-online.target

[Service]
User=interceptor-mcp
StateDirectory=protonmail-mcp
Environment=PROTONMAIL_MCP_TRANSPORT=sse
Environment=PROTONMAIL_MCP_HOST=127.0.0.1
Environment=PROTONMAIL_MCP_PORT=8770
Environment=PROTONMAIL_MCP_CREDENTIAL_BACKEND=file
ExecStart=/usr/local/bin/protonmail-mcp
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

`StateDirectory=protonmail-mcp` makes systemd provide `/var/lib/protonmail-mcp`
(0700, owned by the service user) and sets `$STATE_DIRECTORY` — so the bootstrap
in step 1 and the service resolve the same path.

## 3. Self-heal and its limit

If the refresh token is revoked, the service re-logins from the stored
credentials (TOTP generated from the stored secret) on the next call. **If Proton
answers with a CAPTCHA challenge, it cannot self-heal** — re-run step 1 manually.
````

- [ ] **Step 6: Update README** — add the env-var table from the spec's "Interface surface" section and a link to `docs/headless-deployment.md`.

- [ ] **Step 7: Commit**

```bash
go vet ./cmd/protonmail-mcp/
git add cmd/protonmail-mcp/status.go cmd/protonmail-mcp/status_backend_test.go docs/headless-deployment.md README.md
git commit -m "feat(cli): report backend in status; document headless deploy"
```

---

## Slice D — Unattended self-heal (depends on B/C)

### Task D1: extract `loginLocked` (refactor, no behavior change)

**Files:**
- Modify: `internal/session/session.go:313-388`

- [ ] **Step 1: Extract the body.** Rename the current `Login` body into a lock-free `loginLocked` and make `Login` a thin wrapper. `loginLocked` assumes `s.mu` is already held:

```go
func (s *Session) Login(ctx context.Context, in LoginInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loginLocked(ctx, in)
}

// loginLocked performs password+2FA auth and persists state. Caller MUST hold
// s.mu (so the cold-start refresh path can reuse it without re-locking).
func (s *Session) loginLocked(ctx context.Context, in LoginInput) error {
	// ... the existing body of Login from `c, auth, err := s.mgr.NewClientWithLogin`
	// through the final `return nil`, with the leading `s.mu.Lock(); defer s.mu.Unlock()`
	// REMOVED.
}
```

- [ ] **Step 2: Run the existing login suite (behavior unchanged)**

Run: `go test ./internal/session/ ./cmd/protonmail-mcp/ -race -count=1 && go vet ./...`
Expected: PASS — all existing login/2FA tests still green.

- [ ] **Step 3: Commit**

```bash
git add internal/session/session.go
git commit -m "refactor(session): extract loginLocked for lock reuse"
```

### Task D2: auto-relogin on cold-start refresh failure

**Files:**
- Modify: `internal/session/session.go:204-214` (the `NewClientWithRefresh` error branch)
- Test: add a record-cassette scenario + a `testvcr`-backed test (`internal/session/relogin_test.go`)

- [ ] **Step 1: Write the failing test.** The relogin path is exercised against a cassette where `/auth/v4/refresh` is rejected and a subsequent password+2FA login succeeds. Reuse the `testvcr` pattern (see `internal/server/server_test.go`) and the existing `refresh_revoked` recorder scenario as the starting point.

```go
package session_test

import (
	"context"
	"testing"

	"github.com/zalando/go-keyring"
	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func TestColdStartReloginsFromStoredCreds(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	// Stored creds enable self-heal; stored session has a refresh token the
	// cassette rejects, forcing the relogin path.
	if err := kc.SaveCreds(keychain.Creds{Username: "u@example.test", Password: "pw", TOTPSecret: "REDACTED_TOTP_SECRET_1"}); err != nil {
		t.Fatal(err)
	}
	if err := kc.SaveSession(keychain.Session{UID: "REDACTED_UID_1", AccessToken: "stale", RefreshToken: "rejected"}); err != nil {
		t.Fatal(err)
	}
	rt := testvcr.New(t, "relogin_after_refresh_reject")
	sess := session.New("https://mail.proton.me/api", kc, session.WithTransport(rt))

	if _, err := sess.Client(context.Background()); err != nil {
		t.Fatalf("expected self-heal, got: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/session/ -run TestColdStartReloginsFromStoredCreds -count=1`
Expected: FAIL — refresh rejected, no relogin → `proton/auth_required` error (and/or missing cassette).

- [ ] **Step 3: Record the cassette.** Add a scenario to `cmd/record-cassettes/scenarios/` modeled on `refresh_revoked.go` that: seeds creds+session, issues an authenticated call so the rejected refresh triggers relogin, and records the `/auth/v4/refresh` (reject) → `/auth/v4/...` (login+2FA success) exchange as `relogin_after_refresh_reject`. This requires real Proton access and runs through the existing recorder harness (same manual gate the spec calls out). Scrub per the recorder's allowlist.

- [ ] **Step 4: Implement the relogin branch.** Replace the `NewClientWithRefresh` error handling at `session.go:204-214` with:

```go
	c, refreshed, err := s.mgr.NewClientWithRefresh(ctx, sess.UID, sess.RefreshToken)
	if err != nil {
		if pe := proterr.Map(err); pe != nil && pe.Code == "proton/auth_required" {
			// Refresh token rejected. Attempt unattended relogin from stored
			// creds (headless self-heal). loginLocked sets s.client/current/raw
			// on success; we already hold s.mu.
			creds, lerr := s.kc.LoadCreds()
			if lerr == nil && creds.Username != "" && creds.Password != "" {
				if rerr := s.loginLocked(ctx, LoginInput{
					Username:   creds.Username,
					Password:   creds.Password,
					TOTPSecret: creds.TOTPSecret,
				}); rerr == nil {
					return s.client, nil
				} else if cpe := proterr.Map(rerr); cpe != nil && cpe.Code == "proton/captcha" {
					return nil, cpe // CAPTCHA ceiling: cannot self-heal, surface clearly.
				}
				// other relogin failure: fall through to auth_required below.
			}
			return nil, pe
		}
		return nil, fmt.Errorf("refresh session: %w", err)
	}
```

(The existing happy-path lines below — `c.AddAuthHandler(...)` through `return c, nil` — are unchanged.)

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/session/ -run TestColdStartReloginsFromStoredCreds -race -count=1`
Expected: PASS.

- [ ] **Step 6: Run the full suite**

Run: `go test ./... -race -count=1 && go vet ./...`
Expected: PASS, no warnings.

- [ ] **Step 7: Commit**

```bash
git add internal/session/session.go internal/session/relogin_test.go cmd/record-cassettes/scenarios/ internal/session/testdata/
git commit -m "feat(session): self-heal via relogin on refresh rejection"
```

---

## Final verification (after all slices)

- [ ] `go test ./... -race -count=1` — all pass.
- [ ] `go test -tags=integration ./... -race -count=1` — all pass.
- [ ] `go vet ./...` and `golangci-lint run` — no warnings.
- [ ] Manual (operator, real headless host — spec's documented gate): bootstrap on a `--no-create-home` user with no D-Bus, start the systemd unit, connect a client to `127.0.0.1:8770/sse`, list tools; revoke the session server-side and confirm a subsequent call self-heals (or surfaces the CAPTCHA manual-relogin error).
- [ ] Open the four tracer PRs (A, B, C, D) against `main`; each references #108.

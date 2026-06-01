# Phase 0: Keyring Unlock + Decrypt Tracer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unlock the user + address PGP keyrings from the stored mailbox password and prove it end-to-end by decrypting one message body, while capturing the mailbox password for two-password-mode accounts.

**Architecture:** A new `internal/keyring` package turns fetched Proton salts/keys + the mailbox password into unlocked `*crypto.KeyRing`s and decrypts message bodies. `Session` owns a lazily-populated, session-lifetime keyring cache (cleared on logout/relogin) and exposes `UserKeyRing`/`AddrKeyRing`. Login detects two-password mode and persists a distinct mailbox password; the credential store gains an optional `MailboxPassword` field whose absence means "reuse the login password" (migration-tolerant).

**Tech Stack:** Go, `github.com/ProtonMail/go-proton-api` (pinned), `github.com/ProtonMail/gopenpgp/v2 v2.10.0`, `github.com/modelcontextprotocol/go-sdk`, go-keyring (mocked in tests).

---

## Spec reference

Implements Phase 0 of `docs/superpowers/specs/2026-05-29-feature-parity-roadmap-design.md`:
keyring lifecycle (session-lifetime lazy unlock, never persisted/logged), two-password
mode + credential migration, and the single-decrypt tracer.

## Verified upstream signatures (do not re-derive)

```go
// go-proton-api
func (c *Client) GetSalts(ctx context.Context) (Salts, error)
func (c *Client) GetUser(ctx context.Context) (User, error)        // User.Keys is Keys
func (c *Client) GetAddresses(ctx context.Context) ([]Address, error) // Address.Keys is Keys
func (salts Salts) SaltForKey(keyPass []byte, keyID string) ([]byte, error)
func (keys Keys) Unlock(passphrase []byte, userKR *crypto.KeyRing) (*crypto.KeyRing, error)
func (m Message) Decrypt(kr *crypto.KeyRing) ([]byte, error)        // Message.Body, Message.AddressID
type PasswordMode int
const ( OnePasswordMode PasswordMode = iota + 1; TwoPasswordMode )  // Auth.PasswordMode
func (keys Keys) Primary() (Key, bool)                             // Key.ID
```

User keyring: `User.Keys.Unlock(saltedKeyPass, nil)`. Address keyring:
`Address.Keys.Unlock(saltedKeyPass, userKR)` — the user keyring decrypts token-locked
address keys. `saltedKeyPass = salts.SaltForKey(mailboxPassword, user.Keys-primary-ID)`.

## File structure

- Modify `internal/keychain/keychain.go` — add `MailboxPassword` to `Creds`; storage key; Save/Load/Clear.
- Modify `internal/credfile/credfile.go` — JSON gains the field automatically; add an explicit test.
- Modify `internal/session/session.go` — `LoginInput.MailboxPassword`, `ErrMailboxPasswordRequired`, two-password detection, keyring cache + helpers, cache-clear on logout/relogin.
- Create `internal/keyring/keyring.go` — `Unlock`, `Keyrings`, `KeyFetcher`, `DecryptBody`.
- Create `internal/keyring/keyring_test.go` — synthetic-key decrypt + orchestration tests.
- Modify `internal/tools/messages.go` — `include_body` on `proton_get_message`.
- Modify `internal/log/*` (redaction set) and `README.md` — redact keyring material; update status table.

## Testing strategy (read before Task 3)

A decryption test **cannot** use a scrubbed cassette: scrubbing the keys breaks the math.
Two honest layers instead:

1. **Automated** — `internal/keyring` is unit-tested with a **locally generated gopenpgp
   keypair** (no network, no Proton account): generate a key, lock it with a known
   passphrase, encrypt a body to it, then prove `DecryptBody` recovers the plaintext
   through an unlocked keyring. Orchestration/caching/error paths use a fake `KeyFetcher`.
2. **Live (manual)** — the end-to-end "decrypt a real Proton message" tracer is verified
   by hand against the test account and recorded in the task's checklist. We do **not**
   commit a real private key or an unscrubbed body.

---

## Task 1: Add optional MailboxPassword to the credential store

**Files:**
- Modify: `internal/keychain/keychain.go`
- Test: `internal/keychain/keychain_test.go`
- Test: `internal/credfile/credfile_test.go`

- [ ] **Step 1: Write the failing test** (append to `internal/keychain/keychain_test.go`)

```go
func TestSaveLoadCredsRoundTripsMailboxPassword(t *testing.T) {
	keyring.MockInit()
	k := New()
	want := Creds{Username: "u@example.test", Password: "login-pw", MailboxPassword: "mbox-pw"}
	if err := k.SaveCreds(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := k.LoadCreds()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.MailboxPassword != "mbox-pw" {
		t.Fatalf("MailboxPassword = %q, want %q", got.MailboxPassword, "mbox-pw")
	}
}

func TestLoadCredsToleratesMissingMailboxPassword(t *testing.T) {
	keyring.MockInit()
	k := New()
	// Simulate a pre-migration bundle: username + password only, no mailbox key.
	if err := keyringSet(service, keyUsername, "u@example.test"); err != nil {
		t.Fatal(err)
	}
	if err := keyringSet(service, keyPassword, "login-pw"); err != nil {
		t.Fatal(err)
	}
	got, err := k.LoadCreds()
	if err != nil {
		t.Fatalf("load must tolerate absent mailbox password: %v", err)
	}
	if got.MailboxPassword != "" {
		t.Fatalf("MailboxPassword = %q, want empty", got.MailboxPassword)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/keychain/ -run TestSaveLoadCredsRoundTripsMailboxPassword -v`
Expected: FAIL — `Creds` has no field `MailboxPassword`.

- [ ] **Step 3: Implement** (edit `internal/keychain/keychain.go`)

Add the storage key constant alongside the others:

```go
	keyTOTPSecret   = "totp_secret"
	keyMailboxPass  = "mailbox_password"
```

Extend `Creds`:

```go
type Creds struct {
	Username   string
	Password   string
	TOTPSecret string
	// MailboxPassword is set only for two-password-mode accounts. Empty means
	// the login Password doubles as the mailbox password (one-password mode).
	MailboxPassword string
}
```

In `SaveCreds`, after the TOTP block's final `return nil` is removed, persist the mailbox
password the same optional way TOTP is handled. Replace the TOTP tail of `SaveCreds` with:

```go
	if err := saveOptional(keyTOTPSecret, c.TOTPSecret); err != nil {
		return err
	}
	return saveOptional(keyMailboxPass, c.MailboxPassword)
}

// saveOptional sets key to value, or deletes any stale entry when value is
// empty so a secret from a prior login cannot bleed through.
func saveOptional(key, value string) error {
	if value == "" {
		if err := keyringDelete(service, key); err != nil &&
			!errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("clear stale %s: %w", key, diagnoseKeychainErr(err))
		}
		return nil
	}
	if err := keyringSet(service, key, value); err != nil {
		return fmt.Errorf("save %s: %w", key, diagnoseKeychainErr(err))
	}
	return nil
}
```

In `LoadCreds`, after loading TOTP, load the mailbox password tolerantly and include it:

```go
	m, err := keyringGet(service, keyMailboxPass)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return Creds{}, fmt.Errorf("load mailbox password: %w", diagnoseKeychainErr(err))
	}
	return Creds{Username: u, Password: p, TOTPSecret: t, MailboxPassword: m}, nil
```

In `Clear`, add the key to the slice:

```go
	keys := []string{keyUsername, keyPassword, keyTOTPSecret, keyMailboxPass, keyUID, keyAccessToken, keyRefreshToken}
```

- [ ] **Step 4: Add the credfile coverage test** (`internal/credfile/credfile_test.go`)

```go
func TestCredfileRoundTripsMailboxPassword(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir) // match the existing constructor used elsewhere in this test file
	want := keychain.Creds{Username: "u@example.test", Password: "login-pw", MailboxPassword: "mbox-pw"}
	if err := s.SaveCreds(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.LoadCreds()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.MailboxPassword != "mbox-pw" {
		t.Fatalf("MailboxPassword = %q, want %q", got.MailboxPassword, "mbox-pw")
	}
}
```

(Verify the constructor name against the top of `credfile_test.go` and match it; the field
serializes automatically because `doc.Creds` is `keychain.Creds` with default JSON.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/keychain/ ./internal/credfile/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/keychain/keychain.go internal/keychain/keychain_test.go internal/credfile/credfile_test.go
git commit -m "feat(keychain): store optional mailbox password for two-password accounts"
```

---

## Task 2: Detect two-password mode and capture the mailbox password at login

**Files:**
- Modify: `internal/session/session.go`
- Test: `internal/session/login_persist_test.go`

- [ ] **Step 1: Write the failing test** (append to `internal/session/login_persist_test.go`)

```go
func TestErrMailboxPasswordRequiredIsDistinct(t *testing.T) {
	// Guards the sentinel used by the CLI to branch into a mailbox-password
	// prompt instead of treating it as a generic auth failure.
	if !errors.Is(ErrMailboxPasswordRequired, ErrMailboxPasswordRequired) {
		t.Fatal("sentinel must match itself")
	}
	if errors.Is(ErrMailboxPasswordRequired, ErrTOTPRequired) {
		t.Fatal("mailbox-password and TOTP sentinels must be distinct")
	}
}

func TestLoginInputCarriesMailboxPassword(t *testing.T) {
	in := LoginInput{Username: "u@example.test", Password: "login", MailboxPassword: "mbox"}
	if in.MailboxPassword != "mbox" {
		t.Fatalf("MailboxPassword = %q, want %q", in.MailboxPassword, "mbox")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/session/ -run TestErrMailboxPasswordRequiredIsDistinct -v`
Expected: FAIL — `ErrMailboxPasswordRequired` undefined, `LoginInput` has no `MailboxPassword`.

- [ ] **Step 3: Implement** (edit `internal/session/session.go`)

Add the sentinel beside `ErrTOTPRequired`:

```go
// ErrMailboxPasswordRequired is returned from Login when the account uses
// two-password mode but LoginInput supplied no MailboxPassword. Callers should
// use errors.Is to branch into a mailbox-password prompt.
var ErrMailboxPasswordRequired = errors.New("mailbox password required (two-password mode) but none provided")
```

Add the field to `LoginInput`:

```go
type LoginInput struct {
	Username        string
	Password        string
	TOTPSecret      string
	TOTPCode        string
	MailboxPassword string // required only in two-password mode
}
```

In `loginLocked`, after the 2FA block and before building `next`, enforce two-password mode
and choose the mailbox password to persist:

```go
	mailboxPassword := in.MailboxPassword
	if auth.PasswordMode == proton.TwoPasswordMode && mailboxPassword == "" {
		c.Close()
		return ErrMailboxPasswordRequired
	}
	// One-password mode reuses the login password; persist it empty so the
	// unlock path falls back to Password (keeps existing accounts migration-free).
	if auth.PasswordMode == proton.OnePasswordMode {
		mailboxPassword = ""
	}
```

Thread it through `persistLoginState`'s creds:

```go
	if err := s.persistLoginState(keychain.Creds{
		Username:        in.Username,
		Password:        in.Password,
		TOTPSecret:      in.TOTPSecret,
		MailboxPassword: mailboxPassword,
	}, next); err != nil {
```

In `reloginLocked`, pass the stored mailbox password through so self-heal works for
two-password accounts:

```go
	rerr := s.loginLocked(ctx, LoginInput{
		Username:        creds.Username,
		Password:        creds.Password,
		TOTPSecret:      creds.TOTPSecret,
		MailboxPassword: creds.MailboxPassword,
	})
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ -run 'TestErrMailboxPasswordRequiredIsDistinct|TestLoginInputCarriesMailboxPassword' -v`
Expected: PASS. Then `go build ./...` to confirm the `auth.PasswordMode` references compile.

- [ ] **Step 5: Commit**

```bash
git add internal/session/session.go internal/session/login_persist_test.go
git commit -m "feat(session): capture mailbox password for two-password-mode login"
```

---

## Task 3: internal/keyring — unlock + decrypt

**Files:**
- Create: `internal/keyring/keyring.go`
- Create: `internal/keyring/keyring_test.go`

- [ ] **Step 1: Write the failing test** (`internal/keyring/keyring_test.go`)

Uses a locally generated gopenpgp keypair — no network. Proves `DecryptBody` recovers a
plaintext encrypted to the address key, through a `Keyrings` value.

```go
package keyring

import (
	"testing"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

// newTestKeyRing generates an unlocked in-memory keyring for tests.
func newTestKeyRing(t *testing.T) *crypto.KeyRing {
	t.Helper()
	key, err := crypto.GenerateKey("tester", "tester@example.test", "x25519", 0)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	kr, err := crypto.NewKeyRing(key)
	if err != nil {
		t.Fatalf("new keyring: %v", err)
	}
	return kr
}

func TestDecryptBodyRecoversPlaintext(t *testing.T) {
	kr := newTestKeyRing(t)
	plaintext := "delivery confirmed"
	armored, err := kr.Encrypt(crypto.NewPlainMessageFromString(plaintext), kr)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	pgp, err := armored.GetArmored()
	if err != nil {
		t.Fatalf("armor: %v", err)
	}

	krs := &Keyrings{User: kr, Addr: map[string]*crypto.KeyRing{"addr-1": kr}}
	got, err := krs.DecryptBody("addr-1", pgp)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plaintext {
		t.Fatalf("DecryptBody = %q, want %q", got, plaintext)
	}
}

func TestDecryptBodyUnknownAddressErrors(t *testing.T) {
	krs := &Keyrings{User: nil, Addr: map[string]*crypto.KeyRing{}}
	if _, err := krs.DecryptBody("missing", "irrelevant"); err == nil {
		t.Fatal("expected error for unknown address ID")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/keyring/ -v`
Expected: FAIL — package/types undefined.

- [ ] **Step 3: Implement** (`internal/keyring/keyring.go`)

```go
// Package keyring turns fetched Proton salts/keys plus the mailbox password
// into unlocked PGP keyrings and decrypts message bodies. Unlocked keyrings
// hold decrypted private key material: never persist or log a *Keyrings.
package keyring

import (
	"context"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

// KeyFetcher is the subset of *proton.Client the unlocker needs. *proton.Client
// satisfies it; tests inject fakes.
type KeyFetcher interface {
	GetSalts(ctx context.Context) (proton.Salts, error)
	GetUser(ctx context.Context) (proton.User, error)
	GetAddresses(ctx context.Context) ([]proton.Address, error)
}

// Keyrings holds the unlocked user keyring and one keyring per address ID.
type Keyrings struct {
	User *crypto.KeyRing
	Addr map[string]*crypto.KeyRing
}

// Unlock derives the salted mailbox passphrase, unlocks the user keyring, then
// unlocks each address keyring against it. mailboxPassword must be the actual
// mailbox password (login password for one-password-mode accounts).
func Unlock(ctx context.Context, f KeyFetcher, mailboxPassword []byte) (*Keyrings, error) {
	salts, err := f.GetSalts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get salts: %w", err)
	}
	user, err := f.GetUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	primary, ok := user.Keys.Primary()
	if !ok {
		return nil, fmt.Errorf("user has no primary key")
	}
	saltedKeyPass, err := salts.SaltForKey(mailboxPassword, primary.ID)
	if err != nil {
		return nil, fmt.Errorf("salt for key: %w", err)
	}
	userKR, err := user.Keys.Unlock(saltedKeyPass, nil)
	if err != nil {
		return nil, fmt.Errorf("unlock user keyring: %w", err)
	}
	addrs, err := f.GetAddresses(ctx)
	if err != nil {
		return nil, fmt.Errorf("get addresses: %w", err)
	}
	addrKRs := make(map[string]*crypto.KeyRing, len(addrs))
	for _, a := range addrs {
		kr, err := a.Keys.Unlock(saltedKeyPass, userKR)
		if err != nil {
			// A disabled address with no usable key is not fatal — skip it so a
			// single bad address can't block decryption for all the others.
			continue
		}
		addrKRs[a.ID] = kr
	}
	return &Keyrings{User: userKR, Addr: addrKRs}, nil
}

// DecryptBody decrypts an armored PGP body using the keyring for addrID.
func (k *Keyrings) DecryptBody(addrID, armoredBody string) (string, error) {
	kr, ok := k.Addr[addrID]
	if !ok {
		return "", fmt.Errorf("no unlocked keyring for address %s", addrID)
	}
	msg, err := crypto.NewPGPMessageFromArmored(armoredBody)
	if err != nil {
		return "", fmt.Errorf("parse armored body: %w", err)
	}
	plain, err := kr.Decrypt(msg, nil, 0)
	if err != nil {
		return "", fmt.Errorf("decrypt body: %w", err)
	}
	return plain.GetString(), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/keyring/ -v`
Expected: PASS. If `crypto.GenerateKey`/`Encrypt`/`Decrypt` arities differ in
gopenpgp v2.10.0, adjust the call to match the compiler error — the types above are from
v2.10.0's `crypto` package.

- [ ] **Step 5: Commit**

```bash
git add internal/keyring/
git commit -m "feat(keyring): unlock user/address keyrings and decrypt message bodies"
```

---

## Task 4: Session keyring cache (lazy, cleared on logout/relogin)

**Files:**
- Modify: `internal/session/session.go`
- Test: `internal/session/keyring_cache_test.go` (create)

- [ ] **Step 1: Write the failing test** (`internal/session/keyring_cache_test.go`)

```go
package session

import "testing"

func TestKeyringCacheClearedByLogout(t *testing.T) {
	s := &Session{kc: nil, raw: newRawClient("http://invalid.test", nil)}
	s.keyrings = &keyringHandle{} // pretend a prior unlock populated it
	s.clearKeyringCache()
	if s.keyrings != nil {
		t.Fatal("keyring cache must be nil after clear")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/session/ -run TestKeyringCacheClearedByLogout -v`
Expected: FAIL — `s.keyrings`, `keyringHandle`, `clearKeyringCache` undefined.

- [ ] **Step 3: Implement** (edit `internal/session/session.go`)

Add the import:

```go
	"github.com/millsmillsymills/protonmail-mcp/internal/keyring"
```

Add a cache field to `Session` (beside `current`):

```go
	// keyrings is the lazily-unlocked, session-lifetime PGP keyring cache.
	// Holds decrypted private key material; nil until first crypto use and
	// dropped on logout/relogin. Never persisted, never logged.
	keyrings *keyringHandle
```

Add the handle type and helpers near the bottom of the file:

```go
type keyringHandle struct {
	krs *keyring.Keyrings
}

// clearKeyringCache drops the unlocked keyrings. Caller should hold s.mu when
// reachable from a locked path; Logout already does.
func (s *Session) clearKeyringCache() {
	s.keyrings = nil
}

// Keyrings returns the session keyrings, unlocking them on first use and
// caching the result for the session lifetime.
func (s *Session) Keyrings(ctx context.Context) (*keyring.Keyrings, error) {
	s.mu.RLock()
	if s.keyrings != nil {
		h := s.keyrings
		s.mu.RUnlock()
		return h.krs, nil
	}
	s.mu.RUnlock()

	c, err := s.Client(ctx)
	if err != nil {
		return nil, err
	}
	creds, err := s.kc.LoadCreds()
	if err != nil {
		return nil, fmt.Errorf("load creds for keyring unlock: %w", err)
	}
	mailbox := creds.MailboxPassword
	if mailbox == "" {
		mailbox = creds.Password // one-password-mode accounts
	}
	krs, err := keyring.Unlock(ctx, c, []byte(mailbox))
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.keyrings = &keyringHandle{krs: krs}
	s.mu.Unlock()
	return krs, nil
}
```

In `Logout`, drop the cache (add before the final `return nil`):

```go
	s.clearKeyringCache()
```

In `loginLocked`, drop any stale cache on a successful relogin (add beside
`s.reloginExhausted = false`):

```go
	s.clearKeyringCache()
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ -run TestKeyringCacheClearedByLogout -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/session/session.go internal/session/keyring_cache_test.go
git commit -m "feat(session): lazy session-lifetime keyring cache"
```

---

## Task 5: Tracer — decrypt body via proton_get_message (include_body)

**Files:**
- Modify: `internal/tools/messages.go`
- Test: `internal/tools/messages_test.go` (add a unit test for the field plumbing)

- [ ] **Step 1: Write the failing test** (append to `internal/tools/messages_test.go`)

```go
func TestGetMessageInHasIncludeBody(t *testing.T) {
	in := getMessageIn{ID: "m1", IncludeBody: true}
	if !in.IncludeBody {
		t.Fatal("IncludeBody field missing")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tools/ -run TestGetMessageInHasIncludeBody -v`
Expected: FAIL — `getMessageIn` has no `IncludeBody`.

- [ ] **Step 3: Implement** (edit `internal/tools/messages.go`)

Add the input flag and output field:

```go
type getMessageIn struct {
	ID             string `json:"id"`
	IncludeHeaders bool   `json:"include_headers,omitempty" jsonschema:"if true, return the full raw RFC822 header block + parsed headers"`
	IncludeBody    bool   `json:"include_body,omitempty" jsonschema:"if true, decrypt and return the plaintext body (requires an unlocked keyring)"`
}
type getMessageOut struct {
	Message       messageStubDTO      `json:"message"`
	RawHeaders    string              `json:"raw_headers,omitempty"`
	ParsedHeaders map[string][]string `json:"parsed_headers,omitempty"`
	Body          string              `json:"body,omitempty"`
}
```

In the `proton_get_message` handler, after the `IncludeHeaders` block and before
`return nil, out, nil`, add body decryption:

```go
		if in.IncludeBody {
			krs, kerr := d.Session.Keyrings(ctx)
			if kerr != nil {
				return failure(proterr.Map(kerr)), getMessageOut{}, nil
			}
			body, derr := krs.DecryptBody(raw.AddressID, raw.Body)
			if derr != nil {
				return failure(proterr.Map(derr)), getMessageOut{}, nil
			}
			out.Body = body
		}
```

Update the tool `Description` to drop the "Body is not returned … (v1.5)" sentence and note
`include_body` decrypts the plaintext when a keyring is unlockable.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tools/ -run TestGetMessageInHasIncludeBody -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 5: Live tracer verification (manual — not committed)**

Against the test account on a machine with stored creds:

```bash
PROTONMAIL_MCP_ENABLE_WRITES=0 go run ./cmd/protonmail-mcp   # ensure logged in first via `login`
```

Drive `proton_get_message` with `{"id":"<a real message id>","include_body":true}` through
your MCP client and confirm the returned `body` is the readable plaintext. Record the result
(pass/fail) in this checkbox. Do **not** commit the message or any key.

- [ ] **Step 6: Commit**

```bash
git add internal/tools/messages.go internal/tools/messages_test.go
git commit -m "feat(tools): decrypt message body via proton_get_message include_body"
```

---

## Task 6: Redact keyring material in logs + update status docs

**Files:**
- Modify: `internal/log/` (the redaction field-name set)
- Modify: `README.md`

- [ ] **Step 1: Find the redaction set**

Run: `grep -rn "passphrase\|password\|token\|secret\|totp" internal/log/`
Expected: a slice/map of redacted field-name substrings (per README: "password,
passphrase, token, secret, totp, key").

- [ ] **Step 2: Confirm `key`/`passphrase` already cover keyring material**

If the existing set already contains `passphrase` and `key`, no code change is needed —
`mailbox_password` matches `password` and unlocked-keyring fields match `key`. Add a test
asserting a field named `mailbox_password` is redacted:

```go
func TestRedactsMailboxPassword(t *testing.T) {
	// mirror the existing redaction test in this package
	got := redact(map[string]any{"mailbox_password": "secret"})
	if got["mailbox_password"] == "secret" {
		t.Fatal("mailbox_password must be redacted")
	}
}
```

(Match the actual redaction function name/signature in the package.)

- [ ] **Step 3: Run the redaction test**

Run: `go test ./internal/log/ -v`
Expected: PASS (likely already redacted; the test pins it).

- [ ] **Step 4: Update README status table**

In `README.md`, change the v1 capability row:

```
| Mail search + header inspection (read-only) | yes | metadata + raw headers; body decryption needs unlocked keyring (v1.5) |
```

to reflect that body decryption now works via `include_body`, and update the tool count
note (12 reads / 11 writes) if the read count changed.

- [ ] **Step 5: Commit**

```bash
git add internal/log/ README.md
git commit -m "chore: redact keyring material in logs; document body decryption"
```

---

## Self-review

- **Spec coverage:** keyring lifecycle (Task 4), two-password mode + migration (Tasks 1–2),
  single-decrypt tracer (Tasks 3, 5), never-persist/never-log (Tasks 3 doc comment, Task 6).
  All Phase-0 spec items map to a task.
- **Type consistency:** `Keyrings{User, Addr}`, `DecryptBody(addrID, armoredBody)`,
  `Session.Keyrings(ctx)`, `clearKeyringCache()`, `Creds.MailboxPassword`,
  `LoginInput.MailboxPassword`, `ErrMailboxPasswordRequired`, `getMessageIn.IncludeBody` are
  used identically across tasks.
- **Known adaptation point:** gopenpgp v2.10.0 method arities (`GenerateKey`, `Encrypt`,
  `Decrypt`, `NewKeyRing`) are pinned from that version; if the compiler disagrees, match the
  error — the orchestration logic is unaffected.
- **Coverage floors:** the new `internal/keyring` package must clear the **75% per-package**
  floor under `make coverage-check` (local gate), not just the 90% aggregate. Add table-driven
  error-path tests for `Unlock` (salt error, no-primary-key, unlock failure via a fake
  `KeyFetcher`) if the decrypt tests alone fall short.

## Out of scope (later phases)

Attachment decryption, thread view, send/draft, key generation, and the Calendar/Drive
spikes are separate phase plans per the roadmap.
```

# Feature Parity Roadmap — Design

**Date:** 2026-05-29
**Status:** Approved (roadmap-level; per-phase specs deferred)
**Scope:** Sequencing and cross-cutting architecture for bringing protonmail-mcp toward
feature parity with the Proton product surface reachable through `go-proton-api`.

## Purpose

protonmail-mcp v1 covers account/identity administration (addresses, custom domains,
mail settings, key listing) plus **read-only mail metadata + headers**. This document
is a roadmap, not an implementation spec: it fixes the *order* of work and the
*cross-cutting decisions* that every later phase depends on. Each phase below gets its
own spec → plan → implementation cycle.

This is deliberately a sequencing/architecture artifact. Detailed tool schemas, DTO
shapes, and per-endpoint error mapping belong in each phase's own spec.

## Scope

In scope (all reachable via the pinned `go-proton-api`):

- **Mail** — read (body decryption, attachments, threads), write (draft/send/reply,
  labels, folders, move, mark-read, trash), and key management.
- **Contacts** — list/get/create/update/delete, encrypted vCard cards.
- **Calendar** — feasibility-gated; upstream surface is bridge-oriented and unverified.
- **Drive** — feasibility-gated; upstream surface is partial and unverified.

Explicitly out of scope: **VPN, Pass, Wallet** — no API surface exists in
`go-proton-api`, so parity is not achievable through the current client.

## Key findings that shaped this roadmap

These were verified against the codebase and the pinned upstream
(`github.com/ProtonMail/go-proton-api v0.4.1-0.20260424150947-6bf7f5a61eb8`) on
2026-05-29:

- **The login password is already captured and persisted** (`internal/session/session.go`,
  `NewClientWithLogin(ctx, username, []byte(password))`). The keyring-unlock prerequisite
  is therefore reachable at the data level for single-password accounts — it just is not
  wired up.
- **Upstream already covers most of the target surface.** `go-proton-api` exposes
  `CreateDraft`/`UpdateDraft`/`SendDraft`/`DeleteMessage`/`ImportMessages`,
  `LabelMessages`/`UnlabelMessages`/`MarkMessagesRead`, `CreateLabel`/`CreateFolder`,
  `GetAttachment`/`GetAttachmentInto`, `CreateContacts`/`UpdateContact`/`DeleteContacts`,
  and encryption plumbing (`keyring.go`, `message_encrypt.go`, `message_build.go`). Mail,
  Contacts, and key management are mostly "wire upstream methods to MCP tools," not "build
  a new client."
- **Keyring unlock uses `Client.GetSalts()` → `Salts.SaltForKey(keyPass, keyID)` →
  `Keys.Unlock(passphrase, userKR)`** — user keyring first, then each address keyring.
  There is no single high-level `client.Unlock`; the manager composes the salt and unlock
  steps itself (the same pattern Proton Bridge uses).
- **Two-password mode is detectable** via the upstream `PasswordMode` / `TwoPasswordMode`
  field. Single-password accounts reuse the login password as the mailbox password;
  two-password accounts need a distinct mailbox password that is not captured today.
- **The coverage gate is local, not CI-enforced.** `scripts/coverage-check.sh` enforces
  `AGGREGATE_MIN=90.0` *and* `PER_PKG_MIN=75.0`, run via `make coverage-check`. CI
  (`ci.yml`) runs `make verify-cassettes`, not the coverage check. Each new tool package
  must clear the **75% per-package floor**, not merely lift the aggregate.
- **The MCP SDK supports tool annotations.** The official
  `modelcontextprotocol/go-sdk v1.6.0` `ToolAnnotations` struct exposes `ReadOnlyHint`,
  `DestructiveHint`, `IdempotentHint`, and `OpenWorldHint`. Tools register via
  `mcp.AddTool(server, &mcp.Tool{...})`, one file per area in `internal/tools/`.

## Sequencing strategy

**Foundation-first tracer.** Phase 0 unlocks the keyring and is proven end-to-end by
decrypting a single message body. Every later phase is an independently shippable vertical
slice that mostly wires upstream methods to MCP tools.

Alternatives considered and rejected:

- *Product-by-product* (finish all of Mail before Contacts/Calendar/Drive) bundles the
  risky keyring work with a large mail-write surface in one long phase, delaying the
  de-risking signal.
- *Capability-tier* (all reads everywhere, then all writes, then all dangerous) spreads
  each product across three phases, so no product feels "done" for a long time.

Foundation-first wins because the single real unknown (keyring unlock, two-password mode,
in-memory key residency) is isolated up front and validated by the smallest possible
downstream capability.

### Phase map

| Phase | Deliverable | Upstream support | Gate tier | Risk |
|---|---|---|---|---|
| **0** | Keyring unlock + one-message decrypt tracer; two-password mailbox-password capture; creds-format migration | `keyring.go`, `GetSalts`, `SaltForKey` | read | **High** — the only real unknown |
| **1** | Mail read parity: body decryption, attachment download, thread view | `message.go`, `attachment.go` | read | Low |
| **2** | Mail write: draft/update/send/reply, labels, folders, move, mark-read, trash | `message_send/build/encrypt`, `label.go` | safe + **dangerous** (send/delete) | Med |
| **3** | Key management: generate, set-primary, signed KeyList | `keys.go` | dangerous | Med |
| **4** | Contacts: list/get/create/update/delete (encrypted vCard) | `contact*.go` | safe + dangerous (delete) | Low |
| **5** | Calendar **feasibility spike** → tools if viable | `calendar_event.go` | TBD | **Unverified** |
| **6** | Drive **feasibility spike** → tools if viable | `event_drive.go`, `link_folder.go` | TBD | **Unverified** |

Phases 5–6 each begin with a live feasibility spike against `mail.proton.me`. A spike may
legitimately conclude "defer"; the roadmap records that as a valid outcome, not a failure.

## Cross-cutting architecture

These decisions thread every phase and are fixed here so per-phase specs inherit them.

### Keyring lifecycle

A new `internal/keyring` package provides a decrypted-keyring manager owned by `Session`,
holding the unlocked user keyring plus per-address keyrings.

- **Session-lifetime cache, lazy unlock.** The first crypto-needing tool call triggers
  `Client.GetSalts()` → `Salts.SaltForKey(mailboxPassword, keyID)` →
  `Keys.Unlock(passphrase, userKR)` for the user keyring, then unlocks each address
  keyring against the user keyring. The result is cached for the session.
- **Cleared on logout and relogin.** Any auth-state reset drops the cached keyrings.
- **Never persisted, never logged.** Decrypted keyrings stay in memory only; the existing
  log-redaction set is extended to cover keyring/passphrase material.
- **Surface:** two idempotent helpers — `Session.UserKeyRing(ctx)` and
  `Session.AddrKeyRing(ctx, addrID)`. Both unlock-on-demand and return the cached keyring
  thereafter.

### Two-password mode and credential migration

- **Detection:** read the upstream `PasswordMode` / `TwoPasswordMode` field after auth.
- **Single-password accounts:** the login password is the mailbox password; no new prompt.
- **Two-password accounts:** Phase 0 prompts for and persists a distinct mailbox password.
  `keychain.Creds` (today `{Username, Password, TOTPSecret}`) gains a `MailboxPassword`
  field.
- **Migration:** the keychain **read path must tolerate existing stored creds that lack
  the new field**, so current single-password users are not forced to re-login. An absent
  `MailboxPassword` means "reuse the login password."

### Tiered write gating

Three tiers replace today's binary `PROTONMAIL_MCP_ENABLE_WRITES` flag:

- **read** — always registered (current 12 reads plus new decryption/list tools).
- **safe-write** — `PROTONMAIL_MCP_ENABLE_WRITES=1` (current 11 writes plus drafts,
  labels, folders, mark-read, contact create/update).
- **dangerous** — new `PROTONMAIL_MCP_ENABLE_DANGEROUS=1` for irreversible/outbound
  actions: send, delete message, delete contact, key set-primary. **Dangerous implies
  safe-write.**

Tools additionally carry MCP `ToolAnnotations` (`ReadOnlyHint` / `DestructiveHint` /
`IdempotentHint`) so a client can prompt for confirmation independently of the server-side
env gate. The env gates and the annotations are complementary: gates decide *registration*,
annotations inform *client UX*.

### Tool-file layout

Follows the existing one-file-per-area pattern in `internal/tools/`, each registering via
`mcp.AddTool(server, &mcp.Tool{...})`:

- Phase 1: `bodies.go`, `attachments.go`
- Phase 2: `compose.go`, `labels.go`
- Phase 3: extend `keys.go`
- Phase 4: `contacts.go`
- Phases 5–6: `calendar.go`, `drive.go`

### Encryption plumbing

Send and draft reuse upstream `message_build.go` + `message_encrypt.go` — no hand-rolled
crypto. Outgoing mail is signed with the address primary key from the unlocked keyring;
recipient public keys are fetched via the existing key endpoints; unencrypted-to-external
recipients go through Proton's standard MIME path.

## Verification and risk posture

- **Cassette-backed tests per phase.** Every phase extends the VCR-cassette suite
  (record → scrub → replay), matching the existing `internal/testvcr` harness. New
  cassettes carrying message bodies, attachments, contact cards, or recipient keys must be
  scrubbed of PII before commit.
- **Coverage floors.** Each new package must clear the local `make coverage-check` floors —
  **90% aggregate and 75% per-package** — not just nudge the aggregate. (This gate is
  local; CI does not block on it today.)
- **Feasibility spikes gate Calendar/Drive.** Phases 5–6 start with a live spike; "defer"
  is an acceptable conclusion recorded in that phase's spec.
- **Highest-risk phase is 0.** Keyring unlock, two-password handling, and in-memory key
  residency are concentrated there and validated by the single-decrypt tracer before any
  dependent phase begins.

## Open questions deferred to per-phase specs

- Exact DTO shapes and tool schemas for each new tool.
- Whether thread view (Phase 1) returns decrypted bodies inline or as a separate fetch.
- Reply/forward quoting and attachment-forwarding semantics (Phase 2).
- Signed-KeyList construction details for key generation (Phase 3).
- Whether Calendar/Drive land at all (Phases 5–6 spikes).

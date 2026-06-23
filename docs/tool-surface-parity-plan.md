# protonmail-mcp — Phased plan to ≥90% user-facing API surface coverage

> Grounded against upstream `go-proton-api@v0.4.1-0.20260424150947-6bf7f5a61eb8`
> and the 32 tools in `docs/tool-schema-matrix.md`. Every cited `Client`/`Manager`
> method was confirmed present in the pinned lib; every coverage claim was
> cross-checked against `internal/tools/*.go` registration and gating.

## Current coverage

- Registered tools today: **32** (16 read, 15 write, 1 dangerous).
- Covered user-facing operations: **30 / 73 = 41.1%**.
- Gap to 90% (66 ops): **36 operations**.

## Denominator definition

The denominator is the set of **user-facing data operations** exposed as methods
on `Client`/`Manager`, counted once per distinct capability. It **excludes**
infra/non-feature ops: auth/session (`Auth*`, `AuthInfo`, `AuthDelete`),
salts/key-salt plumbing, 2FA/HV/captcha/verification-code,
ping/status/features/observability/telemetry/report, paging/pool/scheduler
internals, and low-level block/revision streaming primitives. Paged variants
collapse into one op (e.g. `GetContacts`/`GetAllContacts`/`GetAllContactsPaged`
→ "list contacts"; `GetCalendarEvents` + `CountCalendarEvents` → "list events").

Per-domain denominators: mail-messages 16, contacts 6, calendar 5, drive 15,
account-settings 11, addresses-keys 8, domains-labels 12 → **73 total**.

90% target = ceil(0.90 × 73) = **66 ops**. Gap = 66 − 30 = **36 ops**.

## Gap table

| domain | op | rw | upstream method | covered |
|---|---|---|---|---|
| mail-messages | count messages | read | Client.CountMessages / GetGroupedMessageCount | no |
| mail-messages | send draft | dangerous | Client.SendDraft | no |
| mail-messages | import messages | write | Client.ImportMessages | no |
| mail-messages | mark forwarded/unforwarded | write | Client.MarkMessagesForwarded / MarkMessagesUnForwarded | no |
| mail-messages | get label (singular) | read | Client.GetLabel | no |
| mail-messages | create label/folder | write | Client.CreateLabel | no |
| mail-messages | update label/folder | write | Client.UpdateLabel | no |
| mail-messages | delete label/folder | write | Client.DeleteLabel | no |
| contacts | get contact | read | Client.GetContact | no |
| contacts | list contacts | read | Client.GetAllContacts / GetAllContactsPaged | no |
| contacts | list contact emails | read | Client.GetAllContactEmails | no |
| contacts | create contacts | write | Client.CreateContacts | no |
| contacts | update contact | write | Client.UpdateContact | no |
| contacts | delete contacts | dangerous | Client.DeleteContacts | no |
| calendar | get calendar | read | Client.GetCalendar | no |
| calendar | list calendar members | read | Client.GetCalendarMembers | no |
| account-settings | set PGP/draft-mail prefs | write | Client.SetDraftMIMEType / SetAttachPublicKey / SetSignExternalMessages / SetDefaultPGPScheme | no |
| account-settings | get organization data | read | Client.GetOrganizationData | no |
| account-settings | delete user (close account) | dangerous | Client.DeleteUser | no |
| account-settings | create user (signup) | write | Manager.CreateUser | no |
| account-settings | check username available | read | Manager.GetUsernameAvailable | no |
| addresses-keys | reorder addresses | write | Client.OrderAddresses | no |
| addresses-keys | get public keys for email | read | Client.GetPublicKeys | no |
| addresses-keys | create address key | write | Client.CreateAddressKey | no |
| addresses-keys | make address key primary | write | Client.MakeAddressKeyPrimary | no |
| addresses-keys | delete address key | dangerous | Client.DeleteAddressKey | no |
| domains-labels | create label/folder | write | Client.CreateLabel | no |
| domains-labels | update label/folder | write | Client.UpdateLabel | no |
| domains-labels | delete label/folder | write | Client.DeleteLabel | no |

> Label CRUD (`CreateLabel`/`UpdateLabel`/`DeleteLabel`) appears in both the
> mail-messages and domains-labels rows because each domain's denominator counts
> it independently; it is one capability, delivered once by Phase A, and credited
> once toward coverage (Phase A adds 5 ops, not 8).
| drive | list drive volumes | read | Client.ListVolumes | no |
| drive | get drive volume | read | Client.GetVolume | no |
| drive | list drive shares | read | Client.ListShares | no |
| drive | get drive share | read | Client.GetShare | no |
| drive | get drive link | read | Client.GetLink | no |
| drive | create file | write | Client.CreateFile | no |
| drive | create folder | write | Client.CreateFolder | no |
| drive | list file revisions | read | Client.ListRevisions | no |
| drive | get file revision | read | Client.GetRevision | no |
| drive | update file revision | write | Client.UpdateRevision | no |
| drive | list folder children | read | Client.ListChildren | no |
| drive | trash folder children | write | Client.TrashChildren | no |
| drive | delete folder children | dangerous | Client.DeleteChildren | no |
| drive | get attachment | read | Client.GetAttachment | no |
| drive | upload attachment | write | Client.UploadAttachment | no |

## Phased plan

Sequenced by (a) cheapest ops-per-effort first, (b) roadmap order (key-mgmt =
P3, contacts = P4, calendar = P5 done, drive = P6), (c) plaintext/no-decrypt
before decrypt-heavy, (d) write-gating tiers (writes → `ENABLE_WRITES`,
permanent-delete/outbound → `ENABLE_DANGEROUS`).

| Phase | Ops added | Cumulative covered | Cumulative % | Effort | Gate |
|---|---|---|---|---|---|
| A — label CRUD + count + mark-forwarded | 5 | 35 | 47.95% | S | read + ENABLE_WRITES |
| B — org/public-keys/username reads + reorder + PGP prefs | 5 | 40 | 54.79% | S | read + ENABLE_WRITES |
| C — calendar get + members | 2 | 42 | 57.53% | S | read |
| D — contacts (full domain) | 6 | 48 | 65.75% | L | read + ENABLE_WRITES + ENABLE_DANGEROUS |
| E — attachments get + import | 2 | 50 | 68.49% | M | read + ENABLE_WRITES |
| F — address key management | 3 | 53 | 72.60% | M | ENABLE_WRITES + ENABLE_DANGEROUS |
| G — drive reads (volumes/shares/links/children/revisions) | 8 | 61 | 83.56% | L | read |
| H — drive writes + attachment upload | 6 | 67 | 91.78% | L | ENABLE_WRITES + ENABLE_DANGEROUS |

Percentages are cumulative_covered / 73 × 100, monotonic to ≥90%.

### Phase A — Mail/label organization completion (no decrypt) · S
New tools: `proton_create_label`, `proton_update_label`, `proton_delete_label`,
`proton_count_messages`, `proton_mark_forwarded`.
All plaintext metadata, zero keyring. Label CRUD also unblocks the existing
`proton_label_messages` from being limited to pre-existing label IDs.
Tests: VCR cassettes (byte-matched request bodies); gate tests mirroring
`organize_gate_test.go`.

### Phase B — Account/address plaintext reads + reorder · S
New tools: `proton_get_organization`, `proton_get_public_keys`,
`proton_check_username_available`, `proton_order_addresses`; extend
`proton_update_mail_settings` with `pgp_scheme` / `draft_mime_type` /
`attach_public_key` / `sign_external` fields (folds the 4 PGP setters into one
existing tool — no new surface).
Tests: VCR cassettes; extend `settings_cassette_test.go`. Manager-method
cassettes use the offline fake-server pattern.

### Phase C — Calendar reads completion (decrypt-light) · S
New tools: `proton_get_calendar`, `proton_list_calendar_members`. Both
metadata-only (no ICS decrypt); the raw `GetCalendarMembers` path already exists
in `internal/calendar/keyring.go`.
Tests: reuse `calendar_cassette_test.go`.

### Phase D — Contacts (roadmap Phase 4) · L
New tools: `proton_list_contacts`, `proton_list_contact_emails`,
`proton_get_contact` (read); `proton_create_contacts`, `proton_update_contact`
(ENABLE_WRITES); `proton_delete_contacts` (ENABLE_WRITES + ENABLE_DANGEROUS).
Reads land first. `get_contact` decrypts/verifies signed vCard cards; writes
must build and **sign** vCard cards with the keyring (the hard part). New
`internal/tools/contacts.go`.
Tests: metadata reads → VCR cassettes; card decrypt/sign → synthetic-key tests
(cannot use scrubbed cassettes), live-only-skip in CI per the #196 pattern.

### Phase E — Mail attachments + import (decrypt-heavy) · M
New tools: `proton_get_attachment` (read, decrypts via message/address keyring),
`proton_import_messages` (ENABLE_WRITES, low-priority).
Tests: `get_attachment` → synthetic-key decrypt, live-only-skip;
`import_messages` → byte-matched cassette.

### Phase F — Address key management (roadmap Phase 3) · M
New tools: `proton_create_address_key`, `proton_make_address_key_primary`
(ENABLE_WRITES); `proton_delete_address_key` (ENABLE_WRITES + ENABLE_DANGEROUS).
All three require a **signed KeyList** built with the unlocked account
passphrase (deferred per `keys.go:66-71`).
Tests: synthetic-keypair sign tests against the offline fake server; live-only
skip the passphrase-unlock path.

### Phase G — Drive reads (roadmap Phase 6) · L
New tools: `proton_list_drive_volumes`, `proton_get_drive_volume`,
`proton_list_drive_shares`, `proton_get_drive_share`, `proton_get_drive_link`,
`proton_list_folder_children`, `proton_list_file_revisions`,
`proton_get_file_revision`. Largest single jump (8 ops, crosses 80%). Node
names are E2E-encrypted: `get_link`/`list_children`/revisions need the share
keyring to decrypt names; reads only. New `internal/tools/drive.go`.
Tests: metadata → cassettes; name decryption → synthetic share-keyring tests,
live-only-skip. Establish a drive cassette harness mirroring calendar's.

### Phase H — Drive writes + attachment upload (roadmap Phase 6) · L
New tools: `proton_create_drive_folder`, `proton_create_drive_file`,
`proton_update_file_revision`, `proton_trash_folder_children`,
`proton_upload_attachment` (ENABLE_WRITES); `proton_delete_folder_children`
(ENABLE_WRITES + ENABLE_DANGEROUS). Most complex crypto in the plan
(share/node key derivation, content-key encryption, block-upload/manifest
finalize). Crosses 90%.
Tests: encrypted-name/content-key derivation → synthetic share-keyring
round-trip; byte-matched cassettes where decrypt isn't involved.

## Minimum path to 90%

After Phase G: 61 ops = 83.56%. 90% needs 66 ops → **5 more ops from Phase H**.

Cheapest concrete minimum: all of A–G, then the 5 lowest-complexity Phase-H ops
— `proton_create_drive_folder`, `proton_create_drive_file`,
`proton_trash_folder_children`, `proton_update_file_revision`,
`proton_upload_attachment` — deferring `proton_delete_folder_children` (the
dangerous permanent delete) as stretch. That hits **66/73 = 90.41%** without
building outbound send.

## Non-goals & risks

- **SendDraft (outbound mail)** — deliberately NOT built per roadmap. The math
  does not force it; 90% is reachable via drive. Keep deferred.
- **Account-lifecycle ops out of scope:** `DeleteUser` (close account),
  `Manager.CreateUser` (unauth signup, captcha). `GetUsernameAvailable` is
  included in B only because it is a trivial plaintext read.
- **VPN / Pass / Wallet** — no upstream API, permanently out of scope.
- **Drive write complexity (Phase H)** is the dominant risk: share/node key
  derivation, content-key encryption, and block-upload/manifest finalize are the
  hardest crypto in the plan. If Phase H slips, 90% is missed.
- **Decrypt-test constraint:** any op that decrypts (`get_contact`,
  `get_attachment`, drive name decryption, signed-KeyList key-mgmt) cannot use
  scrubbed VCR cassettes — those tests need synthetic key material or a
  full-scope live account (`live-only-skip` in CI).
- **Two gating-tier mismatches surfaced during this analysis are fixed in the
  same change:** `proton_delete_address` and `proton_remove_custom_domain` were
  gated under `WritesEnabled()` only despite being irreversible (permanent
  address delete; domain removal orphans all aliases). Both now require the
  `ENABLE_DANGEROUS` tier, matching `proton_delete_messages`. This is a breaking
  change for callers that invoked them with `ENABLE_WRITES` alone.
```

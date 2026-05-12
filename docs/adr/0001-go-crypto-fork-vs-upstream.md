# 0001. `ProtonMail/go-crypto` `v1.4.1-proton` -> `v1.4.1` swap is safe

- **Status:** Proposed
- **Date:** 2026-05-12
- **Deciders:** maintainer
- **Issue:** #16

## Context

PR #11 swapped `github.com/ProtonMail/go-crypto v1.4.1-proton` -> `v1.4.1` in
the transitive dependency closure. `go.sum` shows identical `go.mod` hashes
between the two tags but a different `h1` source-tree hash
(`I4nanwGU...` -> `9RfcZHqE...`), proving the trees are not byte-equivalent.

The risk #16 flagged: if the `-proton` tag carried a security-relevant patch
that hadn't been upstreamed (timing-attack mitigation, weak-key rejection,
padding-oracle fix in OpenPGP primitives), the swap silently dropped it.

`govulncheck` catches known CVEs in the dependency graph; it does not catch
behavioural or hardening regressions in private patches.

## Mechanical findings

Both tags live in the same upstream repository (`github.com/ProtonMail/go-crypto`).
There is no separate "upstream" project — `v1.4.1` and `v1.4.1-proton` are
two siblings cut from the same history on 2026-03-18.

### Tag relationship

```
git -C <go-crypto> log --oneline v1.4.1..v1.4.1-proton  → 12 commits
git -C <go-crypto> log --oneline v1.4.1-proton..v1.4.1  → 0 commits
```

`v1.4.1` is a **strict ancestor** of `v1.4.1-proton`. The swap from
`v1.4.1-proton` -> `v1.4.1` is a **downgrade**, not a sync: nothing went
upstream between the two tags, but 12 fork-only commits were dropped.

This means there is **no risk of a missed upstream security fix** — the
only risk is dropping a security-relevant fork patch.

### Fork-only commits (dropped by the swap)

| Commit | Subject | Category |
|---|---|---|
| `ab89abf` | Add support for draft-ietf-openpgp-persistent-symmetric-keys-00 | Feature (draft) |
| `57cde19` | feat(pqc): Update mlkem/mldsa to draft-ietf-openpgp-pqc-09 | Feature (PQC draft) |
| `b1abb2a` | feat(pqc): draft-ietf-openpgp-pqc-06 with updated kem combiner | Feature (PQC draft) |
| `d1f3a60` | Revert "Update to draft-ietf-openpgp-persistent-symmetric-keys-00" | Revert (no-op) |
| `ad1d0f0` | Revert "[v2] Use AEAD if all public keys support it" | Revert (no-op) |
| `b34b02e` | Add support for automatic forwarding (#54) | Feature (forwarding) |
| `a0e7c6d` | Update default config | Config |
| `0115f4c` | Update GitHub workflow branches | CI |
| `0036466` | Update to draft-ietf-openpgp-persistent-symmetric-keys-00 | Feature (draft) |
| `14eebf3` | Full PQC support (+33 squashed commits) | Feature (PQC) |
| `5bc5f01` | Replace ioutil.ReadAll with io.ReadAll | Housekeeping |
| `56f0d0f` | openpgp: Add support for symmetric subkeys (#74) | Feature (symmetric keys) |

Keyword scan of all 12 commit **subjects** for
`security|cve|fix|harden|timing|side-channel|leak|oracle`: **0 hits**. The
bodies surface only PQC-internal `fix:` lines from the squashed-commits
breakdown — those are covered explicitly below.

Within the PQC squash (commit `14eebf3`) there are three internal
correctness commits:

- `[9677cf4] feat: Avoid panic on key size in kmac` — panic-prevention in
  the PQC kmac wrapper.
- `[1bd89db] fix: Kem key combiner should use the kmac correct key` —
  correctness fix in the PQC KEM combiner.
- The squash's own "Fix misc bugs and improve tests" rollup line.

All three are PQC-internal robustness/correctness fixes inside code paths
that `v1.4.1` does not contain at all — there is nothing to regress against
when downgrading.

### File changes

```
76 files changed, 8580 insertions(+), 316 deletions(-)
```

The +8264 net is dominated by:

- `openpgp/internal/ecc/curve25519/field/` — new in-tree, ASM-accelerated
  curve25519 field implementation (`fe.go`, `fe_amd64.s`, `fe_arm64.s`,
  generic and noasm fallbacks). Replaces v1.4.1's delegation to the Go
  standard library's `golang.org/x/crypto/curve25519`. Both implementations
  are constant-time by design.
- `openpgp/mlkem_ecdh/`, `openpgp/mldsa_eddsa/` — new PQC packages.
- `openpgp/symmetric/` — new symmetric-subkey packages (AEAD + HMAC,
  experimental variants).
- `openpgp/forwarding.go`, `openpgp/packet/forwarding.go`,
  `openpgp/v2/forwarding.go` — new automatic-forwarding code.
- Additions to `openpgp/packet/{encrypted_key,private_key,public_key,
  signature}.go`, `openpgp/keys.go`, `openpgp/key_generation.go`, the v2
  twins — all dispatch into the new PQC / forwarding / symmetric paths.
- Test data and test files for the above.

### Surface used by protonmail-mcp

This module reaches `go-crypto` through one path only:

```
internal/tools/keys.go
    crypto.NewKey(k.PrivateKey)        // gopenpgp/v2/crypto wraps go-crypto
    pk.GetFingerprint()
    pk.GetArmoredPublicKey()
```

The serialized binary `PrivateKey` is produced by `go-proton-api`'s
`Key.UnmarshalJSON` (which itself calls `crypto.NewKeyFromArmored` then
`Serialize` to canonical binary). The downstream calls touch:

- Key parsing for v3/v4/v6 OpenPGP keys (covered by both tags).
- Fingerprint computation per RFC 4880 / RFC 9580 (covered by both tags).
- ASCII armoring per RFC 4880 (covered by both tags).

None of the 12 dropped commits affect this surface — they all add new
algorithm/key-type families that protonmail-mcp does not invoke, generate,
or accept.

The #50 regression test (`TestListAddressKeys_HappyPath_GopenpgpRegression`)
exercises the full path end-to-end against the current `v1.4.1` pin and
passes; behavioural compatibility on the keys-used path is locked in.

## Decision

> **TBD** — maintainer review required.

This ADR proposes the swap is **safe** for protonmail-mcp's current surface.
The maintainer should fill in either:

- **Accept upstream `v1.4.1`** — confirm the assessment above and close
  #16 with this ADR.
- **Revert to `v1.4.1-proton`** — name the specific concern (e.g. desire
  to follow proton-bridge's exact dependency closure, anticipation of
  using PQC or forwarding in v2) and open a follow-up to gate-check
  future swaps before they land.

Sign here when accepted:

```
Decided by: __________________
Date:       __________________
Rationale:  __________________
```

## Consequences

If the decision is **accept**:

- Future bumps of `ProtonMail/go-crypto` should still be reviewed against
  this ADR's framing: identify the tag relationship (sibling vs ancestor),
  scan the commit-level delta for security-relevant patches, confirm the
  protonmail-mcp surface is unchanged. The #50 regression test catches
  silent behavioural drift on the keys path.
- If protonmail-mcp ever exposes PQC, forwarding, persistent symmetric, or
  symmetric-subkey functionality, this ADR no longer applies and a fresh
  delta audit against the then-current tags is required.

If the decision is **revert**:

- Pin back to `v1.4.1-proton` in the dependency closure (note: the pin
  has to be done via the modules that depend on `go-crypto`, since this
  repo only references it transitively — adding a `replace` directive in
  `go.mod` is the standard fix; `go.mod` already uses one such directive
  to route `go-resty/resty/v2` to ProtonMail's fork, so the precedent is
  in place).
- Document the revert reason in this ADR's Decision section so future
  bumps know not to undo it without re-running the audit.

## Reproduction

```sh
git clone https://github.com/ProtonMail/go-crypto.git /tmp/gc-audit
cd /tmp/gc-audit
git log --oneline v1.4.1..v1.4.1-proton          # 12 commits, fork-only
git log --oneline v1.4.1-proton..v1.4.1          # empty (v1.4.1 is ancestor)
git diff --stat v1.4.1..v1.4.1-proton            # file inventory
git log --pretty='%h %s%n%b' v1.4.1..v1.4.1-proton \
  | grep -iE "security|cve|fix|harden|timing|side-?channel|leak|oracle"
```

# 0003. Self-heal under-scoped sessions by reusing the unattended relogin

- **Status:** Proposed
- **Date:** 2026-06-09
- **Deciders:** maintainer
- **Issue:** #197 (spike; depends on the #195 scope-denial sentinel, now landed)

## Context

`Session` already performs a one-shot unattended relogin, but it fires only
when a refresh token is rejected. `internal/session/session.go:400` gates it:

```go
if proterr.RefreshRejected(err) && !s.reloginExhausted {
    healed, captcha := s.reloginLocked(ctx)
    ...
    s.reloginExhausted = true
}
```

`reloginLocked` (`session.go:548`) re-runs SRP + 2FA from stored credentials
(`s.kc.LoadCreds`), which yields a **full-scope** token via the post-2FA
upgrade. It already returns `(nil, nil)` when no usable creds / no stored TOTP
secret are present, and surfaces `proton/captcha` when Proton answers with a
human-verification challenge — so the eligibility logic this spike needs
already exists and is tested for the refresh path.

Separately, #195 landed a precise sentinel for the *other* dead-end: a session
whose token cannot unlock the mailbox keyring. `proterr.ScopeDenied(err)`
(`internal/proterr/proterr.go:181`) reports exactly the salts scope denial —
HTTP 403 **and** `Code 9100` — distinct from a generic plan/permission 403.
That denial surfaces from `Keyrings()` at the `keyring.Unlock` call
(`session.go:207`, wrapped `"unlock keyrings: %w"`), inside the `unlockMu`
single-flight and **not** holding `s.mu`.

So the machinery to acquire a fresh full-scope session exists, the failure is
precisely detectable, and the two trigger conditions are already isolated in
different code paths. The spike question (#197) is whether to also invoke the
relogin when the *current* failure is scope-insufficiency at unlock.

### The five open questions, resolved against the code

1. **Trigger.** `Keyrings()` is the single cache-miss unlock site
   (`session.go:184-213`); `CalendarKeyring` funnels through it. Hooking the
   trigger there — "if `keyring.Unlock` fails and `proterr.ScopeDenied(err)`,
   attempt one relogin, then retry the unlock once" — covers both message and
   calendar decrypt without touching call sites. Use a **separate** latch from
   `reloginExhausted`: the issue notes the two conditions can co-occur, and a
   shared latch would let a refresh-rejection self-heal silently consume the
   scope-insufficiency budget (or vice-versa).

2. **Eligibility.** Reuse `reloginLocked`'s existing contract verbatim:
   `(nil, nil)` on no-creds / no-TOTP-secret, `(nil, captcha)` on challenge. On
   either, fall through to #195's actionable `ErrScopeInsufficient` error so the
   operator is told to re-run login completing 2FA — no behaviour regression.

3. **Loop safety.** `ScopeDenied` matches only `403 + Code 9100`, so a generic
   plan/permission 403 cannot trigger it. A genuine *unfixable* 9100 (account
   truly lacks the scope even after a fresh full-scope login) is bounded by the
   dedicated one-attempt latch: one relogin, one unlock retry, then fall through
   to the actionable error. The latch resets only on explicit `Login`/`Logout`
   (mirroring `reloginExhausted`, reset at `session.go:521` and `:667`), so a
   recurring 403 cannot defeat it within a session.

4. **Idempotence / concurrency.** The trigger lives **inside** the `unlockMu`
   single-flight, so at most one relogin-and-retry runs at a time and concurrent
   first-use callers reuse the winner's keyrings. `reloginLocked` requires
   `s.mu`; the unlock path holds only `unlockMu`, so the new path must take
   `s.mu` for the relogin and release it before the network retry-unlock
   (`keyring.Unlock` does I/O and must not run under `s.mu`). Ordering:
   acquire `s.mu` → relogin (mutates `s.client`/`s.current`) → release `s.mu`
   → re-`fetcher(ctx)` → retry `Unlock`. The post-relogin keyrings cache write
   already round-trips through `s.mu` (`session.go:211`).

5. **Observability.** `reloginLocked` already logs mapped codes only, never the
   wrapped cause (which could echo credentials) — `session.go:566-579`. Add
   structured `slog` lines for the scope-insufficiency attempt, success, and
   latch exhaustion, tagged distinctly from the refresh-rejection path, with no
   token/secret fields.

## Decision

**Recommend: yes — implement, reusing `reloginLocked` behind a dedicated
scope-insufficiency latch.** The remedy (a fresh full-scope token from stored
creds) is exactly what the refresh-rejection path already produces; the failure
is precisely detectable via the #195 sentinel; the eligibility, latch, and
secret-safe logging patterns are already built and tested. The marginal new
surface is one trigger site in `Keyrings()` plus one latch field — small,
localized, and behind the same anti-abuse bound as the existing self-heal.

This ADR is the spike deliverable (AC#1). It records the decision and risk
tradeoff; it does **not** itself change behaviour. Marking it **Accepted**
authorizes the implementation below; leaving it **Proposed** or moving it to
**Rejected** closes #197 without code change.

## Consequences

- A new dedicated latch field (e.g. `unlockReloginExhausted`) on `Session`,
  reset alongside `reloginExhausted` in `Login`/`Logout`.
- One trigger site added to `Keyrings()`; `CalendarKeyring` inherits it.
- The risk is Proton's login anti-abuse lockout (~10 logins/min) if the latch
  is defeated — mitigated by the same one-attempt bound the refresh path uses,
  plus the strict `403 + Code 9100` match that prevents unrelated 403s from
  triggering it.

### Implementation outline (if Accepted)

1. Add `unlockReloginExhausted bool` to `Session`; reset it where
   `reloginExhausted` is reset (`session.go:521`, `:667`).
2. In `Keyrings()`, when `keyring.Unlock` returns and `proterr.ScopeDenied(err)`
   and `!s.unlockReloginExhausted`: take `s.mu`, `reloginLocked`, release,
   set the latch, then re-`fetcher` and retry `Unlock` once. On captcha /
   ineligible / retry-still-denied, return `proterr.ErrScopeInsufficient`.
3. Tests (cassette/seam-driven, no live calls):
   - under-scoped session + eligible stored creds → self-heals on first decrypt,
     decrypt then succeeds;
   - ineligible creds (no TOTP secret) → falls through to the #195 actionable
     error without attempting login;
   - recurring unfixable 9100 → exactly one relogin, latch holds, no loop;
   - no secret/token fields on any logged line.

## Revisit when

- Proton changes the salts scope-denial code/shape (would move `scopeDeniedCode`
  off 9100 and is pinned by `salts_underscoped_denied.yaml`), or
- the unattended relogin is removed or its anti-abuse bound is reworked.

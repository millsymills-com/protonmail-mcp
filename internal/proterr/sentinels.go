package proterr

import "errors"

// ErrNoSession is the sentinel that session.Client wraps when the keychain
// holds no session. Lives here (not in internal/session) to avoid a circular
// import: proterr.Map needs to match on it, and session imports proterr.
var ErrNoSession = errors.New("no session in keychain")

// ErrKeyringLocked signals the mailbox keyring could not be unlocked (wrong
// mailbox password, or no address key unlocked at all). It is non-retryable —
// distinct from transient proton/upstream transport failures.
var ErrKeyringLocked = errors.New("keyring locked or unusable")

// ErrKeyringUnlockScope signals the session's token lacks the scope needed to
// unlock the mailbox keyring: the salts fetch in the unlock path returned a
// scope denial (HTTP 403 / Proton Code 9100). Distinct from ErrKeyringLocked
// (wrong mailbox password) and from a generic resource-level 403 — the cause is
// an under-scoped session, the remedy is re-login completing two-factor. It
// affects every decryption path (message bodies and calendar events) because
// they all route through the same keyring unlock.
var ErrKeyringUnlockScope = errors.New("session lacks keyring-unlock scope")

// ErrTOTPRejected signals Proton rejected the submitted two-factor code at the
// /auth/v4/2fa step (HTTP 422 / Proton Code 8002). Proton reuses PasswordWrong
// (8002) for both a wrong password and a rejected TOTP, so the password-auth
// step already succeeded by the time this is raised — the misleading "Incorrect
// login credentials" text refers to the code, not the password. The session
// login path wraps this sentinel so Map can give an actionable hint (check the
// otpauth secret matches this account; check the host clock is synced) instead
// of echoing Proton's verbatim message.
var ErrTOTPRejected = errors.New("two-factor code rejected")

// ErrBodyUndecryptable signals a specific message body cannot be decrypted even
// though the keyring is unlocked: it's empty/plaintext/unparseable, encrypted to
// a key we don't hold, or its address has no usable keyring. Distinct from
// ErrKeyringLocked so the operator is not told to re-check their mailbox password
// for a body that was never decryptable with the available keys.
var ErrBodyUndecryptable = errors.New("message body is not decryptable")

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

// ErrBodyUndecryptable signals a specific message body cannot be decrypted even
// though the keyring is unlocked: it's empty/plaintext/unparseable, encrypted to
// a key we don't hold, or its address has no usable keyring. Distinct from
// ErrKeyringLocked so the operator is not told to re-check their mailbox password
// for a body that was never decryptable with the available keys.
var ErrBodyUndecryptable = errors.New("message body is not decryptable")

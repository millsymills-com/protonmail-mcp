package proterr

import "errors"

// ErrNoSession is the sentinel that session.Client wraps when the keychain
// holds no session. Lives here (not in internal/session) to avoid a circular
// import: proterr.Map needs to match on it, and session imports proterr.
var ErrNoSession = errors.New("no session in keychain")

// ErrKeyringLocked signals the mailbox keyring could not be unlocked or used
// (wrong mailbox password, no usable address key, or a decrypt failure). It is
// non-retryable — distinct from transient proton/upstream transport failures.
var ErrKeyringLocked = errors.New("keyring locked or unusable")

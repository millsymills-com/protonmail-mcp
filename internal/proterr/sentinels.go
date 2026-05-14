package proterr

import "errors"

// ErrNoSession is the sentinel that session.Client wraps when the keychain
// holds no session. Lives here (not in internal/session) to avoid a circular
// import: proterr.Map needs to match on it, and session imports proterr.
var ErrNoSession = errors.New("no session in keychain")

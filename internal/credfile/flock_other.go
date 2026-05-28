//go:build !unix

package credfile

import "io/fs"

// flockSupported is false here: non-Unix platforms have no advisory file lock
// or POSIX ownership check, so New refuses the file backend rather than store
// credentials without the concurrency and ownership guards.
const flockSupported = false

// These degrade to no-ops so the package still builds on non-Unix; New gates
// actual use behind flockSupported.
func flockExclusive(uintptr) error { return nil }

func flockUnlock(uintptr) error { return nil }

func ownedByCurrentUser(fs.FileInfo) bool { return true }

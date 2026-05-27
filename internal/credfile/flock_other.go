//go:build !unix

package credfile

import "io/fs"

// Non-Unix platforms have no advisory file lock or POSIX ownership here; the
// file backend targets Unix (Linux deploy, macOS uses the keychain), so these
// degrade to no-ops rather than block builds.
func flockExclusive(uintptr) error { return nil }

func flockUnlock(uintptr) error { return nil }

func ownedByCurrentUser(fs.FileInfo) bool { return true }

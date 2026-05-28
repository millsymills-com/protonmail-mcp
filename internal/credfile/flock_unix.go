//go:build unix

package credfile

import (
	"io/fs"
	"os"
	"syscall"
)

// flockSupported reports whether this platform provides the advisory file
// locking and ownership checks the file backend relies on.
const flockSupported = true

func flockExclusive(fd uintptr) error { return syscall.Flock(int(fd), syscall.LOCK_EX) }

func flockUnlock(fd uintptr) error { return syscall.Flock(int(fd), syscall.LOCK_UN) }

func ownedByCurrentUser(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return true // unknown ownership model — don't block writes
	}
	return int(st.Uid) == os.Getuid()
}

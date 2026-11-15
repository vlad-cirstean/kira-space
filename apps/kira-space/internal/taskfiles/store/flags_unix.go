//go:build unix

package store

import (
	"io/fs"
	"os"
	"syscall"
)

// openFlags never follows a symlink and never blocks on a FIFO swapped in after the lstat.
const openFlags = os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK

func linkCount(fi fs.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink) //nolint:unconvert // the field is uint16 on darwin
	}
	return 1
}

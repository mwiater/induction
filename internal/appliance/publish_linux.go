//go:build linux

package appliance

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func renameNoReplace(dirfd int, old, new string) error {
	return unix.Renameat2(dirfd, old, dirfd, new, unix.RENAME_NOREPLACE)
}
func syncDir(dirfd int) error { return unix.Fsync(dirfd) }
func linkCount(info os.FileInfo) int {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(stat.Nlink)
	}
	return 0
}

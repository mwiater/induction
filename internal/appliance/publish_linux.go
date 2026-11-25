//go:build linux

// Package appliance provides the implementation for this subsystem.
package appliance

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// renameNoReplace performs the internal operation described by its signature.
func renameNoReplace(dirfd int, old, newName string) error {
	return unix.Renameat2(dirfd, old, dirfd, newName, unix.RENAME_NOREPLACE)
}

// syncDir performs the internal operation described by its signature.
func syncDir(dirfd int) error { return unix.Fsync(dirfd) }

// linkCount performs the internal operation described by its signature.
func linkCount(info os.FileInfo) int {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(stat.Nlink)
	}
	return 0
}

// deviceInode performs the internal operation described by its signature.
func deviceInode(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(stat.Dev)
	}
	return 0
}

// inodeNumber performs the internal operation described by its signature.
func inodeNumber(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(stat.Ino)
	}
	return 0
}

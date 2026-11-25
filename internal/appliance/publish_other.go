//go:build !linux

// Package appliance provides the implementation for this subsystem.
package appliance

import (
	"fmt"
	"os"
)

// renameNoReplace performs the internal operation described by its signature.
func renameNoReplace(int, string, string) error {
	return fmt.Errorf("atomic no-clobber publication requires Linux")
}

// syncDir performs the internal operation described by its signature.
func syncDir(int) error { return nil }

// linkCount performs the internal operation described by its signature.
func linkCount(os.FileInfo) int { return 1 }

// deviceInode performs the internal operation described by its signature.
func deviceInode(os.FileInfo) uint64 { return 0 }

// inodeNumber performs the internal operation described by its signature.
func inodeNumber(os.FileInfo) uint64 { return 0 }

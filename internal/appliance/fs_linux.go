//go:build linux

// Package appliance provides the implementation for this subsystem.
package appliance

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// Workspace file operations deliberately accept only one directory component
// and resolve it relative to the pinned startup directory. Landlock is the
// process-wide second layer; openat2 prevents path races and symlink/magic-link
// escapes at each controlled file operation.
func validateWorkspaceName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
		return fmt.Errorf("invalid workspace filename")
	}
	return nil
}

// openWorkspaceFile performs the internal operation described by its signature.
func openWorkspaceFile(dirfd int, name string, flags int, mode uint32) (*os.File, error) {
	if err := validateWorkspaceName(name); err != nil {
		return nil, err
	}
	how := &unix.OpenHow{
		Flags:   uint64(flags) | uint64(unix.O_CLOEXEC),
		Mode:    uint64(mode),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	}
	fd, err := unix.Openat2(dirfd, name, how)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

// readWorkspaceFile performs the internal operation described by its signature.
func readWorkspaceFile(dirfd int, name string, maxBytes int64) ([]byte, os.FileInfo, error) {
	f, err := openWorkspaceFile(dirfd, name, unix.O_RDONLY, 0)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, info, fmt.Errorf("not a regular file")
	}
	if maxBytes > 0 && info.Size() > maxBytes {
		return nil, info, fmt.Errorf("file exceeds maximum size")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, info, err
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return nil, info, fmt.Errorf("file exceeds maximum size")
	}
	return data, info, nil
}

// statWorkspaceFile performs the internal operation described by its signature.
func statWorkspaceFile(dirfd int, name string) (os.FileInfo, error) {
	f, err := openWorkspaceFile(dirfd, name, unix.O_PATH, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return f.Stat()
}

// removeWorkspaceFile performs the internal operation described by its signature.
func removeWorkspaceFile(dirfd int, name string) error {
	if err := validateWorkspaceName(name); err != nil {
		return err
	}
	return unix.Unlinkat(dirfd, name, 0)
}

// rewindWorkspaceDir performs the internal operation described by its signature.
func rewindWorkspaceDir(dir *os.File) error {
	_, err := dir.Seek(0, 0)
	return err
}

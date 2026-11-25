//go:build !linux

// Package appliance provides the implementation for this subsystem.
package appliance

import (
	"fmt"
	"os"
)

// validateWorkspaceName performs the internal operation described by its signature.
func validateWorkspaceName(string) error { return fmt.Errorf("workspace confinement requires Linux") }

// openWorkspaceFile performs the internal operation described by its signature.
func openWorkspaceFile(int, string, int, uint32) (*os.File, error) {
	return nil, fmt.Errorf("workspace confinement requires Linux")
}

// readWorkspaceFile performs the internal operation described by its signature.
func readWorkspaceFile(int, string, int64) ([]byte, os.FileInfo, error) {
	return nil, nil, fmt.Errorf("workspace confinement requires Linux")
}

// statWorkspaceFile performs the internal operation described by its signature.
func statWorkspaceFile(int, string) (os.FileInfo, error) {
	return nil, fmt.Errorf("workspace confinement requires Linux")
}

// removeWorkspaceFile performs the internal operation described by its signature.
func removeWorkspaceFile(int, string) error {
	return fmt.Errorf("workspace confinement requires Linux")
}

// rewindWorkspaceDir performs the internal operation described by its signature.
func rewindWorkspaceDir(*os.File) error { return fmt.Errorf("workspace confinement requires Linux") }

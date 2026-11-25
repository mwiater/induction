//go:build !linux

// Package appliance provides the implementation for this subsystem.
package appliance

import "fmt"

// installConfinement performs the internal operation described by its signature.
func installConfinement(int) error {
	return fmt.Errorf("Landlock appliance confinement requires Linux")
}

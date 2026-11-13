//go:build !linux

package appliance

import "fmt"

func installConfinement(int) error {
	return fmt.Errorf("Landlock appliance confinement requires Linux")
}

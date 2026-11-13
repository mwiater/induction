//go:build !linux

package appliance

import (
	"fmt"
	"os"
)

func renameNoReplace(int, string, string) error {
	return fmt.Errorf("atomic no-clobber publication requires Linux")
}
func syncDir(int) error         { return nil }
func linkCount(os.FileInfo) int { return 1 }

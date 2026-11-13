//go:build linux

package appliance

import (
	"fmt"
	"golang.org/x/sys/unix"
	"unsafe"
)

type landlockRulesetAttr struct{ HandledAccessFs uint64 }
type landlockPathBeneathAttr struct {
	AllowedAccess uint64
	ParentFd      int32
}

func installConfinement(workspaceFD int) error {
	if workspaceFD < 0 {
		return fmt.Errorf("workspace fd unavailable")
	}
	version, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION), 0, 0, 0)
	// ABI 3 provides the REFER/TRUNCATE controls required for safe
	// no-clobber staging and explicit write handling.
	if errno != 0 || version < 3 {
		return fmt.Errorf("Landlock unavailable: %v", errno)
	}
	rights := uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE | unix.LANDLOCK_ACCESS_FS_MAKE_REG | unix.LANDLOCK_ACCESS_FS_REFER | unix.LANDLOCK_ACCESS_FS_TRUNCATE)
	attr := landlockRulesetAttr{HandledAccessFs: rights}
	ruleset, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), uintptr(unsafe.Sizeof(attr)), 0, 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("create Landlock ruleset: %w", errno)
	}
	defer unix.Close(int(ruleset))
	rule := landlockPathBeneathAttr{AllowedAccess: rights, ParentFd: int32(workspaceFD)}
	_, _, errno = unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, ruleset, uintptr(unix.LANDLOCK_RULE_PATH_BENEATH), uintptr(unsafe.Pointer(&rule)), 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("add Landlock rule: %w", errno)
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}
	_, _, errno = unix.Syscall6(unix.SYS_LANDLOCK_RESTRICT_SELF, ruleset, 0, 0, 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("restrict self: %w", errno)
	}
	return nil
}

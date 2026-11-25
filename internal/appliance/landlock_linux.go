//go:build linux

// Package appliance provides the implementation for this subsystem.
package appliance

import (
	"fmt"
	"golang.org/x/sys/unix"
	"unsafe"
)

// landlockRulesetAttr is an internal data type used by this package.
type landlockRulesetAttr struct { // HandledAccessFs contains the Landlock access-bit mask.
	HandledAccessFs uint64
}

// landlockPathBeneathAttr is an internal data type used by this package.
type landlockPathBeneathAttr struct {
	// AllowedAccess stores the value represented by this member.
	AllowedAccess uint64
	// ParentFd stores the value represented by this member.
	ParentFd int32
}

// installConfinement performs the internal operation described by its signature.
// Additional context: ABI 3 provides the REFER/TRUNCATE controls required for safe no-clobber staging and explicit write handling.
// Additional context: The workspace is readable, and only regular-file creation, append/write, file removal, rename, and truncation are needed by the controlled publisher. Directory creation/removal, executable access, device/socket creation, and symlink creation remain denied even inside the workspace.
func installConfinement(workspaceFD int) error {
	if workspaceFD < 0 {
		return fmt.Errorf("workspace fd unavailable")
	}
	version, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION), 0, 0, 0)

	if errno != 0 || version < 3 {
		return fmt.Errorf("Landlock unavailable: %v", errno)
	}
	handledRights := uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE |
		unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
		unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM |
		unix.LANDLOCK_ACCESS_FS_REFER |
		unix.LANDLOCK_ACCESS_FS_TRUNCATE)
	attr := landlockRulesetAttr{HandledAccessFs: handledRights}
	ruleset, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), uintptr(unsafe.Sizeof(attr)), 0, 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("create Landlock ruleset: %w", errno)
	}
	defer unix.Close(int(ruleset))

	allowedRights := uint64(unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_REFER |
		unix.LANDLOCK_ACCESS_FS_TRUNCATE)
	rule := landlockPathBeneathAttr{AllowedAccess: allowedRights, ParentFd: int32(workspaceFD)}
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

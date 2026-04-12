// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

package landlock

import (
	"fmt"
	"unsafe"

	"github.com/altessa-s/go-atlas/core/runtime/nonewprivs"

	"golang.org/x/sys/unix"
)

// Landlock ABI version constants. Each version corresponds to a kernel
// release that added new access flags. This package negotiates the
// highest mutually supported version at runtime and composes the access
// mask accordingly.
//
//	ABI 1 (5.13): initial set — read_file, read_dir, write_file, execute,
//	              remove_*, make_*
//	ABI 2 (5.19): + refer (cross-path rename/link)
//	ABI 3 (6.2):  + truncate
//	ABI 4 (6.7):  + ioctl_dev
//	ABI 5 (6.10): + scoped signals
//	ABI 6 (6.12): additional restrictions
const (
	abiV1 = 1
	abiV3 = 3
)

// readAccessMask is the set of LANDLOCK_ACCESS_FS_* flags requested for a
// read+execute path. It includes execute so the dynamic loader can mmap
// shared libraries, read_file for ordinary file reads, and read_dir for
// directory traversal.
const readAccessMask = unix.LANDLOCK_ACCESS_FS_EXECUTE |
	unix.LANDLOCK_ACCESS_FS_READ_FILE |
	unix.LANDLOCK_ACCESS_FS_READ_DIR

// writeAccessMask is the set of LANDLOCK_ACCESS_FS_* flags requested for a
// read+write path on top of [readAccessMask]. It includes the create/remove
// family so the caller can manage files inside the directory, plus
// truncate (Landlock ABI v3+) when the running kernel supports it.
const writeAccessMask = unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
	unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
	unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
	unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
	unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
	unix.LANDLOCK_ACCESS_FS_MAKE_REG |
	unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
	unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
	unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
	unix.LANDLOCK_ACCESS_FS_MAKE_SYM

// apply is the Linux implementation of the [Apply] platform dispatch. It
// performs the canonical Landlock ruleset dance:
//
//  1. Detect the highest Landlock ABI version supported by the kernel. If
//     the kernel does not support Landlock, return [ErrUnsupported] with no
//     side effects — in particular, NO_NEW_PRIVS is NOT set on
//     unsupported kernels, so callers that fall back to "log and continue"
//     are not silently committed to NO_NEW_PRIVS.
//  2. Set PR_SET_NO_NEW_PRIVS on the calling thread. This is a kernel
//     prerequisite for landlock_restrict_self(2) on any process without
//     CAP_SYS_ADMIN, and is itself irreversible. Once we know Landlock
//     itself works, the prctl side effect is acceptable: the caller has
//     opted into a hardening sequence that requires NNP anyway.
//  3. Build a [unix.LandlockRulesetAttr] with the access mask composed
//     from [readAccessMask] and [writeAccessMask], adding truncate when
//     the kernel supports ABI 3+.
//  4. Call landlock_create_ruleset(2) to obtain a ruleset file descriptor.
//  5. For each read path, open it with O_PATH|O_CLOEXEC, call
//     landlock_add_rule(2) with read+execute, and close the path fd.
//  6. Repeat step 5 for read-write paths with read+execute+write+truncate.
//  7. Call landlock_restrict_self(2) to commit the ruleset irreversibly.
//  8. Close the ruleset fd.
//
// Step 7 is the point of no return for the filesystem ruleset. Step 2
// commits the calling thread to NO_NEW_PRIVS; if a later step (4-7)
// fails, apply returns the wrapped error and the NNP bit remains in
// effect on the thread that ran apply. Steps 1 (probe) and 2 (NNP) are
// ordered this way deliberately so that "Landlock is unsupported" is a
// pure read, never leaving the process partially committed.
func apply(readPaths, readWritePaths []string) error {
	// Step 1: probe ABI support before any irreversible side effect.
	// ABIVersion is a pure read (landlock_create_ruleset with the
	// VERSION flag), so on kernels without Landlock we return cleanly
	// and the caller can decide whether to fall back or fail.
	abi, err := ABIVersion()
	if err != nil {
		// ABIVersion already wraps its own errno in ErrUnsupported; preserve
		// that wrap chain rather than re-wrapping under ErrFailed, so callers
		// can still match ErrUnsupported via errors.Is.
		return err
	}
	if abi < abiV1 {
		return fmt.Errorf("%w: kernel reports ABI version %d", ErrUnsupported, abi)
	}

	// Step 2: NO_NEW_PRIVS is a prerequisite for landlock_restrict_self
	// on any process without CAP_SYS_ADMIN. Delegated to the standalone
	// [nonewprivs] package so both primitives share one canonical
	// implementation. The prctl is idempotent when already set and, like
	// Landlock itself, irreversible for the rest of the calling thread's
	// lifetime. Failure here is vanishingly rare (only kernels older
	// than 3.5, which also do not support Landlock and would have failed
	// step 1) but is re-wrapped as ErrFailed so callers matching on this
	// package's sentinels see a consistent error surface.
	if setErr := nonewprivs.Set(); setErr != nil {
		return fmt.Errorf("%w: %w", ErrFailed, setErr)
	}

	read, write := accessMasks(abi)

	rulesetFd, err := createRuleset(read | write)
	if err != nil {
		return fmt.Errorf("%w: create_ruleset: %w", ErrFailed, err)
	}
	defer func() { _ = unix.Close(rulesetFd) }()

	for _, path := range readPaths {
		if err := addPathRule(rulesetFd, path, read); err != nil {
			return fmt.Errorf("%w: add read rule %q: %w", ErrFailed, path, err)
		}
	}
	for _, path := range readWritePaths {
		if err := addPathRule(rulesetFd, path, read|write); err != nil {
			return fmt.Errorf("%w: add read-write rule %q: %w", ErrFailed, path, err)
		}
	}

	if err := restrictSelf(rulesetFd); err != nil {
		return fmt.Errorf(
			"%w: restrict_self: %w (hint: run with CAP_SYS_ADMIN if NO_NEW_PRIVS is not acceptable)",
			ErrFailed, err,
		)
	}
	return nil
}

// Supported reports whether the running kernel supports Landlock ABI v1 or
// newer. It calls [ABIVersion] and treats any error as "unsupported".
func Supported() bool {
	v, err := ABIVersion()
	return err == nil && v >= abiV1
}

// ABIVersion returns the highest Landlock ABI version supported by the
// running kernel, or [ErrUnsupported] when Landlock is unavailable (kernel
// < 5.13 or CONFIG_SECURITY_LANDLOCK disabled).
//
// It works by calling landlock_create_ruleset with a NULL attribute
// pointer and the LANDLOCK_CREATE_RULESET_VERSION flag, which is
// documented to return the supported version as a small positive integer
// without creating a ruleset.
func ABIVersion() (int, error) {
	r1, _, errno := unix.Syscall(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		0,
		0,
		uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION),
	)
	if errno != 0 {
		return 0, fmt.Errorf("%w: %w", ErrUnsupported, errno)
	}
	return int(r1), nil
}

// accessMasks returns the (read, write) access masks scoped to the kernel's
// supported ABI. ABI v3 added LANDLOCK_ACCESS_FS_TRUNCATE to the write
// set; older kernels skip it.
func accessMasks(abi int) (read, write uint64) {
	read = readAccessMask
	write = writeAccessMask
	if abi >= abiV3 {
		write |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	return read, write
}

// createRuleset wraps landlock_create_ruleset(2).
func createRuleset(handledAccessFs uint64) (int, error) {
	attr := unix.LandlockRulesetAttr{
		Access_fs: handledAccessFs,
	}
	r1, _, errno := unix.Syscall(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if errno != 0 {
		return -1, errno
	}
	return int(r1), nil
}

// addPathRule opens path with O_PATH|O_CLOEXEC, registers it with the
// ruleset under the requested access mask, and closes the path fd. The
// O_PATH open mode is the canonical Landlock idiom — it grants no read or
// write capability on the underlying inode, only enough metadata for the
// kernel to identify it in the ruleset.
func addPathRule(rulesetFd int, path string, accessMask uint64) error {
	pathFd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(pathFd) }()

	attr := unix.LandlockPathBeneathAttr{
		Allowed_access: accessMask,
		Parent_fd:      int32(pathFd),
	}
	_, _, errno := unix.Syscall6(
		unix.SYS_LANDLOCK_ADD_RULE,
		uintptr(rulesetFd),
		uintptr(unix.LANDLOCK_RULE_PATH_BENEATH),
		uintptr(unsafe.Pointer(&attr)),
		0, 0, 0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}

// restrictSelf wraps landlock_restrict_self(2). It commits the ruleset to
// the calling process and every descendant. After this call returns
// successfully there is no way to relax the restriction for the lifetime
// of the process.
func restrictSelf(rulesetFd int) error {
	_, _, errno := unix.Syscall(
		unix.SYS_LANDLOCK_RESTRICT_SELF,
		uintptr(rulesetFd),
		0,
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}

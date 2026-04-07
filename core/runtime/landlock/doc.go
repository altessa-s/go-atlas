// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package landlock wraps the Linux Landlock LSM unprivileged filesystem
// sandbox syscalls (landlock_create_ruleset, landlock_add_rule, and
// landlock_restrict_self) added in kernel 5.13.
//
// Landlock lets a process unilaterally restrict its own filesystem access
// to a strict allowlist without requiring CAP_SYS_ADMIN or root. Once
// [Apply] returns successfully, the restrictions are irreversible for the
// lifetime of the process — there is no in-process undo.
//
// # Quick start
//
// Install a ruleset that allows read+execute under /etc and /usr/lib and
// read+execute+write under /var/log:
//
//	err := landlock.Apply(
//	    landlock.WithReadPaths("/etc", "/usr/lib"),
//	    landlock.WithReadWritePaths("/var/log"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// After [Apply] returns nil, any read, execute, or write outside the
// configured allowlist returns EACCES — even for the current goroutine's
// own descendants.
//
// # Implicit PR_SET_NO_NEW_PRIVS
//
// landlock_restrict_self(2) requires the calling process to either hold
// CAP_SYS_ADMIN or have PR_SET_NO_NEW_PRIVS set. [Apply] sets the
// NO_NEW_PRIVS bit unconditionally as the very first step, so callers do
// not need to call prctl themselves. Like Landlock itself, NO_NEW_PRIVS is
// irreversible for the lifetime of the process — invoking [Apply] commits
// the process to both restrictions. If you need to preserve SUID-exec
// capability after Landlock (a CAP_SYS_ADMIN scenario), call the kernel
// syscalls directly via [golang.org/x/sys/unix] instead of this package.
//
// # Threat model
//
// This package is a defense-in-depth primitive. It restricts filesystem
// access only. It does NOT:
//
//   - Sandbox network, signal, or process syscalls.
//   - Isolate memory, CPU, or IPC.
//   - Protect against a malicious binary already loaded in-process — such
//     a binary still has full access to host memory, stack, and registers.
//
// Use it to reduce blast radius for buggy code and constrain well-behaved
// components to their declared filesystem footprint.
//
// # Caveats
//
//   - Linux 5.13+ only. On older kernels [Apply] returns [ErrUnsupported];
//     on non-Linux platforms every exported function returns
//     [ErrUnsupported] or its zero value equivalent.
//   - Process-wide and irreversible. The restrictions apply to every
//     goroutine and every child process for the rest of the host's
//     lifetime. There is no "unsandbox" syscall.
//   - Strict allowlist. Nothing is auto-added; the operator must list the
//     plugin/loader directories, libc, shared libraries, host data
//     directories, and every other path the process legitimately needs.
//   - Symlinks are resolved before the check; the restriction applies to
//     the target of the symlink, not the link itself.
//
// # ABI version detection
//
// Landlock has evolved across kernel versions:
//
//	ABI 1 (5.13):  initial access set — read_file, read_dir, write_file,
//	               execute, remove_*, make_*
//	ABI 2 (5.19):  + refer (cross-path rename/link)
//	ABI 3 (6.2):   + truncate
//	ABI 4 (6.7):   + ioctl_dev
//	ABI 5 (6.10):  + scoped signals
//	ABI 6 (6.12):  additional restrictions
//
// This package uses access flags up to ABI 3. Older kernels degrade
// gracefully — [Apply] still succeeds on a v1 kernel but without truncate
// enforcement. [ABIVersion] reports the highest version supported at
// runtime; [Supported] returns true when the kernel supports at least
// ABI 1.
package landlock

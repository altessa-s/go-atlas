// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nonewprivs installs the Linux PR_SET_NO_NEW_PRIVS bit on the
// calling process. Once set, the process and every binary it subsequently
// execs cannot gain privileges via SUID/SGID — the kernel silently drops
// the ambient privilege escalation. The bit is irreversible for the
// lifetime of the process.
//
// This package is a defense-in-depth primitive. It does NOT sandbox
// filesystem access, network, memory, or any other resource — only the
// "may a child gain privileges on exec" bit. Combine with
// [github.com/altessa-s/go-atlas/core/runtime/landlock] for filesystem
// confinement and
// [github.com/altessa-s/go-atlas/core/runtime/rlimits] for resource caps.
//
// # Quick start
//
//	if err := nonewprivs.Set(); err != nil {
//	    log.Fatalf("nonewprivs: %v", err)
//	}
//
// # Caveats
//
//   - Linux only. [Set] returns [ErrUnsupported] on non-Linux platforms so
//     cross-platform code can import the package without build tags.
//   - Process-wide and irreversible. Every goroutine, every child process,
//     every binary reached via exec inherits the bit for the rest of the
//     host's lifetime.
//   - Breaks legitimate SUID tools like sudo, su, mount. Only enable in
//     processes that have no legitimate need to exec setuid binaries.
//   - Required prerequisite for
//     landlock_restrict_self(2) on processes without CAP_SYS_ADMIN.
//     [github.com/altessa-s/go-atlas/core/runtime/landlock] sets it
//     automatically for you; there is no need to call [Set] manually when
//     invoking landlock.Apply.
package nonewprivs

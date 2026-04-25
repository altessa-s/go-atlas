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
//   - Per-thread, irreversible per thread. PR_SET_NO_NEW_PRIVS is a
//     per-thread bit on Linux; [Set] only sets it on the goroutine's
//     current OS thread. The Go runtime has multiple OS threads by the
//     time user code runs (sysmon, GC, netpoll, GOMAXPROCS workers), and
//     Go does not expose a way to iterate or pin every existing thread.
//     Peer threads keep their original NNP state, and new threads created
//     via clone(2) inherit NNP from the cloning thread — not from
//     whichever thread called [Set] most recently. The bit IS inherited
//     across execve(2) for binaries spawned from a NNP-set thread, which
//     is its fundamental purpose.
//   - For a real process-wide NNP, set the bit externally before the Go
//     binary starts: a systemd unit with NoNewPrivileges=yes, a container
//     runtime with --security-opt=no-new-privileges, or a C launcher
//     that calls prctl before execve(2) of the Go binary.
//   - Breaks legitimate SUID tools like sudo, su, mount. Only enable in
//     processes that have no legitimate need to exec setuid binaries.
//   - Required prerequisite for landlock_restrict_self(2) on processes
//     without CAP_SYS_ADMIN.
//     [github.com/altessa-s/go-atlas/core/runtime/landlock] sets it
//     automatically for you; there is no need to call [Set] manually when
//     invoking landlock.Apply. Both [Set] and landlock.Apply share the
//     same per-thread limitation described above.
package nonewprivs

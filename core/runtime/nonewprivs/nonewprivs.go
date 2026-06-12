// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nonewprivs

import "errors"

// Sentinel errors for programmatic inspection via [errors.Is]. Errors
// returned from [Set] also preserve the underlying [syscall.Errno] (when
// applicable) via multi-wrap, so callers can additionally use
// [errors.As] to recover the raw kernel error.
var (
	// ErrUnsupported is returned when the running platform does not
	// support PR_SET_NO_NEW_PRIVS (non-Linux platforms, or Linux < 3.5).
	ErrUnsupported = errors.New("nonewprivs: platform does not support PR_SET_NO_NEW_PRIVS")

	// ErrFailed wraps any error from the underlying prctl(2) syscall.
	// The wrap chain preserves the kernel errno so callers can match on
	// [syscall.Errno] via [errors.As] in addition to matching the
	// sentinel via [errors.Is].
	ErrFailed = errors.New("nonewprivs: prctl failed")
)

// Set installs PR_SET_NO_NEW_PRIVS on the calling THREAD. Once set,
// any binary the thread later execs cannot gain privileges via
// SUID/SGID — the kernel silently drops the ambient escalation. The
// bit is irreversible for the lifetime of the thread and idempotent —
// calling Set twice is harmless.
//
// On Linux, Set wraps prctl(2) with PR_SET_NO_NEW_PRIVS. On every other
// platform Set returns [ErrUnsupported].
//
// Per-thread scope (read this before using):
//
// PR_SET_NO_NEW_PRIVS is per-thread on Linux. The Go runtime has
// already created several OS threads (sysmon, GC, netpoll, GOMAXPROCS
// workers) by the time user code runs, so calling Set from a
// goroutine after startup only sets the bit on the goroutine's
// current thread. Peer threads keep their original NNP state. New
// threads created via clone(2) inherit NNP from the cloning thread,
// not from whichever thread called Set most recently — so Set is
// also not retroactive. The kernel does inherit NNP across execve(2)
// for binaries spawned from a NNP-set thread, which is the
// fundamental property the bit is named for.
//
// Callers that sequence Set with another per-thread syscall that
// requires the bit (seccomp(2), landlock_restrict_self(2)) must hold
// [runtime.LockOSThread] across the whole sequence — otherwise the
// scheduler may migrate the goroutine between the calls and the
// follow-up syscall lands on a thread without NNP. The sibling
// seccomp and landlock packages do this internally.
//
// For a real process-wide NNP, set the bit externally before the Go
// binary starts: a systemd unit with NoNewPrivileges=yes, a container
// runtime with --security-opt=no-new-privileges, or a C launcher that
// calls prctl before execve(2) of the Go binary. Set then becomes a
// defense-in-depth helper for the threads that can be reached.
func Set() error {
	return set()
}

// Enabled reports whether PR_SET_NO_NEW_PRIVS is currently set on the
// calling THREAD. It is a thin wrapper around prctl(2) with
// PR_GET_NO_NEW_PRIVS. Returns (false, [ErrUnsupported]) on non-Linux
// platforms. Like [Set], the result reflects the calling thread only,
// not the whole process.
func Enabled() (bool, error) {
	return enabled()
}

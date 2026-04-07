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

// Set installs PR_SET_NO_NEW_PRIVS on the calling process. Once set, the
// process and every binary it execs cannot gain privileges via SUID/SGID
// for the rest of the process's lifetime. The operation is irreversible
// and idempotent — calling Set twice is harmless.
//
// On Linux, Set wraps prctl(2) with PR_SET_NO_NEW_PRIVS. On every other
// platform Set returns [ErrUnsupported].
//
// Set is safe to call from any goroutine and at any point in the
// process's lifetime, but is typically invoked once during startup.
func Set() error {
	return set()
}

// Enabled reports whether PR_SET_NO_NEW_PRIVS is currently set on the
// calling process. It is a thin wrapper around prctl(2) with
// PR_GET_NO_NEW_PRIVS. Returns (false, [ErrUnsupported]) on non-Linux
// platforms.
func Enabled() (bool, error) {
	return enabled()
}

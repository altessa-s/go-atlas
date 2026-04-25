// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

package nonewprivs

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// set is the Linux implementation of [Set]. It calls
// prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0). A failure here is vanishingly
// rare in practice — the syscall has existed since kernel 3.5, so on any
// kernel new enough to run modern userspace it either succeeds or fails
// for reasons far outside this package's control (seccomp filter,
// exotic LSM stack).
func set() error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("%w: PR_SET_NO_NEW_PRIVS: %w", ErrFailed, err)
	}
	return nil
}

// enabled is the Linux implementation of [Enabled]. It calls
// prctl(PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0) and returns true when the
// kernel reports the bit is set.
func enabled() (bool, error) {
	v, err := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0)
	if err != nil {
		return false, fmt.Errorf("%w: PR_GET_NO_NEW_PRIVS: %w", ErrFailed, err)
	}
	return v == 1, nil
}

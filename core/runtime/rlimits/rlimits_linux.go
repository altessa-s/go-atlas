// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

package rlimits

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

// apply is the Linux implementation of the [Apply] platform dispatch.
// It walks every configured rlimit in a fixed order and calls
// syscall.Setrlimit for each non-zero value. A failure on any single
// rlimit aborts immediately and leaves earlier successful rlimits in
// place — partial application is intentional, because a half-applied
// sandbox is better than silently dropping requested restrictions.
func apply(o *options) error {
	if err := setLimit(syscall.RLIMIT_AS, "RLIMIT_AS", o.memoryBytes); err != nil {
		return err
	}
	if err := setLimit(syscall.RLIMIT_NOFILE, "RLIMIT_NOFILE", o.maxOpenFiles); err != nil {
		return err
	}
	if err := setLimit(unix.RLIMIT_NPROC, "RLIMIT_NPROC", o.maxProcesses); err != nil {
		return err
	}
	if err := setLimit(syscall.RLIMIT_FSIZE, "RLIMIT_FSIZE", o.maxFileSizeBytes); err != nil {
		return err
	}
	if o.disableCoreDumps {
		zero := syscall.Rlimit{Cur: 0, Max: 0}
		if err := syscall.Setrlimit(syscall.RLIMIT_CORE, &zero); err != nil {
			return fmt.Errorf("%w: setrlimit RLIMIT_CORE=0: %w", ErrFailed, err)
		}
	}
	return nil
}

// setLimit applies a single rlimit when value > 0, leaving the kernel
// default in place otherwise. Both Cur and Max are pinned to value, so
// the soft limit can never be raised back above value for the lifetime
// of the process and the hard limit cannot be raised at all without
// CAP_SYS_RESOURCE.
func setLimit(resource int, name string, value int64) error {
	if value <= 0 {
		return nil
	}
	rl := syscall.Rlimit{Cur: uint64(value), Max: uint64(value)}
	if err := syscall.Setrlimit(resource, &rl); err != nil {
		return fmt.Errorf("%w: setrlimit %s=%d: %w", ErrFailed, name, value, err)
	}
	return nil
}

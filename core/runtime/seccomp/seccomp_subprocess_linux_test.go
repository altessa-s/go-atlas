// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux && (amd64 || arm64)

// Subprocess tests for the irreversible Linux primitives in this
// package. seccomp(SECCOMP_SET_MODE_FILTER) cannot be removed once
// installed and PR_SET_NO_NEW_PRIVS is committed as part of the
// install sequence, so a successful BlockDangerousSyscalls call
// would contaminate every later test in the same binary. These
// tests run in a re-execed child that exits immediately after the
// assertion.

package seccomp_test

import (
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/internal/exectest"
	"github.com/altessa-s/go-atlas/core/runtime/seccomp"

	"golang.org/x/sys/unix"
)

// TestBlockDangerousSyscalls_DeniesMountInSubprocess is the headline
// integration test. It installs the denylist and then tries to call
// mount(2) — which is in dangerousSyscalls. The expected outcome is
// EPERM (the kernel rewrites the syscall return per the BPF program's
// SECCOMP_RET_ERRNO|EPERM action).
//
// mount is the cleanest probe in the denylist because it has no
// privileged side effect when it fails — unlike e.g. reboot — and the
// kernel does not require special arguments to reach the seccomp
// filter check.
func TestBlockDangerousSyscalls_DeniesMountInSubprocess(t *testing.T) {
	exectest.RunInSubprocess(t, func() {
		if err := seccomp.BlockDangerousSyscalls(); err != nil {
			exectest.Failf("BlockDangerousSyscalls: %v", err)
		}

		// Try mount("none", "/tmp", "tmpfs", 0, ""). The
		// arguments are deliberately benign — the seccomp filter
		// fires before the kernel evaluates them, so we never
		// actually mount anything. We expect EPERM from the
		// filter, not EINVAL or EACCES from a real mount attempt.
		err := unix.Mount("none", "/tmp", "tmpfs", 0, "")
		if err == nil {
			exectest.Failf("mount(2) unexpectedly succeeded after BlockDangerousSyscalls — " +
				"the seccomp filter did not intercept it")
		}
		if !errors.Is(err, unix.EPERM) {
			exectest.Failf("mount(2) returned %v, want EPERM "+
				"(seccomp filter should rewrite return to EPERM via SECCOMP_RET_ERRNO)",
				err)
		}
	})
}

// TestBlockDangerousSyscalls_DeniesPtraceInSubprocess covers a
// second entry in the denylist with a different syscall family. If
// the BPF program's JEQ chain were broken in a way that only blocks
// the first or last entry, this catches it.
func TestBlockDangerousSyscalls_DeniesPtraceInSubprocess(t *testing.T) {
	exectest.RunInSubprocess(t, func() {
		if err := seccomp.BlockDangerousSyscalls(); err != nil {
			exectest.Failf("BlockDangerousSyscalls: %v", err)
		}

		// ptrace(PTRACE_TRACEME, 0, 0, 0). PTRACE_TRACEME is the
		// most benign ptrace request: it asks the kernel to mark
		// the calling process as traceable by its parent. With
		// the seccomp filter installed, the kernel must intercept
		// the syscall before evaluating the request type.
		_, _, errno := unix.Syscall6(
			unix.SYS_PTRACE,
			uintptr(unix.PTRACE_TRACEME),
			0, 0, 0, 0, 0,
		)
		if errno == 0 {
			exectest.Failf("ptrace(PTRACE_TRACEME) unexpectedly succeeded after " +
				"BlockDangerousSyscalls — the seccomp filter did not intercept it")
		}
		if errno != unix.EPERM {
			exectest.Failf("ptrace(2) returned errno=%v, want EPERM "+
				"(seccomp filter should rewrite return via SECCOMP_RET_ERRNO|EPERM)",
				errno)
		}
	})
}

// TestBlockDangerousSyscalls_AllowsBenignSyscallInSubprocess
// guards against an over-broad filter that accidentally rewrites
// syscalls outside the denylist. We install the filter and then
// call getpid(2), which is not in dangerousSyscalls and must return
// successfully — without this check, a regression that swapped the
// "default allow" and "default deny" actions would only be caught
// when something else broke first.
func TestBlockDangerousSyscalls_AllowsBenignSyscallInSubprocess(t *testing.T) {
	exectest.RunInSubprocess(t, func() {
		if err := seccomp.BlockDangerousSyscalls(); err != nil {
			exectest.Failf("BlockDangerousSyscalls: %v", err)
		}
		// getpid is the canonical "trivially allowed" syscall and
		// the Go runtime calls it indirectly via os.Getpid.
		pid := unix.Getpid()
		if pid <= 0 {
			exectest.Failf("getpid returned %d after seccomp install — filter is rewriting "+
				"syscalls outside the denylist", pid)
		}
	})
}

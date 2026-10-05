// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux && amd64

package seccomp

import "golang.org/x/sys/unix"

// expectedArch is the AUDIT_ARCH_* token the filter's arch-prologue
// compares against. On amd64 this is AUDIT_ARCH_X86_64. A kernel
// that hands us seccomp_data.arch with any other value (e.g. an i386
// syscall issued via int 0x80, reported as AUDIT_ARCH_I386) causes the filter to
// kill the process — syscall numbers differ by arch and trusting the
// wrong numbering is worse than no filter at all.
const expectedArch uint32 = unix.AUDIT_ARCH_X86_64

// rejectX32 makes the filter kill the process for x32 ABI syscalls.
// They report AUDIT_ARCH_X86_64, so the arch check does not catch
// them, but they set [x32SyscallBit] in the syscall number (or, before
// Linux 5.4, may use the x32-only numbers 512–547 without it) and
// would otherwise miss every denylist entry and fall through to ALLOW.
const rejectX32 = true
